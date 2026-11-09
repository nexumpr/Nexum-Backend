"""Verificador local de publicacion y compatibilidad de la API (I-150).

Sin AWS y sin HTTP real:

- ``contrato``: OpenAPI 3.1 coherente con A-040/A-110 y con la matriz H06.
- ``publicacion``: URL HTTPS, artefacto OpenAPI con checksum verificado y CORS seguro.
- ``compatibilidad``: matriz E001-E075 con clases M/S/E/O y campos obligatorios.
- ``evidencia``: plan HTTP desde cliente independiente, todo NOT_RUN.
- ``deprecacion``: versionado semantico y politica de cambios incompatibles.
- ``alcance``: Mobile, Play Store y landing fuera del gate del Backend.

Codigos: 0 correcto, 1 rechazado, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
ENDPOINTS = RAIZ / "sdd" / "03-endpoints"
OPENAPI_JSON = ENDPOINTS / "openapi" / "nexum-api-v1.json"
MATRIZ_CSV = ENDPOINTS / "MATRIZ_OPERACIONES_H06.csv"
CONTROL = RAIZ / "sdd" / "_control" / "H08"
FIXTURES = RAIZ / "tests" / "fixtures" / "publicacion"

CLASES = ("M", "S", "E", "O")
CLASES_ESPERADAS = {"M": 33, "S": 23, "E": 12, "O": 7}
METODOS = ("get", "post", "put", "patch", "delete")
PREFIJO_API = "/api/v1/"

CAMPOS_MATRIZ = (
    "id",
    "metodo",
    "ruta",
    "clase_h04",
    "dominio",
    "resumen",
    "alcance",
    "auth",
    "request_schema",
    "response_schema",
    "http_exito",
    "idempotency_key",
    "if_match",
    "carpeta",
    "regla_clave",
)

EVIDENCIAS_REQUERIDAS = (
    "health-live",
    "health-ready",
    "registro",
    "login",
    "logout",
    "endpoint-adicional-1",
    "endpoint-adicional-2",
    "endpoint-adicional-3",
    "endpoint-adicional-4",
    "errores-401-403-404",
    "tls-ca-valida",
    "persistencia-dynamodb",
    "dos-nodos",
)

FUERA_DE_ALCANCE = ("android", "play-store", "pantallas-mobile", "pruebas-mobile", "landing")


class ErrorUso(Exception):
    """Entrada invalida (codigo 2)."""


def cargar_json(ruta: Path | str) -> Any:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorUso(f"No se puede leer {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorUso(f"{ruta} no es JSON valido: {exc}") from exc


def sha256_archivo(ruta: Path) -> str:
    return hashlib.sha256(ruta.read_bytes()).hexdigest()


# --------------------------------------------------------------------------- #
# INF-150-01 contrato
# --------------------------------------------------------------------------- #


def validar_contrato(datos: Mapping[str, Any] | None = None, ruta: Path | None = None) -> dict[str, Any]:
    if datos is None:
        objetivo = Path(ruta) if ruta else OPENAPI_JSON
        if not objetivo.is_file():
            raise ErrorUso(f"Falta el contrato OpenAPI {objetivo}")
        datos = cargar_json(objetivo)

    problemas: list[str] = []
    if datos.get("openapi") != "3.1.0":
        problemas.append(f"openapi debe ser 3.1.0, es {datos.get('openapi')!r}")

    servidores = datos.get("servers") or []
    if not servidores:
        problemas.append("falta el bloque servers")
    else:
        url = str(servidores[0].get("url", ""))
        placeholder = "example.invalid" in url or bool(servidores[0].get("x-nexum-placeholder"))
        if not url.startswith("https://"):
            problemas.append("la URL publica debe ser HTTPS")
        if not placeholder:
            problemas.append("la URL no puede afirmar un endpoint AWS real antes del despliegue")

    paths = datos.get("paths") or {}
    if not paths:
        problemas.append("no hay rutas")
    for ruta_api, operaciones in paths.items():
        if not ruta_api.startswith(PREFIJO_API):
            problemas.append(f"ruta fuera de {PREFIJO_API}: {ruta_api}")
        for metodo in operaciones:
            if metodo in METODOS and not operaciones[metodo].get("operationId"):
                problemas.append(f"{metodo.upper()} {ruta_api} sin operationId")

    esquemas = (datos.get("components") or {}).get("schemas") or {}
    tiene_error = any(
        isinstance(e, Mapping) and "traceId" in json.dumps(e) for e in esquemas.values()
    )
    if not tiene_error:
        problemas.append("no hay esquema de error con traceId")

    dinero_sospechoso = []
    for nombre, esquema in esquemas.items():
        texto = json.dumps(esquema)
        for campo in ("amount", "importe", "balance", "saldo"):
            if f'"{campo}"' in texto and '"type": "number"' in texto:
                dinero_sospechoso.append(f"{nombre}.{campo}")
    if dinero_sospechoso:
        problemas.append("importes monetarios como number: " + ", ".join(dinero_sospechoso))

    cabeceras_idem = {
        str(p.get("name", "")).lower()
        for ruta_api, ops in paths.items()
        for metodo, op in ops.items()
        if metodo in METODOS
        for p in (op.get("parameters") or [])
        if isinstance(p, Mapping)
    }
    return {
        "comando": "contrato",
        "openapi": datos.get("openapi"),
        "rutas": len(paths),
        "esquemas": len(esquemas),
        "errores_con_traceid": tiene_error,
        "cabeceras": sorted(c for c in cabeceras_idem if c),
        "problemas": problemas,
        "estado": "CONTRATO_VALIDO" if not problemas else "CONTRATO_RECHAZADO",
        "advertencia": "La URL del contrato es documental; ninguna ruta se comprobo por HTTP real.",
    }


# --------------------------------------------------------------------------- #
# INF-150-02 publicacion
# --------------------------------------------------------------------------- #


def validar_publicacion(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con el plan de publicacion.")
    problemas: list[str] = []

    url = str(datos.get("url_publica", ""))
    if not url.startswith("https://"):
        problemas.append("la URL publica debe ser HTTPS")

    artefacto = datos.get("artefacto_openapi") or {}
    ruta_artefacto = str(artefacto.get("ruta", ""))
    declarado = str(artefacto.get("sha256", ""))
    if not ruta_artefacto or not declarado:
        problemas.append("faltan ruta o checksum del artefacto OpenAPI")
    else:
        archivo = RAIZ / ruta_artefacto
        if not archivo.is_file():
            problemas.append(f"artefacto inexistente: {ruta_artefacto}")
        else:
            real = sha256_archivo(archivo)
            if declarado.lower() != real:
                problemas.append(f"checksum del artefacto no coincide ({declarado[:12]}… vs {real[:12]}…)")

    cors = datos.get("cors") or {}
    origenes = cors.get("origenes") or []
    if not isinstance(origenes, list) or not origenes:
        problemas.append("CORS debe declarar una allowlist de origenes")
    if "*" in origenes:
        problemas.append("CORS no puede usar '*' (menos aun con credenciales)")
    if cors.get("credenciales") is True and "*" in origenes:
        problemas.append("CORS con credenciales exige origenes explicitos")

    ejemplos = datos.get("ejemplos") or {}
    if ejemplos.get("con_credenciales") is True:
        problemas.append("los ejemplos publicados no pueden incluir credenciales")

    return {
        "comando": "publicacion",
        "url_publica": url,
        "artefacto": ruta_artefacto,
        "checksum_verificado": "checksum del artefacto no coincide" not in " ".join(problemas)
        and bool(ruta_artefacto),
        "problemas": problemas,
        "estado": "PUBLICACION_VALIDA" if not problemas else "PUBLICACION_RECHAZADA",
        "advertencia": "No se publico nada: el plan solo se valida en local.",
    }


# --------------------------------------------------------------------------- #
# INF-150-03 compatibilidad
# --------------------------------------------------------------------------- #


def leer_matriz(ruta: Path | None = None) -> list[dict[str, str]]:
    objetivo = Path(ruta) if ruta else MATRIZ_CSV
    if not objetivo.is_file():
        raise ErrorUso(f"Falta la matriz H06 {objetivo}")
    with objetivo.open(encoding="utf-8") as manejador:
        return list(csv.DictReader(manejador))


def validar_compatibilidad(filas: list[dict[str, str]] | None = None) -> dict[str, Any]:
    datos = filas if filas is not None else leer_matriz()
    problemas: list[str] = []
    if len(datos) != 75:
        problemas.append(f"la matriz debe tener 75 filas, tiene {len(datos)}")

    identificadores = {fila.get("id", "") for fila in datos}
    esperados = {f"E{i:03d}" for i in range(1, 76)}
    if identificadores != esperados:
        problemas.append("identificadores incompletos o duplicados")

    conteo = {clase: 0 for clase in CLASES}
    obligatorios = [c for c in CAMPOS_MATRIZ if c not in ("request_schema", "response_schema")]
    for fila in datos:
        identificador = fila.get("id", "?")
        for campo in obligatorios:
            if not fila.get(campo):
                problemas.append(f"{identificador}: falta {campo}")
        # Un 204 sin cuerpo es legitimo; el resto debe declarar esquemas.
        exito = str(fila.get("http_exito", ""))
        if exito != "204" and not fila.get("response_schema"):
            problemas.append(f"{identificador}: falta response_schema")
        if fila.get("metodo") in ("POST", "PUT") and exito not in ("204", "202") and not fila.get("request_schema"):
            problemas.append(f"{identificador}: falta request_schema")
        clase = fila.get("clase_h04")
        if clase in conteo:
            conteo[clase] += 1
        else:
            problemas.append(f"{identificador}: clase desconocida {clase!r}")
        ruta = fila.get("ruta", "")
        if not ruta.startswith(PREFIJO_API):
            problemas.append(f"{identificador}: ruta fuera de {PREFIJO_API}")
        if fila.get("metodo") not in ("GET", "POST", "PUT", "PATCH", "DELETE"):
            problemas.append(f"{identificador}: metodo invalido")

    if conteo != CLASES_ESPERADAS:
        problemas.append(f"clases {conteo} != {CLASES_ESPERADAS}")

    return {
        "comando": "compatibilidad",
        "filas": len(datos),
        "clases": conteo,
        "problemas": problemas,
        "estado": "MATRIZ_COMPATIBLE" if not problemas else "MATRIZ_INCOMPATIBLE",
        "advertencia": (
            "La clasificacion M/S/E/O no exime requisitos obligatorios del profesor; un contrato "
            "OpenAPI no demuestra handler funcional."
        ),
    }


# --------------------------------------------------------------------------- #
# INF-150-04 evidencia HTTP
# --------------------------------------------------------------------------- #


def validar_evidencia(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con el plan de evidencia HTTP.")
    pruebas = datos.get("pruebas")
    if not isinstance(pruebas, list):
        raise ErrorUso("'pruebas' debe ser una lista.")

    identificadores = {str(p.get("id")) for p in pruebas if isinstance(p, Mapping)}
    faltantes = [nombre for nombre in EVIDENCIAS_REQUERIDAS if nombre not in identificadores]
    problemas: list[str] = []
    falsos_pass: list[str] = []

    for prueba in pruebas:
        if not isinstance(prueba, Mapping):
            problemas.append("prueba malformada")
            continue
        identificador = str(prueba.get("id"))
        for campo in ("metodo", "ruta", "esperado", "estado"):
            if not prueba.get(campo):
                problemas.append(f"{identificador}: falta {campo}")
        if prueba.get("cliente_independiente") is not True:
            problemas.append(f"{identificador}: exige cliente independiente del Backend")
        estado = prueba.get("estado")
        if estado not in ("NOT_RUN", "PASS", "FAIL", "BLOCKED"):
            problemas.append(f"{identificador}: estado desconocido {estado!r}")
        if estado == "PASS":
            if prueba.get("requiere_aws") is not False:
                falsos_pass.append(identificador)
                problemas.append(
                    f"{identificador}: no puede declararse PASS sin ejecucion AWS autorizada"
                )
            if not prueba.get("evidencia_sha256"):
                problemas.append(f"{identificador}: PASS exige huella de la respuesta")

    return {
        "comando": "evidencia",
        "requeridas": len(EVIDENCIAS_REQUERIDAS),
        "presentes": len(identificadores),
        "faltantes": faltantes,
        "declaran_pass_sin_aws": falsos_pass,
        "problemas": problemas,
        "estado": "PLAN_VALIDO" if not faltantes and not problemas else "PLAN_RECHAZADO",
        "ejecucion_aws": "NOT_RUN",
        "advertencia": "Ninguna evidencia HTTP se ejecuto; todas las pruebas quedan NOT_RUN.",
    }



# --------------------------------------------------------------------------- #
# INF-150-01b paridad YAML / JSON / matriz
# --------------------------------------------------------------------------- #

YAML = ENDPOINTS / "openapi" / "nexum-api-v1.yaml"
METODOS_OPENAPI = ("get", "post", "put", "patch", "delete")

CLAVES_CONTENIDO = ("text", "content", "message", "description", "body", "comment")
FORMATOS_MEDIA = ("binary", "base64", "byte")


def _lineas_yaml(texto_yaml: str) -> list[tuple[int, str]]:
    resultado: list[tuple[int, str]] = []
    for numero, linea in enumerate(texto_yaml.splitlines(), start=1):
        limpia = linea.rstrip()
        if not limpia.strip() or limpia.lstrip().startswith("#"):
            continue
        sangria = len(limpia) - len(limpia.lstrip(" \t"))
        if "\t" in limpia[:sangria]:
            raise ErrorUso(f"YAML con tabulaciones en la linea {numero}")
        resultado.append((sangria, limpia.strip()))
    return resultado


def _escalar(valor: str) -> Any:
    limpio = valor.strip()
    if limpio.startswith("'") and limpio.endswith("'") and len(limpio) > 1:
        return limpio[1:-1]
    if limpio.startswith('"') and limpio.endswith('"') and len(limpio) > 1:
        return limpio[1:-1]
    if limpio == "[]":
        return []
    if limpio == "{}":
        return {}
    if limpio.lower() in ("true", "false"):
        return limpio.lower() == "true"
    if limpio.startswith("["):
        interior = limpio[1:-1] if limpio.endswith("]") else limpio[1:]
        return [_escalar(parte) for parte in interior.split(",") if parte.strip()]
    return limpio


def _parse_mapa(lineas: list[tuple[int, str]], indice: int, sangria: int) -> tuple[dict[str, Any], int]:
    resultado: dict[str, Any] = {}
    while indice < len(lineas):
        nivel, contenido = lineas[indice]
        if nivel < sangria:
            break
        if nivel > sangria:
            indice += 1
            continue
        if contenido.startswith("- "):
            break
        clave, _, resto = contenido.partition(":")
        clave = clave.strip().strip("'\"")
        resto = resto.strip()
        if clave in resultado:
            raise ErrorUso(f"clave duplicada en YAML: {clave}")
        indice += 1
        if resto:
            resultado[clave] = _escalar(resto)
            continue
        if indice < len(lineas) and lineas[indice][0] > sangria:
            hijo, indice = _parse(lineas, indice, lineas[indice][0])
            resultado[clave] = hijo
        elif indice < len(lineas) and lineas[indice][0] == sangria and lineas[indice][1].startswith("- "):
            hijo, indice = _parse_lista(lineas, indice, sangria)
            resultado[clave] = hijo
        else:
            resultado[clave] = None
    return resultado, indice


def _parse_lista(lineas: list[tuple[int, str]], indice: int, sangria: int) -> tuple[list[Any], int]:
    resultado: list[Any] = []
    while indice < len(lineas):
        nivel, contenido = lineas[indice]
        if nivel != sangria or not contenido.startswith("- "):
            break
        elemento = contenido[2:].strip()
        indice += 1
        if not elemento:
            if indice < len(lineas) and lineas[indice][0] > sangria:
                valor, indice = _parse(lineas, indice, lineas[indice][0])
                resultado.append(valor)
            else:
                resultado.append(None)
            continue
        if ":" in elemento:
            clave, _, resto = elemento.partition(":")
            clave = clave.strip().strip("'\"")
            resto = resto.strip()
            item: dict[str, Any] = {clave: (_escalar(resto) if resto else None)}
            if indice < len(lineas) and lineas[indice][0] > sangria:
                extra, indice = _parse_mapa(lineas, indice, lineas[indice][0])
                item.update(extra)
            resultado.append(item)
        else:
            resultado.append(_escalar(elemento))
    return resultado, indice


def _parse(lineas: list[tuple[int, str]], indice: int, sangria: int) -> tuple[Any, int]:
    if indice < len(lineas) and lineas[indice][0] == sangria and lineas[indice][1].startswith("- "):
        return _parse_lista(lineas, indice, sangria)
    return _parse_mapa(lineas, indice, sangria)


def estructura_yaml(texto_yaml: str) -> dict[str, Any]:
    """Parsea el YAML (subconjunto de bloque) y devuelve la estructura equivalente."""
    lineas = _lineas_yaml(texto_yaml)
    if not lineas:
        raise ErrorUso("YAML vacio")
    if lineas[0][0] != 0:
        raise ErrorUso("YAML malformado: la raiz debe empezar en la columna 0")
    estructura, consumidas = _parse_mapa(lineas, 0, 0)
    if consumidas != len(lineas):
        raise ErrorUso(
            f"YAML no consumido por completo ({consumidas} de {len(lineas)} lineas): estructura no soportada"
        )
    return estructura


def _refs_json(nodo: Any) -> set[str]:
    encontrados: set[str] = set()
    if isinstance(nodo, Mapping):
        for clave, valor in nodo.items():
            if clave == "$ref" and isinstance(valor, str):
                encontrados.add(valor)
            else:
                encontrados |= _refs_json(valor)
    elif isinstance(nodo, list):
        for valor in nodo:
            encontrados |= _refs_json(valor)
    return encontrados


def _seguridad(valor: Any) -> list[dict[str, list[str]]]:
    if not isinstance(valor, list):
        return []
    normal: list[dict[str, list[str]]] = []
    for requisito in valor:
        if isinstance(requisito, Mapping):
            normal.append({str(k): sorted(map(str, v or [])) for k, v in requisito.items()})
    return normal


def _params(valor: Any) -> list[tuple[str, str, bool]]:
    normal: list[tuple[str, str, bool]] = []
    for parametro in valor or []:
        if not isinstance(parametro, Mapping):
            continue
        if parametro.get("$ref"):
            normal.append((str(parametro["$ref"]), "", False))
            continue
        normal.append(
            (
                str(parametro.get("name", "")),
                str(parametro.get("in", "")),
                bool(parametro.get("required")),
            )
        )
    return sorted(normal)


def _respuestas(valor: Any) -> dict[str, str]:
    normal: dict[str, str] = {}
    for codigo, cuerpo in (valor or {}).items():
        if not isinstance(cuerpo, Mapping):
            normal[str(codigo)] = ""
            continue
        esquema = ((cuerpo.get("content") or {}).get("application/json") or {}).get("schema") or {}
        normal[str(codigo)] = str(esquema.get("$ref") or esquema.get("type") or "")
    return normal


def _ref_cuerpo(valor: Any) -> str:
    esquema = ((valor or {}).get("content") or {}).get("application/json", {}).get("schema") or {}
    return str(esquema.get("$ref") or "")


def validar_paridad(ruta_yaml: Path | None = None) -> dict[str, Any]:
    """Paridad semantica JSON ↔ YAML ↔ matriz, operacion por operacion."""
    if not OPENAPI_JSON.is_file() or not YAML.is_file() or not MATRIZ_CSV.is_file():
        raise ErrorUso("Faltan JSON, YAML o la matriz H06.")
    datos = cargar_json(OPENAPI_JSON)
    origen_yaml = Path(ruta_yaml) if ruta_yaml else YAML
    if not origen_yaml.is_file():
        raise ErrorUso(f"Falta el YAML {origen_yaml}")
    estructura = estructura_yaml(origen_yaml.read_text(encoding="utf-8"))
    filas = leer_matriz()

    problemas: list[str] = []
    rutas_json = datos.get("paths") or {}
    rutas_yaml = estructura.get("paths") or {}
    esquemas = (datos.get("components") or {}).get("schemas") or {}
    parametros_componentes = (datos.get("components") or {}).get("parameters") or {}
    refs_validas = {f"#/components/schemas/{n}" for n in esquemas} | {
        f"#/components/parameters/{n}" for n in parametros_componentes
    }

    ops_json = {(ruta, m) for ruta, item in rutas_json.items() for m in item if m in METODOS_OPENAPI}
    ops_yaml = {
        (ruta, m)
        for ruta, item in rutas_yaml.items()
        if isinstance(item, Mapping)
        for m in item
        if m in METODOS_OPENAPI
    }
    for ruta, metodo in sorted(ops_json - ops_yaml):
        problemas.append(f"YAML sin {metodo.upper()} {ruta}")
    for ruta, metodo in sorted(ops_yaml - ops_json):
        problemas.append(f"YAML con {metodo.upper()} {ruta} inexistente en JSON")
    if len(ops_json) != 75 or len(ops_yaml) != 75:
        problemas.append(f"operaciones JSON={len(ops_json)} YAML={len(ops_yaml)}, se esperan 75")

    for ruta, metodo in sorted(ops_json & ops_yaml):
        op_json = rutas_json[ruta][metodo]
        op_yaml = rutas_yaml[ruta][metodo]
        etiqueta = f"{metodo.upper()} {ruta}"

        if op_json.get("operationId") != op_yaml.get("operationId"):
            problemas.append(f"{etiqueta}: operationId difiere")
        if op_json.get("x-nexum-catalog-id") != op_yaml.get("x-nexum-catalog-id"):
            problemas.append(f"{etiqueta}: x-nexum-catalog-id difiere")
        if _seguridad(op_json.get("security")) != _seguridad(op_yaml.get("security")):
            problemas.append(f"{etiqueta}: seguridad difiere")
        if _params(op_json.get("parameters")) != _params(op_yaml.get("parameters")):
            problemas.append(f"{etiqueta}: parametros difieren")
        if _ref_cuerpo(op_json.get("requestBody")) != _ref_cuerpo(op_yaml.get("requestBody")):
            problemas.append(f"{etiqueta}: requestBody difiere")
        respuestas_json = _respuestas(op_json.get("responses"))
        respuestas_yaml = _respuestas(op_yaml.get("responses"))
        if respuestas_json != respuestas_yaml:
            problemas.append(f"{etiqueta}: respuestas difieren")

        for referencia in _refs_json(op_yaml) | _refs_json(op_json):
            if referencia not in refs_validas:
                problemas.append(f"{etiqueta}: $ref irresoluble {referencia}")

    ids_csv = {fila.get("id") for fila in filas}
    catalogo = {
        str(op.get("x-nexum-catalog-id"))
        for item in rutas_json.values()
        for op in item.values()
        if isinstance(op, Mapping)
    }
    if catalogo != ids_csv:
        problemas.append("x-nexum-catalog-id no coincide con la matriz H06")
    for fila in filas:
        esperado = "nexum_" + str(fila.get("id")).lower()
        operacion = (rutas_json.get(fila.get("ruta")) or {}).get(str(fila.get("metodo")).lower(), {})
        if operacion.get("operationId") != esperado:
            problemas.append(f"{fila.get('id')}: operationId distinto de {esperado}")

    return {
        "comando": "paridad",
        "operaciones_json": len(ops_json),
        "operaciones_yaml": len(ops_yaml),
        "rutas_json": len(rutas_json),
        "rutas_yaml": len(rutas_yaml),
        "filas_matriz": len(filas),
        "problemas": problemas,
        "estado": "PARIDAD_OK" if not problemas else "PARIDAD_ROTA",
        "advertencia": (
            "Paridad semantica (metodo, ruta, operationId, seguridad, parametros, requestBody y "
            "respuestas) contra el YAML real; no acredita handlers ni HTTP."
        ),
    }


# --------------------------------------------------------------------------- #
# INF-150-07 inventario de contenido y moderacion
# --------------------------------------------------------------------------- #


def escanear_contenido(contrato: Mapping[str, Any] | None = None) -> dict[str, Any]:
    """Clasifica operaciones con contenido de usuario y exige diseño de moderacion."""
    datos = contrato if contrato is not None else cargar_json(OPENAPI_JSON)
    esquemas = (datos.get("components") or {}).get("schemas") or {}
    media: list[str] = []
    texto_libre: list[str] = []

    for ruta, item in (datos.get("paths") or {}).items():
        for metodo, operacion in (item or {}).items():
            if metodo not in METODOS_OPENAPI or not isinstance(operacion, Mapping):
                continue
            identificador = str(operacion.get("x-nexum-catalog-id") or f"{metodo.upper()} {ruta}")
            cuerpo = ((operacion.get("requestBody") or {}).get("content") or {}).get("application/json") or {}
            referencia = (cuerpo.get("schema") or {}).get("$ref", "")
            nombre = referencia.rsplit("/", 1)[-1] if referencia else ""
            propiedades = ((esquemas.get(nombre) or {}).get("properties") or {})
            for campo, propiedad in propiedades.items():
                if not isinstance(propiedad, Mapping):
                    continue
                formato = str(propiedad.get("format", ""))
                if formato in FORMATOS_MEDIA:
                    media.append(f"{identificador}:{campo}")
                elif any(clave in campo.lower() for clave in CLAVES_CONTENIDO) and formato not in ("email", "date", "date-time", "uuid"):
                    texto_libre.append(f"{identificador}:{campo}")

    diseno = CONTROL / "MODERACION_CDN_I150.md"
    problemas: list[str] = []
    if media and not diseno.is_file():
        problemas.append("hay operaciones con medios y falta MODERACION_CDN_I150.md")

    # Cada operacion con contenido debe figurar en el diseno (fail-closed).
    if (media or texto_libre) and not diseno.is_file():
        problemas.append("hay operaciones con contenido y falta MODERACION_CDN_I150.md")
    if diseno.is_file():
        documento = diseno.read_text(encoding="utf-8")
        for entrada in sorted(set(media) | set(texto_libre)):
            identificador = entrada.split(":")[0]
            if identificador not in documento:
                problemas.append(f"{identificador} con contenido sin tratamiento documentado")
        if not re.search(r"DISEÑO_LOCAL_COMPLETO|DISENO_LOCAL_COMPLETO", documento):
            problemas.append("el diseno no declara su estado local")
        if "moderacion_operativa" not in documento and "moderación operativa" not in documento.lower():
            problemas.append("el diseno no declara que la moderacion no esta operativa")

    return {
        "comando": "contenido",
        "operaciones_con_media": sorted(set(media)),
        "operaciones_con_texto_libre": sorted(set(texto_libre)),
        "diseno_moderacion": diseno.is_file(),
        "problemas": problemas,
        "estado": "CONTENIDO_CUBIERTO" if not problemas else "CONTENIDO_SIN_DISENO",
        "moderacion_operativa": False,
        "estado_diseno": "DISEÑO_LOCAL_COMPLETO — IMPLEMENTACIÓN_CLOUD_PENDIENTE",
        "advertencia": (
            "Ningun servicio de moderacion o CDN esta desplegado; el diseno no acredita moderacion real."
        ),
    }


# --------------------------------------------------------------------------- #
# INF-150-04 comandos de evidencia (sin ejecutar HTTP)
# --------------------------------------------------------------------------- #


def generar_comandos(datos: Mapping[str, Any]) -> list[str]:
    comandos: list[str] = []
    base = str(datos.get("url_base") or "<URL_PUBLICA_HTTPS>").rstrip("/")
    for prueba in datos.get("pruebas", []):
        metodo = str(prueba.get("metodo", "GET")).upper()
        ruta = str(prueba.get("ruta", ""))
        esperado = prueba.get("esperado", "")
        if ruta.startswith("https://"):
            url = ruta
        else:
            url = f"{base}{ruta}"
        extra = ""
        if metodo in ("POST", "PUT", "PATCH"):
            extra = ' -H "Content-Type: application/json" -d @cuerpo_sintetico.json'
        if prueba.get("auth"):
            extra += ' -H "Authorization: Bearer $TOKEN_SINTETICO"'
        comandos.append(f"# esperado: {esperado}\ncurl -sS -o /dev/null -w '%{{http_code}}' -X {metodo} '{url}'{extra}")
    return comandos


# --------------------------------------------------------------------------- #
# INF-150-05 deprecacion
# --------------------------------------------------------------------------- #


def validar_deprecacion(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con la politica de versionado.")
    problemas: list[str] = []
    version = str(datos.get("version_api", ""))
    if not re.fullmatch(r"v[0-9]+", version):
        problemas.append(f"version_api invalida: {version!r}")

    politica = datos.get("politica") or {}
    if politica.get("esquema") != "semver":
        problemas.append("la politica debe declarar esquema semver")
    if not politica.get("ventana_deprecacion"):
        problemas.append("falta la ventana de deprecacion")
    if politica.get("breaking_change") not in ("major", "nueva_version_api"):
        problemas.append("un cambio incompatible exige major o nueva version de API")
    if not politica.get("requiere_rfc") is True:
        problemas.append("todo cambio incompatible exige RFC")
    if not politica.get("detectado_en_ci") is True:
        problemas.append("la deteccion de breaking changes debe ejecutarse en CI")

    return {
        "comando": "deprecacion",
        "version_api": version,
        "politica": politica,
        "problemas": problemas,
        "estado": "POLITICA_VALIDA" if not problemas else "POLITICA_RECHAZADA",
        "advertencia": "Sin cambios en /api/v1; la politica solo se valida en local.",
    }


# --------------------------------------------------------------------------- #
# INF-150-06 alcance
# --------------------------------------------------------------------------- #


def validar_alcance(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con la separacion de alcance.")
    excluidos = {str(x).lower() for x in datos.get("fuera_de_alcance", [])}
    problemas: list[str] = []
    faltantes = [elemento for elemento in FUERA_DE_ALCANCE if elemento not in excluidos]
    if faltantes:
        problemas.append("faltan exclusiones: " + ", ".join(faltantes))
    if datos.get("bloquea_por_mobile") is True:
        problemas.append("el Backend no puede bloquearse por aprobacion de Mobile")
    if datos.get("gate_mobile_requerido") is True:
        problemas.append("Mobile no puede ser gate del API")
    return {
        "comando": "alcance",
        "fuera_de_alcance": sorted(excluidos),
        "problemas": problemas,
        "estado": "ALCANCE_SEPARADO" if not problemas else "ALCANCE_INCORRECTO",
        "advertencia": "Android, Play Store y landing son de otros equipos.",
    }


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="verificar_publicacion_i150.py",
        description="Publicacion y compatibilidad local de la API (I-150). Sin AWS.",
    )
    parser.add_argument(
        "comando",
        help="contrato | publicacion | compatibilidad | evidencia | deprecacion | alcance",
    )
    parser.add_argument("--datos", default=None)
    parser.add_argument("--openapi", default=None)
    parser.add_argument("--salida", default=None)
    parser.add_argument(
        "--comandos",
        action="store_true",
        help="Genera los comandos curl de evidencia sin ejecutarlos",
    )
    return parser


BLOQUEANTES = (
    "CONTRATO_RECHAZADO",
    "PARIDAD_ROTA",
    "CONTENIDO_SIN_DISENO",
    "PUBLICACION_RECHAZADA",
    "MATRIZ_INCOMPATIBLE",
    "PLAN_RECHAZADO",
    "POLITICA_RECHAZADA",
    "ALCANCE_INCORRECTO",
)


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    comando = args.comando.strip().lower()
    datos = cargar_json(args.datos) if args.datos else None

    if comando == "contrato":
        resultado = validar_contrato(ruta=Path(args.openapi) if args.openapi else None)
    elif comando == "publicacion":
        resultado = validar_publicacion(datos)
    elif comando == "compatibilidad":
        resultado = validar_compatibilidad()
    elif comando == "evidencia":
        resultado = validar_evidencia(datos)
    elif comando == "deprecacion":
        resultado = validar_deprecacion(datos)
    elif comando == "alcance":
        resultado = validar_alcance(datos)
    elif comando == "paridad":
        resultado = validar_paridad()
    elif comando == "contenido":
        resultado = escanear_contenido()
    else:
        raise ErrorUso(
            f"Comando desconocido: {comando!r}. Solo existen: contrato, paridad, publicacion, "
            "compatibilidad, evidencia, deprecacion, alcance, contenido."
        )

    if comando == "evidencia" and getattr(args, "comandos", False):
        resultado["comandos"] = generar_comandos(datos or {})

    resultado["aws_consultado"] = False
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
    return (1 if resultado.get("estado") in BLOQUEANTES else 0), resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except (ErrorUso, OSError) as exc:
        print(f"DENEGADO: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
