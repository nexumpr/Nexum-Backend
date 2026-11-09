"""Verificador operativo de Nexum-Backend (I-140).

Contratos cubiertos, todo en local y sin AWS:

- INF-140-01 runbooks: 12 procedimientos con 13 campos y estado no ejecutado.
- INF-140-02 incidentes: S1/S2/S3 con propietario, escalamiento y ciclo completo.
- INF-140-03 drills: 12 simulacros locales con estados LOCAL_PASS / LOCAL_FAIL /
  NOT_RUN_AWS / BLOCKED_DEPENDENCY y sin declarar AWS.
- INF-140-04 aceptacion: matriz con 12 A + 16 I + 75 E.
- INF-140-05 acceso: sin root, sin SSH publico y con identidades temporales.
- INF-140-06 inventario: derivado de fuentes reales con criticidad y estado.

Codigos: 0 correcto, 1 incompleto/rechazado, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from decimal import Decimal
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
CONTROL = RAIZ / "sdd" / "_control" / "H08"
RUNBOOK = CONTROL / "RUNBOOK_OPERATIVO.md"
FIXTURES = RAIZ / "tests" / "fixtures" / "operacion"
INVENTARIO_OPERACION = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"
INVENTARIO_RESPALDO = RAIZ / "infra" / "terraform" / "respaldo" / "inventario_backup.json"
CATALOGO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"

RUNBOOKS_REQUERIDOS = (
    "deploy-rollback",
    "health-fail",
    "instancia-perdida",
    "dynamo-throttle",
    "redis-unavailable",
    "https-certificados-cloudfront",
    "rotacion-secretos-jwt",
    "auth-inesperado",
    "incidente-seguridad-datos",
    "contencion-presupuestal",
    "restauracion-continuidad",
    "eliminacion-iac",
)

CAMPOS_RUNBOOK = (
    "disparador",
    "clasificacion",
    "responsable",
    "prerrequisitos",
    "diagnostico",
    "impacto",
    "mitigacion",
    "autorizacion",
    "escalamiento",
    "rollback",
    "recuperacion",
    "evidencia",
    "criterio-cierre",
)

SEVERIDADES = ("S1", "S2", "S3")
ESCALAMIENTO_OBLIGATORIO = ("S1", "S2")

CICLO_INCIDENTE = (
    "deteccion",
    "registro_auditable",
    "priorizacion",
    "notificacion",
    "escalamiento",
    "contencion",
    "resolucion",
    "cierre",
    "reapertura",
    "evidencia",
)

DRILLS_REQUERIDOS = (
    "caida-instancia-a",
    "continuidad-b",
    "reemplazo-target",
    "redis-no-disponible",
    "dynamo-throttling",
    "timeouts-reintentos",
    "operacion-financiera-parcial",
    "idempotencia",
    "revocacion-rotacion-jwt",
    "health-readiness",
    "perdida-observabilidad",
    "riesgo-rollback",
)

ESTADOS_DRILL = ("LOCAL_PASS", "LOCAL_FAIL", "NOT_RUN_AWS", "BLOCKED_DEPENDENCY")

ACCIONES_PROHIBIDAS = ("delete", "destroy", "deshabilitar", "disable", "drop")

PROPIETARIO_POR_COMPONENTE = {
    "computo": "Backend / Infraestructura",
    "balanceo": "Infraestructura",
    "cache": "Backend",
    "datos": "Backend / Datos",
    "red": "Infraestructura",
    "estado": "Infraestructura",
    "observabilidad": "Operacion / SRE",
    "seguridad": "Seguridad",
}

CUENTAS_ESPERADAS = {"A": 12, "I": 16, "E": 75}


class ErrorUso(Exception):
    """Entrada invalida (codigo 2)."""


def cargar_json(ruta: Path | str) -> Any:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorUso(f"No se puede leer {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorUso(f"{ruta} no es JSON valido: {exc}") from exc


def _secciones(texto: str) -> dict[str, str]:
    secciones: dict[str, list[str]] = {}
    actual = None
    for linea in texto.splitlines():
        coincidencia = re.match(r"^##\s+([A-Za-z0-9_./-]+)\s*$", linea.strip())
        if coincidencia:
            actual = coincidencia.group(1)
            secciones[actual] = []
            continue
        if actual is not None:
            secciones[actual].append(linea)
    return {clave: "\n".join(valor) for clave, valor in secciones.items()}


# --------------------------------------------------------------------------- #
# INF-140-01 runbooks
# --------------------------------------------------------------------------- #


def validar_runbooks(texto: str | None = None, ruta: Path | None = None) -> dict[str, Any]:
    """12 procedimientos con 13 campos; un fichero ausente es error, no fallback."""
    if texto is None:
        objetivo = Path(ruta) if ruta is not None else RUNBOOK
        if not objetivo.is_file():
            raise ErrorUso(f"Falta el runbook {objetivo}")
        texto = objetivo.read_text(encoding="utf-8")

    secciones = _secciones(texto)
    faltantes: dict[str, list[str]] = {}
    for nombre in RUNBOOKS_REQUERIDOS:
        if nombre not in secciones:
            faltantes[nombre] = ["seccion ausente"]
            continue
        cuerpo = secciones[nombre]
        sin_campo = [campo for campo in CAMPOS_RUNBOOK if f"- **{campo}**" not in cuerpo]
        if sin_campo:
            faltantes[nombre] = sin_campo

    presentes = [n for n in RUNBOOKS_REQUERIDOS if n in secciones]
    sin_ejecutar = all("DOCUMENTADO_NO_EJECUTADO" in secciones.get(n, "") for n in presentes)
    prohibidos = [
        n for n in presentes
        if re.search(r"\baws\s+(?:[a-z0-9-]+\s+)?(delete|terminate|destroy|remove|purge)\b", secciones[n], flags=re.IGNORECASE)
    ]
    return {
        "comando": "runbooks",
        "requeridos": len(RUNBOOKS_REQUERIDOS),
        "presentes": len(presentes),
        "campos_por_runbook": len(CAMPOS_RUNBOOK),
        "faltantes": faltantes,
        "comandos_peligrosos": prohibidos,
        "todos_documentados_sin_ejecutar": sin_ejecutar,
        "estado": (
            "RUNBOOKS_COMPLETOS"
            if not faltantes and sin_ejecutar and not prohibidos
            else "RUNBOOKS_INCOMPLETOS"
        ),
        "ejecucion_aws": "NOT_RUN",
        "advertencia": "Un runbook documentado no acredita que se haya ejecutado.",
    }


# --------------------------------------------------------------------------- #
# INF-140-02 incidentes
# --------------------------------------------------------------------------- #


def validar_incidentes(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con la matriz de incidentes.")
    incidentes = datos.get("incidentes")
    if not isinstance(incidentes, list) or not incidentes:
        raise ErrorUso("'incidentes' debe ser una lista no vacia.")

    problemas: list[str] = []
    severidades_vistas: set[str] = set()
    tiempos: dict[str, Any] = {}

    for incidente in incidentes:
        if not isinstance(incidente, Mapping):
            problemas.append("incidente malformado")
            continue
        identificador = str(incidente.get("id"))
        severidad = incidente.get("severidad")
        if severidad not in SEVERIDADES:
            problemas.append(f"{identificador}: severidad desconocida {severidad!r}")
            continue
        severidades_vistas.add(severidad)
        tiempos[severidad] = incidente.get("tiempo_respuesta")

        if not incidente.get("propietario"):
            problemas.append(f"{identificador}: incidente sin propietario")
        for campo in CICLO_INCIDENTE:
            if not incidente.get(campo):
                problemas.append(f"{identificador}: falta {campo}")
        if severidad in ESCALAMIENTO_OBLIGATORIO and not incidente.get("escalamiento"):
            problemas.append(f"{identificador}: {severidad} exige escalamiento")
        if incidente.get("contiene_datos_sensibles") is True:
            problemas.append(f"{identificador}: la matriz no puede contener datos sensibles")
        if incidente.get("evidencia_aws_simulada") is True:
            problemas.append(f"{identificador}: evidencia AWS simulada presentada como real")

    faltan_severidades = [s for s in SEVERIDADES if s not in severidades_vistas]
    if faltan_severidades:
        problemas.append("faltan severidades: " + ", ".join(faltan_severidades))

    # Reglas contradictorias: S1 no puede ser mas lento que S2/S3 con tiempos definidos.
    numericos = {
        nivel: valor for nivel, valor in tiempos.items()
        if isinstance(valor, int) and not isinstance(valor, bool)
    }
    if "S1" in numericos and "S2" in numericos and numericos["S1"] > numericos["S2"]:
        problemas.append("contradiccion: S1 no puede tardar mas que S2")
    if "S2" in numericos and "S3" in numericos and numericos["S2"] > numericos["S3"]:
        problemas.append("contradiccion: S2 no puede tardar mas que S3")

    return {
        "comando": "incidentes",
        "incidentes": len(incidentes),
        "severidades": sorted(severidades_vistas),
        "problemas": problemas,
        "tiempos": tiempos,
        "ratificacion": datos.get("ratificacion", "PENDIENTE_DECISION_HUMANA"),
        "estado": "INCIDENTES_VALIDOS" if not problemas else "INCIDENTES_RECHAZADOS",
        "advertencia": "Los tiempos propuestos no son un SLA ratificado ni activan notificaciones.",
    }


# --------------------------------------------------------------------------- #
# INF-140-03 drills
# --------------------------------------------------------------------------- #


def _evidencia_valida(evidencia: Any) -> tuple[bool, str]:
    """La evidencia local exige archivo y prueba existentes, con resultado PASS."""
    if not isinstance(evidencia, Mapping):
        return False, "sin evidencia verificable"
    archivo = str(evidencia.get("archivo", "")).strip()
    prueba = str(evidencia.get("prueba", "")).strip()
    resultado = str(evidencia.get("resultado", "")).strip().upper()
    if not archivo or not prueba:
        return False, "evidencia incompleta (archivo o prueba)"
    if resultado != "PASS":
        return False, "la evidencia no declara resultado PASS"
    comando = str(evidencia.get("comando", "")).strip()
    if not comando:
        return False, "la evidencia no declara el comando ejecutado en CI"
    ruta = RAIZ / archivo
    if not ruta.is_file():
        return False, f"archivo de evidencia inexistente: {archivo}"
    contenido = ruta.read_text(encoding="utf-8", errors="replace")
    patrones = (
        f"func {prueba}(",
        f"class {prueba}(",
        f"def {prueba}(",
    )
    if not any(patron in contenido for patron in patrones):
        return False, f"la prueba {prueba} no existe en {archivo}"

    # La prueba debe ejecutarse en el CI vigente: el nombre aparece en el workflow.
    workflow = RAIZ / ".github" / "workflows" / "go-backend.yml"
    if not workflow.is_file():
        return False, "no se encuentra el workflow de CI"
    if prueba not in workflow.read_text(encoding="utf-8"):
        return False, f"la prueba {prueba} no se ejecuta en el workflow de CI"
    return True, "verificada contra archivo y workflow de CI"


def validar_drills(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    """12 drills; LOCAL_PASS solo con evidencia de prueba existente y unica."""
    if datos is None:
        raise ErrorUso("Se requiere --datos con el plan de drills.")
    drills = datos.get("drills")
    if not isinstance(drills, list):
        raise ErrorUso("'drills' debe ser una lista.")

    identificadores = {str(d.get("id")) for d in drills if isinstance(d, Mapping)}
    faltantes = [nombre for nombre in DRILLS_REQUERIDOS if nombre not in identificadores]
    problemas: list[str] = []
    mutantes: list[str] = []
    falsos_aws: list[str] = []
    sin_evidencia: list[str] = []
    evidencias_usadas: dict[tuple[str, str], str] = {}
    conteo_estados: dict[str, int] = {}

    for drill in drills:
        if not isinstance(drill, Mapping):
            problemas.append("drill malformado")
            continue
        identificador = str(drill.get("id"))
        for campo in ("componente", "esperado", "criterio", "rollback", "estado"):
            if not drill.get(campo):
                problemas.append(f"{identificador}: falta {campo}")
        estado = drill.get("estado")
        if estado not in ESTADOS_DRILL:
            problemas.append(f"{identificador}: estado desconocido {estado!r}")
        else:
            conteo_estados[estado] = conteo_estados.get(estado, 0) + 1

        if drill.get("demuestra_aws") is True:
            falsos_aws.append(identificador)
        if drill.get("requiere_aws") and estado not in ("NOT_RUN_AWS", "BLOCKED_DEPENDENCY"):
            problemas.append(
                f"{identificador}: un drill con AWS no puede declararse {estado}"
            )
        if drill.get("datos_sinteticos") is not True:
            problemas.append(f"{identificador}: exige datos sinteticos")
        accion = str(drill.get("accion", "")).lower()
        if any(prohibida in accion for prohibida in ACCIONES_PROHIBIDAS):
            mutantes.append(identificador)

        if estado == "LOCAL_PASS":
            valida, motivo = _evidencia_valida(drill.get("evidencia"))
            if not valida:
                sin_evidencia.append(identificador)
                problemas.append(f"{identificador}: LOCAL_PASS sin evidencia valida ({motivo})")
            else:
                evidencia = drill["evidencia"]
                clave = (str(evidencia["archivo"]), str(evidencia["prueba"]))
                if clave in evidencias_usadas:
                    problemas.append(
                        f"{identificador}: evidencia duplicada con {evidencias_usadas[clave]}"
                    )
                else:
                    evidencias_usadas[clave] = identificador
        if estado == "BLOCKED_DEPENDENCY" and not drill.get("dependencia"):
            problemas.append(f"{identificador}: BLOCKED_DEPENDENCY exige 'dependencia'")

    return {
        "comando": "drills",
        "requeridos": len(DRILLS_REQUERIDOS),
        "presentes": len(identificadores),
        "faltantes": faltantes,
        "problemas": problemas,
        "drills_mutantes": mutantes,
        "declaran_aws_sin_evidencia": falsos_aws,
        "sin_evidencia_local": sin_evidencia,
        "evidencias_distintas": len(evidencias_usadas),
        "conteo_estados": conteo_estados,
        "estado": (
            "PLAN_VALIDO"
            if not faltantes and not problemas and not mutantes and not falsos_aws
            else "PLAN_RECHAZADO"
        ),
        "ejecucion_aws": "NOT_RUN",
        "advertencia": (
            "LocalPASS exige archivo y prueba reales; un drill local no demuestra dos EC2 "
            "reales, DynamoDB persistente ni Redis distribuido."
        ),
    }


# --------------------------------------------------------------------------- #
# INF-140-04 aceptacion
# --------------------------------------------------------------------------- #


def _ids_por_directorio(patron: str, base: Path) -> list[str]:
    """Extrae el identificador de dos segmentos (A-000, I-130) de cada directorio."""
    identificadores = []
    for ruta in base.glob(patron):
        if not ruta.is_dir():
            continue
        partes = ruta.name.split("-")
        if len(partes) >= 2:
            identificadores.append(f"{partes[0]}-{partes[1]}")
    return sorted(identificadores)


def inventario_sdd() -> dict[str, list[str]]:
    arquitectura = _ids_por_directorio("A-*", RAIZ / "sdd" / "01-arquitectura-general")
    infraestructura = _ids_por_directorio("I-*", RAIZ / "sdd" / "02-infraestructura")
    endpoints = sorted(
        ruta.name.split("-")[0]
        for ruta in (RAIZ / "sdd" / "03-endpoints").iterdir()
        if ruta.is_dir() and ruta.name.startswith("E0")
    )
    return {"A": arquitectura, "I": infraestructura, "E": sorted(set(endpoints))}


def validar_matriz(texto: str | None = None, ruta: Path | None = None) -> dict[str, Any]:
    if texto is None:
        objetivo = Path(ruta) if ruta is not None else CONTROL / "MATRIZ_INF_140.md"
        if not objetivo.is_file():
            raise ErrorUso(f"Falta la matriz {objetivo}")
        texto = objetivo.read_text(encoding="utf-8")

    reales = inventario_sdd()
    faltantes: dict[str, list[str]] = {}
    for clave, esperados in CUENTAS_ESPERADAS.items():
        ausentes = [identificador for identificador in reales[clave] if identificador not in texto]
        if ausentes:
            faltantes[clave] = ausentes

    conteos = {clave: len(reales[clave]) for clave in reales}
    discrepancias = {
        clave: {"esperado": CUENTAS_ESPERADAS[clave], "real": conteos[clave]}
        for clave in CUENTAS_ESPERADAS
        if conteos[clave] != CUENTAS_ESPERADAS[clave]
    }
    return {
        "comando": "aceptacion",
        "conteos": conteos,
        "faltantes_en_matriz": faltantes,
        "discrepancias_conteo": discrepancias,
        "estado": (
            "MATRIZ_COMPLETA"
            if not faltantes and not discrepancias
            else "MATRIZ_INCOMPLETA"
        ),
        "advertencia": (
            "Los contratos E001-E075 son OpenAPI; no demuestran handlers funcionales ni "
            "verificacion HTTP real."
        ),
    }


# --------------------------------------------------------------------------- #
# INF-140-05 acceso
# --------------------------------------------------------------------------- #


def validar_acceso(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con la politica de acceso.")
    problemas: list[str] = []
    if datos.get("shell_root") is True:
        problemas.append("shell de root prohibido")
    if datos.get("ssh_publico") is True:
        problemas.append("SSH publico prohibido")
    if datos.get("identidades_temporales") is not True:
        problemas.append("se exigen identidades temporales (SSO/OIDC)")
    if datos.get("claves_estaticas") is True:
        problemas.append("no se admiten claves de acceso estaticas")
    if datos.get("cambio_scp") is True:
        problemas.append("cambiar la SCP requiere RFC y Gate")
    if datos.get("ssm_solo_en_ventana") is not True:
        problemas.append("SSM debe limitarse a la ventana autorizada")
    if datos.get("minimo_privilegio") is not True:
        problemas.append("se exige minimo privilegio documentado")
    if datos.get("auditoria") is not True:
        problemas.append("se exige auditoria de accesos")
    return {
        "comando": "acceso",
        "estado": "ACCESO_CONFORME" if not problemas else "ACCESO_RECHAZADO",
        "problemas": problemas,
        "advertencia": "Politica declarativa; no se valido ningun acceso real.",
    }


# --------------------------------------------------------------------------- #
# INF-140-06 inventario
# --------------------------------------------------------------------------- #


ALIAS_RECURSOS = {
    "terraform-state": "terraform-state-s3",
    "secretos-bootstrap": "secretsmanager-secretos",
}

ESTADOS_PRECIO_VALIDOS = ("VERIFIED_AWS", "DOC_OFICIAL", "DOC_ONLY", "SUPUESTO")


def consolidar_partidas(partidas: list[Mapping[str, Any]]) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    """Una sola contabilizacion por (sku, base); cantidades distintas son conflicto."""
    unicas: list[dict[str, Any]] = []
    conflictos: list[dict[str, Any]] = []
    vistas: dict[tuple[str, str], Mapping[str, Any]] = {}
    for partida in partidas:
        clave = (str(partida.get("sku")), str(partida.get("base")))
        previa = vistas.get(clave)
        if previa is None:
            vistas[clave] = partida
            unicas.append(dict(partida))
            continue
        if str(previa.get("cantidad")) != str(partida.get("cantidad")):
            conflictos.append(
                {
                    "sku": clave[0],
                    "base": clave[1],
                    "cantidad_previa": str(previa.get("cantidad")),
                    "cantidad_nueva": str(partida.get("cantidad")),
                }
            )
    return unicas, conflictos


def _precio(catalogo: Mapping[str, Any], sku: str) -> tuple[Decimal | None, str]:
    """(precio, estado). Un precio ausente devuelve None, nunca 0."""
    datos = catalogo.get("items", {}).get(sku, {})
    valor = datos.get("precio_usd")
    estado = str(datos.get("estado", "DESCONOCIDO"))
    if valor is None:
        return None, "DESCONOCIDO"
    return Decimal(str(valor)), estado


def costos_activo(activo: Mapping[str, Any], catalogo: Mapping[str, Any]) -> dict[str, Any]:
    """Coste por hora y por mes, con unidades separadas y sin precios inventados."""
    hora = Decimal("0")
    mes = Decimal("0")
    desconocidos: list[str] = []
    partidas: list[dict[str, Any]] = []
    for partida in activo.get("partidas", []) or []:
        base = partida.get("base")
        cantidad = partida.get("cantidad")
        sku = str(partida.get("sku"))
        if base == "variable" or cantidad is None:
            partidas.append({"sku": sku, "base": base, "importe": None, "estado": "VARIABLE"})
            continue
        precio, estado = _precio(catalogo, sku)
        if precio is None:
            desconocidos.append(sku)
            partidas.append({"sku": sku, "base": base, "importe": None, "estado": "DESCONOCIDO"})
            continue
        importe = precio * Decimal(str(cantidad))
        if base == "hora":
            hora += importe
            partidas.append(
                {"sku": sku, "base": base, "importe": str(importe.quantize(Decimal("0.0001"))), "estado": estado}
            )
        elif base == "mes":
            mes += importe
            hora += importe / Decimal("730")
            partidas.append(
                {"sku": sku, "base": base, "importe": str(importe.quantize(Decimal("0.0001"))), "estado": estado}
            )
        else:
            partidas.append({"sku": sku, "base": base, "importe": None, "estado": estado})

    # Si falta un precio, el total no se declara parcial: se anula.
    total_hora = None if desconocidos else hora
    total_mes = None if desconocidos else mes
    return {
        "costo_hora_usd": None if total_hora is None else str(total_hora.quantize(Decimal("0.0001"))),
        "costo_mes_usd": None if total_mes is None else str(total_mes.quantize(Decimal("0.0001"))),
        "precios_desconocidos": sorted(set(desconocidos)),
        "partidas": partidas,
    }


def capacidad_asg_declarada(raiz: Path | None = None) -> dict[str, Any]:
    """Lee la capacidad del ASG en Terraform; no afirma instancias reales."""
    modulo = (raiz or RAIZ / "infra" / "terraform" / "modules" / "computo") / "variables.tf"
    main = modulo.parent / "main.tf"
    if not modulo.is_file() or not main.is_file():
        return {"declarada": None, "ok": False, "motivo": "modulo de computo ausente"}
    variables = modulo.read_text(encoding="utf-8")
    principal = main.read_text(encoding="utf-8")
    bloque = re.search(r'variable\s+"asg_capacidad"\s*\{.*?\n\}', variables, re.S)
    defecto = re.search(r"default\s*=\s*(\d+)", bloque.group(0)) if bloque else None
    valida = re.search(r"var\.asg_capacidad\s*==\s*(\d+)", principal)
    referencias = all(
        re.search(rf"\b{campo}\s*=\s*var\.asg_capacidad\b", principal) is not None
        for campo in ("min_size", "desired_capacity", "max_size")
    )
    valor = int(valor_ := defecto.group(1)) if defecto else None
    exigido = int(valida.group(1)) if valida else None
    ok = bool(valor is not None and valor == 2 and exigido == 2 and referencias)
    return {
        "declarada": valor,
        "exigida_por_validacion": exigido,
        "min_desired_max_ligados": referencias,
        "ok": ok,
        "motivo": (
            "el ASG declara min=desired=max=2"
            if ok
            else "el ASG no declara de forma verificable dos instancias"
        ),
    }


def inventario_infraestructura() -> dict[str, Any]:
    """Inventario consolidado de I-120 e I-130. Sin AWS y sin duplicar costos."""
    if not INVENTARIO_OPERACION.is_file() or not INVENTARIO_RESPALDO.is_file():
        raise ErrorUso("Faltan los inventarios de I-120 o I-130.")
    operacion = cargar_json(INVENTARIO_OPERACION)
    respaldo = cargar_json(INVENTARIO_RESPALDO)
    catalogo = cargar_json(CATALOGO)

    consolidado: dict[str, dict[str, Any]] = {}
    duplicados: list[dict[str, str]] = []

    def registrar(nombre: str, datos: dict[str, Any], fuente: str) -> None:
        clave = ALIAS_RECURSOS.get(nombre, nombre)
        existente = consolidado.get(clave)
        if existente is None:
            consolidado[clave] = {**datos, "nombre": clave, "fuentes": [fuente]}
            return
        duplicados.append({"nombre": nombre, "consolidado_en": clave})
        existente["fuentes"] = sorted(set(existente["fuentes"] + [fuente]))
        existente["dependencias"] = sorted(
            set(existente["dependencias"]) | set(datos["dependencias"])
        )
        existente["etiquetas"] = {**existente["etiquetas"], **datos["etiquetas"]}
        # Se conservan las fuentes, pero cada cargo se contabiliza una sola vez.
        existente["partidas"] = existente["partidas"] + datos["partidas"]
        # El estado mas restrictivo gana.
        orden = ["DECLARADO_NO_CREADO", "OPCIONAL_NO_CREADO", "BLOQUEADO_RFC004"]
        if orden.index(datos["estado_declarado"]) > orden.index(existente["estado_declarado"]):
            existente["estado_declarado"] = datos["estado_declarado"]
        if existente["owner"] == "PENDIENTE_DECLARACION" and datos["owner"] != "PENDIENTE_DECLARACION":
            existente["owner"] = datos["owner"]

    for activo in operacion["items"]:
        registrar(
            activo["nombre"],
            {
                "tipo": activo["tipo"],
                "componente": activo["componente"],
                "owner": PROPIETARIO_POR_COMPONENTE.get(activo["componente"], "PENDIENTE_DECLARACION"),
                "criticidad": "CORE_24X7" if activo["criticidad"] == "core" else "AUXILIAR_AUTORIZABLE",
                "dependencias": activo.get("dependencias", []),
                "etiquetas": {"nexum:criticality": activo["criticidad"]},
                "partidas": activo.get("partidas", []) or [],
                "estado_declarado": activo["estado_recurso"],
            },
            "infra/terraform/operacion/inventario.json (I-120)",
        )

    for activo in respaldo["activos"]:
        componente = "datos" if "dynamodb" in activo["tipo"] else (
            "seguridad" if "secretsmanager" in activo["tipo"] else "estado"
        )
        registrar(
            activo["nombre"],
            {
                "tipo": activo["tipo"],
                "componente": componente,
                "owner": PROPIETARIO_POR_COMPONENTE.get(componente, "PENDIENTE_DECLARACION"),
                "criticidad": "CORE_24X7",
                "dependencias": activo.get("dependencias", []),
                "etiquetas": {"nexum:criticality": "core"},
                "partidas": activo.get("partidas", []) or [],
                "estado_declarado": activo["estado"],
            },
            "infra/terraform/respaldo/inventario_backup.json (I-130)",
        )

    recursos: list[dict[str, Any]] = []
    precios_desconocidos: set[str] = set()
    conflictos_partidas: list[dict[str, Any]] = []
    for clave in sorted(consolidado):
        datos = consolidado[clave]
        partidas_unicas, conflictos = consolidar_partidas(datos["partidas"])
        for conflicto in conflictos:
            conflictos_partidas.append({"recurso": clave, **conflicto})
        datos = {**datos, "partidas": partidas_unicas}
        costos = costos_activo(datos, catalogo)
        precios_desconocidos.update(costos["precios_desconocidos"])
        recursos.append(
            {
                "nombre": clave,
                "tipo": datos["tipo"],
                "fuentes": datos["fuentes"],
                "componente": datos["componente"],
                "owner": datos["owner"],
                "criticidad": datos["criticidad"],
                "dependencias": sorted(datos["dependencias"]),
                "etiquetas": datos["etiquetas"],
                "costo_hora_usd": costos["costo_hora_usd"],
                "costo_mes_usd": costos["costo_mes_usd"],
                "partidas": costos["partidas"],
                "estado_declarado": datos["estado_declarado"],
                "verificado_aws": False,
            }
        )

    asg = capacidad_asg_declarada()
    problemas: list[str] = []
    core = [r for r in recursos if r["criticidad"] == "CORE_24X7"]
    if not any(r["tipo"] == "aws_autoscaling_group" for r in core):
        problemas.append("no hay ASG core en el inventario")
    if not asg["ok"]:
        problemas.append(asg["motivo"])
    if any(
        r["criticidad"] == "AUXILIAR_AUTORIZABLE" and "redis" in r["nombre"].lower() for r in recursos
    ):
        problemas.append("Redis no puede clasificarse como auxiliar")
    for recurso in recursos:
        if recurso["verificado_aws"]:
            problemas.append(f"{recurso['nombre']}: no puede estar verificado en AWS en local")
    if precios_desconocidos:
        problemas.append(
            "precios no verificados: " + ", ".join(sorted(precios_desconocidos))
        )
    if conflictos_partidas:
        problemas.append(
            "partidas contradictorias entre fuentes: "
            + "; ".join(
                f"{c['recurso']}/{c['sku']} {c['cantidad_previa']} vs {c['cantidad_nueva']}"
                for c in conflictos_partidas
            )
        )

    return {
        "comando": "inventario",
        "recursos": recursos,
        "total": len(recursos),
        "core_24x7": len(core),
        "auxiliares": len([r for r in recursos if r["criticidad"] == "AUXILIAR_AUTORIZABLE"]),
        "consolidados": duplicados,
        "owners_pendientes": sorted(
            r["nombre"] for r in recursos if r["owner"] == "PENDIENTE_DECLARACION"
        ),
        "precios_desconocidos": sorted(precios_desconocidos),
        "conflictos_partidas": conflictos_partidas,
        "capacidad_asg": asg,
        "problemas": problemas,
        "estado": "INVENTARIO_TRAZABLE" if not problemas else "INVENTARIO_INCOMPLETO",
        "existe_en_aws": "NOT_VERIFIED",
        "costo_observado": "NOT_VERIFIED",
        "advertencia": (
            "Costos estimados por existencia con precios del catalogo; un recurso declarado no "
            "equivale a uno existente en AWS y AWS_COST_REAL sigue NOT_VERIFIED."
        ),
        "aws_consultado": False,
    }


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="verificar_operacion_i140.py",
        description="Verificacion operativa de I-140. Sin AWS.",
    )
    parser.add_argument(
        "comando", help="runbooks | incidentes | drills | aceptacion | acceso | inventario"
    )
    parser.add_argument("--datos", default=None)
    parser.add_argument("--runbook", default=str(RUNBOOK))
    parser.add_argument("--salida", default=None)
    return parser


BLOQUEANTES = (
    "RUNBOOKS_INCOMPLETOS",
    "INCIDENTES_RECHAZADOS",
    "PLAN_RECHAZADO",
    "MATRIZ_INCOMPLETA",
    "ACCESO_RECHAZADO",
    "INVENTARIO_INCOMPLETO",
)


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    comando = args.comando.strip().lower()
    datos = cargar_json(args.datos) if args.datos else None

    if comando == "runbooks":
        resultado = validar_runbooks(ruta=Path(args.runbook))
    elif comando == "incidentes":
        resultado = validar_incidentes(datos)
    elif comando == "drills":
        resultado = validar_drills(datos)
    elif comando == "aceptacion":
        resultado = validar_matriz()
    elif comando == "acceso":
        resultado = validar_acceso(datos)
    elif comando == "inventario":
        resultado = inventario_infraestructura()
    else:
        raise ErrorUso(
            f"Comando desconocido: {comando!r}. Solo existen: runbooks, incidentes, drills, "
            "aceptacion, acceso, inventario."
        )

    resultado.setdefault("aws_consultado", False)
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
