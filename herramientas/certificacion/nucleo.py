"""Núcleo del harness de certificación funcional 75/75 de Nexum-Backend.

Este módulo no envía peticiones por sí mismo salvo que se le inyecte el transporte
real. Contiene las piezas verificables de forma unitaria:

* inventario derivado del OpenAPI (sin contadores escritos a mano),
* estados de certificación y cálculo del código de salida,
* validador de JSON Schema 3.1 (subconjunto usado por el contrato),
* puerta única de autorización de mutaciones,
* cliente HTTP con HTTPS obligatorio, sin seguir redirecciones a otro origen,
* registro de secretos y auditoría para que ningún resultado los contenga,
* reanudación de una ejecución anterior sin aceptar fallos heredados.

Reglas de seguridad que este módulo hace cumplir:

1. Ningún token se acepta por argumentos de línea de comandos: se leen de variables
   de entorno o de entrada interactiva sin eco (`getpass`).
2. Ningún resultado puede contener contraseñas, tokens ni correos; se redactan y,
   antes de escribir, se audita el documento completo y se falla cerrado.
3. Toda petición declara el E-ID que ejercita. La mutación se decide con el registro
   del escenario, no con lo que el llamador afirme, de modo que no existe una segunda
   puerta que pueda saltarse `--allow-mutations`.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from datetime import date, datetime, timezone
from decimal import Decimal, InvalidOperation
from getpass import getpass
from typing import Any, Callable, Iterable, Mapping

VERSION_RESULTADOS = 2

# --------------------------------------------------------------------------- #
# Estados
# --------------------------------------------------------------------------- #

PASS = "PASS"
FAIL = "FAIL"
NOT_RUN = "NOT_RUN"
BLOCKED_EXTERNAL = "BLOCKED_EXTERNAL"
BLOCKED_DEPENDENCY = "BLOCKED_DEPENDENCY"
CONTRACT_MISMATCH = "CONTRACT_MISMATCH"
CONTENT_MISMATCH = "CONTENT_MISMATCH"

ESTADOS = (
    PASS,
    FAIL,
    NOT_RUN,
    BLOCKED_EXTERNAL,
    BLOCKED_DEPENDENCY,
    CONTRACT_MISMATCH,
    CONTENT_MISMATCH,
)

ESTADOS_FALLO = frozenset({FAIL, CONTRACT_MISMATCH, CONTENT_MISMATCH})
ESTADOS_BLOQUEO = frozenset({BLOCKED_EXTERNAL, BLOCKED_DEPENDENCY})

# Prioridad para combinar varios checks en un único estado. Un fallo real pesa más
# que una violación de contrato, que a su vez pesa más que una discrepancia de
# contenido; los bloqueos nunca tapan un fallo.
PRIORIDAD = {
    PASS: 0,
    NOT_RUN: 1,
    BLOCKED_DEPENDENCY: 2,
    BLOCKED_EXTERNAL: 3,
    CONTENT_MISMATCH: 4,
    CONTRACT_MISMATCH: 5,
    FAIL: 6,
}

MOTIVOS_JUSTIFICADOS = frozenset({"FUERA_DE_FASE", "FUERA_DE_SELECCION", "NO_EJECUTADO_DRY_RUN"})

METODOS = frozenset({"GET", "POST", "PUT", "PATCH", "DELETE"})


class ErrorCertificacion(Exception):
    """Error de configuración o de uso del harness (nunca de un endpoint)."""


class ConfiguracionInvalida(ErrorCertificacion):
    """La configuración no permite certificar de forma segura."""


class MutacionDenegada(ErrorCertificacion):
    """Una petición mutante se intentó sin la autorización explícita."""


class SolicitudInvalida(ErrorCertificacion):
    """El payload que el harness iba a enviar no cumple el contrato OpenAPI."""


class FugaDeSecreto(ErrorCertificacion):
    """El documento de resultados contendría un secreto; se aborta la escritura."""


class InventarioInvalido(ErrorCertificacion):
    """El OpenAPI o el registro de escenarios no permiten una certificación 75/75."""


# --------------------------------------------------------------------------- #
# OpenAPI e inventario
# --------------------------------------------------------------------------- #

ID_OPERACION = re.compile(r"^nexum_e(\d{3})$")


@dataclass(frozen=True)
class Operacion:
    eid: str
    metodo: str
    ruta: str
    idempotencia: bool = False
    seguridad: bool = False
    solicitud: Mapping[str, Any] | None = None
    respuestas: Mapping[str, Any] = field(default_factory=dict)

    def dict(self) -> dict[str, Any]:
        return {"eid": self.eid, "metodo": self.metodo, "ruta": self.ruta}


def cargar_openapi(ruta: str | os.PathLike[str]) -> dict[str, Any]:
    with open(ruta, encoding="utf-8") as fh:
        documento = json.load(fh)
    if not isinstance(documento, dict):
        raise InventarioInvalido("el OpenAPI no es un objeto JSON")
    if not str(documento.get("openapi", "")).startswith("3.1"):
        raise InventarioInvalido("se exige OpenAPI 3.1")
    if not isinstance(documento.get("paths"), dict) or not documento["paths"]:
        raise InventarioInvalido("el OpenAPI no declara paths")
    return documento


def extraer_operaciones(openapi: Mapping[str, Any]) -> dict[str, Operacion]:
    """Deriva las operaciones del contrato. Nunca usa un total escrito a mano."""
    operaciones: dict[str, Operacion] = {}
    problemas: list[str] = []
    for ruta, item in openapi.get("paths", {}).items():
        if not isinstance(ruta, str) or not ruta.startswith("/api/v1/"):
            problemas.append(f"ruta fuera de /api/v1: {ruta!r}")
            continue
        if not isinstance(item, dict):
            problemas.append(f"path malformado: {ruta!r}")
            continue
        for metodo, op in item.items():
            if metodo.upper() not in METODOS:
                continue
            if not isinstance(op, dict):
                problemas.append(f"operación malformada: {metodo.upper()} {ruta}")
                continue
            oid = op.get("operationId")
            if not isinstance(oid, str) or not ID_OPERACION.match(oid):
                problemas.append(f"operationId inválido en {metodo.upper()} {ruta}: {oid!r}")
                continue
            eid = oid.replace("nexum_", "").upper()
            if eid in operaciones:
                problemas.append(f"operationId duplicado: {oid}")
                continue
            cabeceras = [p.get("name") for p in op.get("parameters", []) if isinstance(p, dict) and p.get("in") == "header"]
            cuerpo = None
            rb = op.get("requestBody")
            if isinstance(rb, dict):
                for _ct, media in (rb.get("content") or {}).items():
                    if isinstance(media, dict):
                        cuerpo = media.get("schema")
                        break
            seguridad = bool(op.get("security", openapi.get("security")))
            operaciones[eid] = Operacion(
                eid=eid,
                metodo=metodo.upper(),
                ruta=ruta,
                idempotencia="Idempotency-Key" in cabeceras,
                seguridad=seguridad,
                solicitud=cuerpo,
                respuestas=op.get("responses") or {},
            )
    if problemas:
        raise InventarioInvalido("; ".join(problemas))
    if not operaciones:
        raise InventarioInvalido("el OpenAPI no contiene operaciones nexum_eNNN")
    return operaciones


def digest_inventario(operaciones: Mapping[str, Operacion]) -> str:
    material = "\n".join(
        f"{eid}|{op.metodo}|{op.ruta}" for eid, op in sorted(operaciones.items())
    )
    return hashlib.sha256(material.encode("utf-8")).hexdigest()


def _sin_query(ruta: str) -> str:
    return ruta.split("?", 1)[0]


def ruta_coincide(plantilla: str, ruta: str) -> bool:
    """Compara la ruta con la plantilla OpenAPI; los `{param}` aceptan un segmento."""
    partes_plantilla = _sin_query(plantilla).strip("/").split("/")
    partes_ruta = _sin_query(ruta).strip("/").split("/")
    if len(partes_plantilla) != len(partes_ruta):
        return False
    for esperada, real in zip(partes_plantilla, partes_ruta):
        if esperada.startswith("{") and esperada.endswith("}"):
            if not real:
                return False
            continue
        if esperada != real:
            return False
    return True


@dataclass
class InformeRegistro:
    faltantes: list[str] = field(default_factory=list)
    sobrantes: list[str] = field(default_factory=list)
    discrepancias: list[str] = field(default_factory=list)
    duplicados: list[str] = field(default_factory=list)

    @property
    def ok(self) -> bool:
        return not (self.faltantes or self.sobrantes or self.discrepancias or self.duplicados)

    def problemas(self) -> list[str]:
        salida = []
        if self.faltantes:
            salida.append("escenarios ausentes: " + ", ".join(sorted(self.faltantes)))
        if self.sobrantes:
            salida.append("escenarios sin operación: " + ", ".join(sorted(self.sobrantes)))
        if self.discrepancias:
            salida.extend(self.discrepancias)
        if self.duplicados:
            salida.append("E-ID duplicados en el orden: " + ", ".join(sorted(self.duplicados)))
        return salida


def verificar_registro(
    operaciones: Mapping[str, Operacion],
    escenarios: Mapping[str, Any],
    orden: Iterable[str],
) -> InformeRegistro:
    """Exige exactamente un escenario por operación y método/ruta coincidentes."""
    informe = InformeRegistro()
    informe.faltantes = sorted(set(operaciones) - set(escenarios))
    informe.sobrantes = sorted(set(escenarios) - set(operaciones))
    vistos: set[str] = set()
    for eid in orden:
        if eid in vistos:
            informe.duplicados.append(eid)
        vistos.add(eid)
    for eid in sorted(set(operaciones) & set(escenarios)):
        op = operaciones[eid]
        esc = escenarios[eid]
        if getattr(esc, "metodo", None) != op.metodo:
            informe.discrepancias.append(
                f"{eid}: método del escenario {getattr(esc, 'metodo', None)} != OpenAPI {op.metodo}"
            )
        if getattr(esc, "ruta", None) != op.ruta:
            informe.discrepancias.append(
                f"{eid}: ruta del escenario {getattr(esc, 'ruta', None)} != OpenAPI {op.ruta}"
            )
    faltan_en_orden = sorted(set(operaciones) - vistos)
    if faltan_en_orden:
        informe.discrepancias.append("operaciones sin orden de ejecución: " + ", ".join(faltan_en_orden))
    extra_en_orden = sorted(vistos - set(operaciones))
    if extra_en_orden:
        informe.discrepancias.append("orden con E-ID inexistentes: " + ", ".join(extra_en_orden))
    return informe


# --------------------------------------------------------------------------- #
# Resultados
# --------------------------------------------------------------------------- #

class Resultado:
    """Resultado de un E-ID. Solo se cierra cuando el orquestador lo pide."""

    def __init__(self, eid: str, titulo: str, metodo: str = "", ruta: str = ""):
        self.eid = eid
        self.titulo = titulo
        self.metodo = metodo
        self.ruta = ruta
        self.checks: list[dict[str, Any]] = []
        self.motivo = ""
        self.origen = "ejecutado"
        self.heredado_de: dict[str, Any] = {}
        self.cerrado = False

    # -- registro de checks -------------------------------------------------- #

    def ok(self, nombre: str, detalle: str = "") -> bool:
        self.checks.append({"check": nombre, "ok": True, "estado": PASS, "detalle": detalle[:300]})
        return True

    def fallo(self, nombre: str, detalle: str = "", estado: str = FAIL) -> bool:
        if estado not in ESTADOS_FALLO:
            raise ValueError(f"estado de fallo inválido: {estado}")
        self.checks.append({"check": nombre, "ok": False, "estado": estado, "detalle": detalle[:300]})
        return False

    def bloqueo(self, nombre: str, detalle: str = "", estado: str = BLOCKED_EXTERNAL) -> bool:
        if estado not in ESTADOS_BLOQUEO:
            raise ValueError(f"estado de bloqueo inválido: {estado}")
        self.checks.append({"check": nombre, "ok": False, "estado": estado, "detalle": detalle[:300]})
        return False

    def bloqueado(self) -> bool:
        return any(c["estado"] in ESTADOS_BLOQUEO for c in self.checks)

    # -- cierre -------------------------------------------------------------- #

    def estado_final(self) -> str:
        if not self.checks:
            return NOT_RUN
        peor = PASS
        for check in self.checks:
            estado = check["estado"]
            if PRIORIDAD[estado] > PRIORIDAD[peor]:
                peor = estado
        return peor

    def cerrar(self) -> str:
        self.cerrado = True
        return self.estado_final()

    def justificado(self) -> bool:
        return self.estado_final() != NOT_RUN or self.motivo in MOTIVOS_JUSTIFICADOS

    def fallos(self) -> list[str]:
        return [c["check"] for c in self.checks if c["estado"] in ESTADOS_FALLO]

    def dict(self) -> dict[str, Any]:
        documento = {
            "eid": self.eid,
            "titulo": self.titulo,
            "metodo": self.metodo,
            "ruta": self.ruta,
            "estado": self.estado_final(),
            "motivo": self.motivo,
            "origen": self.origen,
            "checks": self.checks,
        }
        if self.heredado_de:
            documento["heredado_de"] = self.heredado_de
        return documento


# --------------------------------------------------------------------------- #
# Validador JSON Schema (subconjunto OpenAPI 3.1)
# --------------------------------------------------------------------------- #

RE_UUID = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
RE_EMAIL = re.compile(r"^[^@\s]+@[^@\s]+\.[^@\s]+$")
RE_FECHA = re.compile(r"^\d{4}-\d{2}-\d{2}$")
INT64_MIN = -(2**63)
INT64_MAX = 2**63 - 1


def _mismo_valor(a: Any, b: Any) -> bool:
    """Igualdad tipo-consciente: en Python ``True == 1``, en JSON no."""
    if isinstance(a, bool) or isinstance(b, bool):
        return isinstance(a, bool) and isinstance(b, bool) and a == b
    if isinstance(a, (int, float)) and isinstance(b, (int, float)):
        return a == b
    return type(a) is type(b) and a == b


def _tipo_json(valor: Any) -> str:
    if valor is None:
        return "null"
    if isinstance(valor, bool):
        return "boolean"
    if isinstance(valor, int):
        return "integer"
    if isinstance(valor, float):
        return "number"
    if isinstance(valor, str):
        return "string"
    if isinstance(valor, list):
        return "array"
    if isinstance(valor, dict):
        return "object"
    return "desconocido"


def _fecha_hora_valida(valor: str) -> bool:
    texto = valor.strip()
    if not texto:
        return False
    if texto.endswith("Z"):
        texto = texto[:-1] + "+00:00"
    try:
        instante = datetime.fromisoformat(texto)
    except ValueError:
        return False
    return instante.tzinfo is not None


class ValidadorEsquema:
    """Valida instancias contra los schemas reales del OpenAPI 3.1."""

    def __init__(self, openapi: Mapping[str, Any], operaciones: Mapping[str, Operacion]):
        self.openapi = openapi
        self.schemas: Mapping[str, Any] = (openapi.get("components") or {}).get("schemas") or {}
        self.operaciones = operaciones

    # -- helpers ------------------------------------------------------------- #

    def resolver(self, schema: Any, profundidad: int = 0) -> Any:
        if profundidad > 30 or not isinstance(schema, dict):
            return schema
        ref = schema.get("$ref")
        if isinstance(ref, str):
            if not ref.startswith("#/components/schemas/"):
                raise InventarioInvalido(f"$ref no local no soportado: {ref}")
            nombre = ref.rsplit("/", 1)[-1]
            if nombre not in self.schemas:
                raise InventarioInvalido(f"$ref irresoluble: {ref}")
            return self.resolver(self.schemas[nombre], profundidad + 1)
        return schema

    def validar(self, valor: Any, schema: Any, camino: str = "$") -> list[str]:
        schema = self.resolver(schema)
        if schema is True or schema == {}:
            return []
        if schema is False:
            return [f"{camino}: schema prohibido (false)"]
        if not isinstance(schema, dict):
            return [f"{camino}: schema inválido"]
        errores: list[str] = []

        if schema.get("nullable") is True and valor is None:
            return []

        if "anyOf" in schema:
            intentos = [self.validar(valor, sub, camino) for sub in schema["anyOf"]]
            if not any(not e for e in intentos):
                errores.append(f"{camino}: no cumple ninguna alternativa anyOf")
            return errores

        if "oneOf" in schema:
            intentos = [self.validar(valor, sub, camino) for sub in schema["oneOf"]]
            if sum(1 for e in intentos if not e) != 1:
                errores.append(f"{camino}: no cumple exactamente una alternativa oneOf")
            return errores

        if "allOf" in schema:
            for sub in schema["allOf"]:
                errores.extend(self.validar(valor, sub, camino))
            return errores

        tipo = schema.get("type")
        if tipo is not None:
            permitidos = tipo if isinstance(tipo, list) else [tipo]
            real = _tipo_json(valor)
            valido = real in permitidos
            if real == "integer" and "number" in permitidos:
                valido = True
            if real == "number" and "integer" in permitidos:
                valido = float(valor).is_integer()
            if not valido:
                errores.append(f"{camino}: tipo {real} no permitido {permitidos}")
                return errores

        if "enum" in schema:
            if not any(_mismo_valor(valor, opcion) for opcion in schema["enum"]):
                errores.append(f"{camino}: valor fuera de enum")
                return errores

        if isinstance(valor, str):
            if "minLength" in schema and len(valor) < int(schema["minLength"]):
                errores.append(f"{camino}: longitud menor que minLength")
            if "maxLength" in schema and len(valor) > int(schema["maxLength"]):
                errores.append(f"{camino}: longitud mayor que maxLength")
            if "pattern" in schema:
                try:
                    if re.search(str(schema["pattern"]), valor) is None:
                        errores.append(f"{camino}: no cumple pattern")
                except re.error as exc:
                    errores.append(f"{camino}: pattern inválido ({exc})")
            formato = schema.get("format")
            if formato == "uuid" and not RE_UUID.match(valor):
                errores.append(f"{camino}: uuid inválido")
            elif formato == "date-time" and not _fecha_hora_valida(valor):
                errores.append(f"{camino}: date-time inválido")
            elif formato == "date":
                if not RE_FECHA.match(valor):
                    errores.append(f"{camino}: date inválido")
                else:
                    try:
                        date.fromisoformat(valor)
                    except ValueError:
                        errores.append(f"{camino}: date inexistente")
            elif formato == "email" and not RE_EMAIL.match(valor):
                errores.append(f"{camino}: email inválido")
            elif formato == "uri":
                partes = urllib.parse.urlparse(valor)
                if partes.scheme not in ("http", "https") or not partes.netloc:
                    errores.append(f"{camino}: uri inválida")
            elif formato == "int64":
                errores.append(f"{camino}: format int64 sobre string")

        if isinstance(valor, (int, float)) and not isinstance(valor, bool):
            if "minimum" in schema and valor < schema["minimum"]:
                errores.append(f"{camino}: menor que minimum")
            if "maximum" in schema and valor > schema["maximum"]:
                errores.append(f"{camino}: mayor que maximum")
            if schema.get("format") == "int64" and not (INT64_MIN <= valor <= INT64_MAX):
                errores.append(f"{camino}: fuera de int64")

        if isinstance(valor, list):
            if "minItems" in schema and len(valor) < int(schema["minItems"]):
                errores.append(f"{camino}: menos elementos que minItems")
            if "maxItems" in schema and len(valor) > int(schema["maxItems"]):
                errores.append(f"{camino}: más elementos que maxItems")
            sub = schema.get("items")
            if sub is not None:
                for indice, elemento in enumerate(valor):
                    errores.extend(self.validar(elemento, sub, f"{camino}[{indice}]"))

        if isinstance(valor, dict):
            requeridos = schema.get("required") or []
            for clave in requeridos:
                if clave not in valor:
                    errores.append(f"{camino}: falta requerido {clave}")
            propiedades = schema.get("properties") or {}
            for clave, sub in propiedades.items():
                if clave in valor:
                    errores.extend(self.validar(valor[clave], sub, f"{camino}.{clave}"))
            adicionales = schema.get("additionalProperties", True)
            if adicionales is not True:
                for clave in valor:
                    if clave in propiedades:
                        continue
                    if adicionales is False:
                        errores.append(f"{camino}: propiedad no declarada {clave}")
                    elif isinstance(adicionales, dict):
                        errores.extend(self.validar(valor[clave], adicionales, f"{camino}.{clave}"))
        return errores

    # -- solicitudes y respuestas ------------------------------------------- #

    def validar_solicitud(self, eid: str, cuerpo: Any) -> list[str]:
        op = self.operaciones.get(eid)
        if op is None:
            return [f"operación desconocida: {eid}"]
        if op.solicitud is None:
            return [f"{eid}: el contrato no declara requestBody; no se envía cuerpo"] if cuerpo is not None else []
        return self.validar(cuerpo, op.solicitud, "$solicitud")

    def validar_respuesta(self, eid: str, codigo: int, cuerpo: Any) -> list[str]:
        op = self.operaciones[eid]
        respuesta = op.respuestas.get(str(codigo))
        if respuesta is None:
            return [f"{eid}: código {codigo} no documentado"]
        contenido = (respuesta.get("content") or {}) if isinstance(respuesta, dict) else {}
        if not contenido:
            if cuerpo not in (None, ""):
                return [f"{eid}: código {codigo} no declara cuerpo"]
            return []
        schema = None
        for _ct, media in contenido.items():
            if isinstance(media, dict):
                schema = media.get("schema")
                break
        if schema is None:
            return [f"{eid}: código {codigo} sin schema"]
        return self.validar(cuerpo, schema, "$respuesta")


# --------------------------------------------------------------------------- #
# Secretos
# --------------------------------------------------------------------------- #

RE_CORREO = re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}")
RE_BEARER = re.compile(r"(?i)bearer\s+[A-Za-z0-9._\-]+")
RE_JWT = re.compile(r"eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}")


class Secretos:
    """Registro de valores sensibles para redactar y auditar resultados."""

    def __init__(self) -> None:
        self._valores: dict[str, str] = {}

    def registrar(self, tipo: str, valor: str | None) -> str:
        if not valor:
            return ""
        if len(valor) >= 6:
            self._valores.setdefault(valor, tipo)
        return valor

    def registrar_correo(self, correo: str | None) -> str:
        return self.registrar("correo", (correo or "").strip().lower())

    @property
    def valores(self) -> Mapping[str, str]:
        return dict(self._valores)

    def redactar(self, texto: str) -> str:
        if not texto:
            return texto
        salida = texto
        for valor, tipo in sorted(self._valores.items(), key=lambda kv: -len(kv[0])):
            salida = salida.replace(valor, f"[REDACTADO:{tipo}]")
        salida = RE_CORREO.sub("[REDACTADO:correo]", salida)
        salida = RE_BEARER.sub("[REDACTADO:token]", salida)
        salida = RE_JWT.sub("[REDACTADO:jwt]", salida)
        return salida

    def auditar(self, documento: Any) -> list[str]:
        """Devuelve rutas con fugas; nunca devuelve el valor encontrado."""
        crudo = json.dumps(documento, ensure_ascii=False, sort_keys=True)
        fugas: list[str] = []
        for valor, tipo in self._valores.items():
            if valor in crudo:
                fugas.append(f"{tipo} literal presente en resultados")
        for patron, nombre in ((RE_CORREO, "correo"), (RE_BEARER, "bearer"), (RE_JWT, "jwt")):
            if patron.search(crudo):
                fugas.append(f"patrón de {nombre} presente en resultados")
        return fugas


# --------------------------------------------------------------------------- #
# Autorización de mutaciones
# --------------------------------------------------------------------------- #

class Autorizacion:
    """Puerta única. Se consulta con el escenario, nunca con una bandera del llamador."""

    def __init__(self, permitir_mutaciones: bool = False, permitir_eliminar_cuenta: bool = False):
        self.permitir_mutaciones = bool(permitir_mutaciones)
        self.permitir_eliminar_cuenta = bool(permitir_eliminar_cuenta)

    def permite(self, escenario: Any) -> bool:
        if getattr(escenario, "destructivo", False) and not self.permitir_eliminar_cuenta:
            return False
        if getattr(escenario, "muta", False) and not self.permitir_mutaciones:
            return False
        return True

    def exigir(self, escenario: Any) -> None:
        if getattr(escenario, "destructivo", False) and not self.permitir_eliminar_cuenta:
            raise MutacionDenegada(f"{escenario.eid} requiere --allow-delete-account")
        if getattr(escenario, "muta", False) and not self.permitir_mutaciones:
            raise MutacionDenegada(f"{escenario.eid} requiere --allow-mutations")


# --------------------------------------------------------------------------- #
# Cliente HTTP
# --------------------------------------------------------------------------- #

@dataclass
class Respuesta:
    codigo: int
    cuerpo: Any
    cabeceras: dict[str, str]
    latencia_ms: int
    texto: str = ""
    error_red: str = ""

    @property
    def etag(self) -> str:
        return self.cabeceras.get("etag", "")

    def campo(self, *ruta: str, default: Any = None) -> Any:
        actual: Any = self.cuerpo
        for parte in ruta:
            if isinstance(actual, dict) and parte in actual:
                actual = actual[parte]
            else:
                return default
        return actual


def validar_base_url(valor: str) -> str:
    limpio = (valor or "").strip()
    if not limpio:
        raise ConfiguracionInvalida("falta --base-url")
    partes = urllib.parse.urlparse(limpio)
    if partes.scheme != "https":
        raise ConfiguracionInvalida("la base debe usar https; no se admite http")
    if not partes.hostname:
        raise ConfiguracionInvalida("la base no tiene host")
    if partes.username or partes.password:
        raise ConfiguracionInvalida("la base no puede incluir credenciales")
    if partes.query or partes.fragment:
        raise ConfiguracionInvalida("la base no puede incluir query ni fragmento")
    camino = (partes.path or "").rstrip("/")
    if camino not in ("", "/api/v1"):
        raise ConfiguracionInvalida("la base debe ser el origen o el origen seguido de /api/v1")
    host = partes.hostname
    puerto = f":{partes.port}" if partes.port else ""
    return f"https://{host}{puerto}"


class _RedireccionSegura(urllib.request.HTTPRedirectHandler):
    max_redirections = 3

    def redirect_request(self, req, fp, code, msg, headers, newurl):  # type: ignore[no-untyped-def]
        origen = urllib.parse.urlparse(req.full_url)
        destino = urllib.parse.urlparse(newurl)
        if (
            destino.scheme != "https"
            or destino.scheme != origen.scheme
            or destino.hostname != origen.hostname
            or (destino.port or 443) != (origen.port or 443)
        ):
            raise urllib.error.HTTPError(
                newurl, code, "redirección a otro origen rechazada", headers, fp
            )
        return super().redirect_request(req, fp, code, msg, headers, newurl)


class Cliente:
    """Cliente que solo habla HTTPS, no sigue redirecciones de otro origen y audita
    cada petición contra el registro de escenarios y el contrato."""

    def __init__(
        self,
        base_url: str,
        autorizacion: Autorizacion,
        escenarios: Mapping[str, Any],
        validador: ValidadorEsquema,
        transporte: Callable[[str, str, bytes | None, dict[str, str], int], tuple[int, dict[str, str], bytes]] | None = None,
        timeout: int = 20,
    ):
        self.base = validar_base_url(base_url)
        self.autorizacion = autorizacion
        self.escenarios = escenarios
        self.validador = validador
        self.timeout = timeout
        self.transporte = transporte or self._transporte_urllib
        self.llamadas: list[dict[str, Any]] = []
        self.contexto_ssl = ssl.create_default_context()
        if self.contexto_ssl.verify_mode != ssl.CERT_REQUIRED or not self.contexto_ssl.check_hostname:
            raise ConfiguracionInvalida("la verificación TLS debe permanecer activa")

    # -- transporte real ----------------------------------------------------- #

    def _transporte_urllib(
        self, metodo: str, url: str, datos: bytes | None, cabeceras: dict[str, str], timeout: int
    ) -> tuple[int, dict[str, str], bytes]:
        peticion = urllib.request.Request(url, data=datos, method=metodo, headers=cabeceras)
        abridor = urllib.request.build_opener(
            _RedireccionSegura(), urllib.request.HTTPSHandler(context=self.contexto_ssl)
        )
        try:
            with abridor.open(peticion, timeout=timeout) as respuesta:
                final = respuesta.geturl()
                self._exigir_mismo_origen(final)
                return respuesta.status, dict(respuesta.headers), respuesta.read()
        except urllib.error.HTTPError as exc:
            crudo = exc.read() if hasattr(exc, "read") else b""
            return exc.code, dict(exc.headers or {}), crudo
        except urllib.error.URLError as exc:
            raise ConnectionError(str(exc.reason)) from exc

    def _exigir_mismo_origen(self, url: str) -> None:
        origen = urllib.parse.urlparse(self.base)
        final = urllib.parse.urlparse(url)
        if final.scheme != "https" or final.hostname != origen.hostname or (final.port or 443) != (origen.port or 443):
            raise ConfiguracionInvalida("la respuesta final pertenece a otro origen")

    # -- API ----------------------------------------------------------------- #

    def pedir(
        self,
        eid: str,
        *,
        ruta: str | None = None,
        token: str | None = None,
        cuerpo: Any = None,
        cabeceras: Mapping[str, str] | None = None,
        idempotencia: str | None = None,
        timeout: int | None = None,
        validar_payload: bool = True,
    ) -> Respuesta:
        escenario = self.escenarios.get(eid)
        if escenario is None:
            raise ConfiguracionInvalida(f"petición sin escenario registrado: {eid}")
        metodo = escenario.metodo
        ruta_final = ruta or escenario.ruta
        if not ruta_coincide(escenario.ruta, ruta_final):
            raise SolicitudInvalida(f"{eid}: ruta {ruta_final} no coincide con {escenario.ruta}")
        self.autorizacion.exigir(escenario)
        if escenario.idempotencia and not idempotencia:
            raise SolicitudInvalida(f"{eid}: Idempotency-Key obligatoria")
        if validar_payload:
            if cuerpo is not None:
                problemas = self.validador.validar_solicitud(eid, cuerpo)
                if problemas:
                    raise SolicitudInvalida(f"{eid}: payload inválido: " + "; ".join(problemas[:3]))
            elif escenario.solicitud is not None and escenario.idempotencia:
                # Hay operaciones con cuerpo obligatorio; si el contrato exige un objeto,
                # no se puede enviar sin cuerpo.
                problemas = self.validador.validar_solicitud(eid, {})
                if problemas and "falta requerido" in problemas[0]:
                    raise SolicitudInvalida(f"{eid}: el contrato exige cuerpo")

        datos = None
        hdrs = {"Accept": "application/json", "User-Agent": "nexum-certificacion/2"}
        if cuerpo is not None:
            datos = json.dumps(cuerpo, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
            hdrs["Content-Type"] = "application/json"
        if token:
            hdrs["Authorization"] = "Bearer " + token
        if idempotencia:
            hdrs["Idempotency-Key"] = idempotencia
        if cabeceras:
            hdrs.update(cabeceras)

        url = self.base + _sin_query(ruta_final)
        consulta = ruta_final.split("?", 1)
        if len(consulta) == 2 and consulta[1]:
            url += "?" + consulta[1]

        inicio = time.monotonic()
        intentos = 0
        while True:
            try:
                codigo, cabeceras_respuesta, crudo = self.transporte(
                    metodo, url, datos, hdrs, timeout or self.timeout
                )
            except ConnectionError as exc:
                latencia = int((time.monotonic() - inicio) * 1000)
                respuesta = Respuesta(0, None, {}, latencia, "", f"error de red: {exc}")
                self._registrar_llamada(escenario, respuesta)
                return respuesta
            if codigo == 429 and intentos < 6:
                # DEF-H10: respetar Retry-After en lugar de fallar por cuota. El limitador
                # rechaza antes de procesar, por lo que el reintento no duplica efectos.
                cabeceras_429 = {str(k).lower(): str(v) for k, v in (cabeceras_respuesta or {}).items()}
                espera = 5.0 * (2 ** intentos)
                try:
                    espera = max(1.0, min(90.0, float(cabeceras_429.get("retry-after", espera))))
                except ValueError:
                    pass
                intentos += 1
                time.sleep(espera)
                continue
            break
        latencia = int((time.monotonic() - inicio) * 1000)
        texto = crudo.decode("utf-8", errors="replace") if crudo else ""
        try:
            decodificado = json.loads(texto) if texto.strip() else None
        except json.JSONDecodeError:
            decodificado = None
        normalizadas = {str(k).lower(): str(v) for k, v in (cabeceras_respuesta or {}).items()}
        respuesta = Respuesta(codigo, decodificado, normalizadas, latencia, texto)
        self._registrar_llamada(escenario, respuesta)
        return respuesta

    def _registrar_llamada(self, escenario: Any, respuesta: Respuesta) -> None:
        self.llamadas.append(
            {
                "eid": escenario.eid,
                "metodo": escenario.metodo,
                "ruta": escenario.ruta,
                "codigo": respuesta.codigo,
                "latencia_ms": respuesta.latencia_ms,
            }
        )


# --------------------------------------------------------------------------- #
# Sesiones y contexto
# --------------------------------------------------------------------------- #

@dataclass(repr=False)
class Sesion:
    etiqueta: str
    email: str
    password: str
    password_origen: str = "generado"
    access: str = ""
    refresh: str = ""
    session_id: str = ""
    device_id: str = ""
    user_id: str = ""

    def __repr__(self) -> str:  # nunca exponer la contraseña en trazas
        return f"Sesion(etiqueta={self.etiqueta!r}, email=[REDACTADO], access={'sí' if self.access else 'no'})"


@dataclass
class Opciones:
    fases: tuple[str, ...] = ("todo",)
    solo: tuple[str, ...] = ()
    permitir_mutaciones: bool = False
    permitir_eliminar_cuenta: bool = False
    fcm_token: str = ""
    reset_token: str = ""
    verify_token: str = ""
    fcm_receipt_file: str = ""
    espera_ocurrencia_s: int = 90
    intervalo_poll_s: int = 10
    verbose: bool = False
    reanudar: str = ""
    out: str = ""
    base_url: str = ""


class Contexto:
    def __init__(
        self,
        cliente: Cliente,
        autorizacion: Autorizacion,
        a: Sesion,
        b: Sesion,
        opciones: Opciones,
        resultados: Mapping[str, Resultado],
        inventario: Mapping[str, Operacion],
        escenarios: Mapping[str, Any],
        seleccion: Iterable[str],
        secretos: Secretos,
        reanudados: Mapping[str, Resultado] | None = None,
    ):
        self.cliente = cliente
        self.autorizacion = autorizacion
        self.a = a
        self.b = b
        self.opciones = opciones
        self.resultados = dict(resultados)
        self.inventario = inventario
        self.escenarios = escenarios
        self.seleccion = set(seleccion)
        self.secretos = secretos
        self.reanudados = dict(reanudados or {})
        self.datos: dict[str, Any] = {}
        self.bloqueos: list[str] = []
        self.advertencias: list[str] = []
        self.validador = cliente.validador

    # -- resultados ---------------------------------------------------------- #

    def reg(self, eid: str) -> Resultado:
        if eid not in self.resultados:
            esc = self.escenarios[eid]
            self.resultados[eid] = Resultado(eid, getattr(esc, "titulo", ""), esc.metodo, esc.ruta)
        return self.resultados[eid]

    def activo(self, eid: str) -> bool:
        return eid in self.seleccion

    def disponible(self, eid: str) -> bool:
        actual = self.resultados.get(eid)
        if actual is not None and actual.estado_final() == PASS:
            return True
        previo = self.reanudados.get(eid)
        return previo is not None and previo.estado_final() == PASS

    def dependencia(self, eid: str, resultado: Resultado) -> bool:
        if self.disponible(eid):
            return True
        motivo = f"requiere {eid} ejecutado en esta corrida o reanudado con PASS"
        resultado.bloqueo(f"dependencia {eid}", motivo, BLOCKED_DEPENDENCY)
        return False

    # -- red ---------------------------------------------------------------- #

    def pedir(self, eid: str, **kwargs: Any) -> Respuesta:
        return self.cliente.pedir(eid, **kwargs)

    def etiqueta_peticion(self, eid: str, respuesta: Respuesta) -> str:
        """Texto corto y sin secretos para un check."""
        sufijo = "red" if respuesta.codigo == 0 else str(respuesta.codigo)
        return f"{self.escenarios[eid].metodo} {self.escenarios[eid].ruta} -> {sufijo}"

    # -- contrato ------------------------------------------------------------ #

    def exigir_contrato(self, eid: str, respuesta: Respuesta, resultado: Resultado, esperado: Iterable[int]) -> bool:
        """Valida código esperado, documentación y schema. Devuelve True si el cuerpo
        se puede interpretar con seguridad."""
        esperados = tuple(esperado)
        if respuesta.codigo == 0:
            resultado.fallo("respuesta HTTP", self.etiqueta_peticion(eid, respuesta), FAIL)
            return False
        if respuesta.codigo not in esperados:
            estado = FAIL if respuesta.codigo >= 500 or respuesta.codigo == 0 else FAIL
            resultado.fallo(
                f"HTTP en {esperados}",
                self.etiqueta_peticion(eid, respuesta),
                estado,
            )
            return False
        problemas = self.validador.validar_respuesta(eid, respuesta.codigo, respuesta.cuerpo)
        if problemas:
            resultado.fallo(
                "respuesta conforme al OpenAPI",
                "; ".join(problemas[:3]),
                CONTRACT_MISMATCH,
            )
            return False
        if respuesta.codigo in (204, 202) and respuesta.texto.strip():
            # 202 sí documenta cuerpo; 204 no. Se resuelve en el check concreto.
            pass
        return True

    # -- sesión -------------------------------------------------------------- #

    def cargar_sesion(self, sesion: Sesion, respuesta: Respuesta) -> None:
        sesion.access = respuesta.campo("accessToken") or sesion.access
        sesion.refresh = respuesta.campo("refreshToken") or sesion.refresh
        sesion.session_id = respuesta.campo("sessionId") or respuesta.campo("session", "id") or sesion.session_id
        sesion.user_id = respuesta.campo("user", "id") or sesion.user_id
        self.secretos.registrar("token-acceso", sesion.access)
        self.secretos.registrar("token-refresh", sesion.refresh)

    def _ruta_cache_sesiones(self) -> str:
        return os.environ.get("NEXUM_SESSION_CACHE", "").strip()

    def _restaurar_sesion(self, sesion: Sesion) -> bool:
        """Reutiliza un access token vigente de una corrida anterior (misma cuenta)."""
        ruta = self._ruta_cache_sesiones()
        if not ruta or not os.path.exists(ruta):
            return False
        try:
            datos = json.loads(io.open(ruta, encoding="utf-8").read())
        except Exception:  # noqa: BLE001
            return False
        entrada = datos.get(sesion.email) or {}
        if not entrada.get("access") or float(entrada.get("expira_en", 0)) <= time.time():
            return False
        sesion.access = entrada["access"]
        sesion.refresh = entrada.get("refresh", "")
        sesion.session_id = entrada.get("session_id", "")
        sesion.user_id = entrada.get("user_id", "")
        return True

    def _guardar_sesion(self, sesion: Sesion, respuesta: Respuesta) -> None:
        ruta = self._ruta_cache_sesiones()
        if not ruta:
            return
        try:
            datos = json.loads(io.open(ruta, encoding="utf-8").read()) if os.path.exists(ruta) else {}
        except Exception:  # noqa: BLE001
            datos = {}
        expires = respuesta.campo("expiresIn")
        segundos = int(expires) if isinstance(expires, (int, float)) else 840
        datos[sesion.email] = {
            "access": sesion.access,
            "refresh": sesion.refresh,
            "session_id": sesion.session_id,
            "user_id": sesion.user_id,
            "expira_en": time.time() + max(60, segundos - 60),
        }
        try:
            os.makedirs(os.path.dirname(ruta), exist_ok=True)
            io.open(ruta, "w", encoding="utf-8").write(json.dumps(datos, ensure_ascii=False))
        except Exception:  # noqa: BLE001
            pass

    def login_auxiliar(self, sesion: Sesion, resultado: Resultado) -> bool:
        """Login fuera del escenario E002 (para reanudar fases sin repetir la suite).

        No certifica E002: si falla, el estado es BLOCKED_EXTERNAL, no FAIL.
        """
        if sesion.access:
            return True
        if self._restaurar_sesion(sesion):
            return True
        if not self.autorizacion.permite(self.escenarios["E002"]):
            resultado.bloqueo(
                "sesión disponible",
                "requiere --allow-mutations para el login auxiliar",
                BLOCKED_EXTERNAL,
            )
            return False
        if not sesion.email or not sesion.password:
            resultado.bloqueo(
                "sesión disponible",
                "faltan credenciales de la cuenta (NEXUM_EMAIL_*/NEXUM_PASSWORD_*)",
                BLOCKED_EXTERNAL,
            )
            return False
        respuesta = self.pedir(
            "E002",
            cuerpo={"email": sesion.email, "password": sesion.password, "deviceId": sesion.device_id},
        )
        if respuesta.codigo != 200:
            resultado.bloqueo(
                "sesión disponible",
                f"login auxiliar {respuesta.codigo}; no se certifica E002 en esta fase",
                BLOCKED_EXTERNAL,
            )
            return False
        self.cargar_sesion(sesion, respuesta)
        self._guardar_sesion(sesion, respuesta)
        return True

    # -- datos de dependencia ------------------------------------------------ #

    def notice_version(self) -> str:
        if self.datos.get("notice_version"):
            return str(self.datos["notice_version"])
        respuesta = self.pedir("E015")
        version = respuesta.campo("version")
        if isinstance(version, str) and version:
            self.datos["notice_version"] = version
            return version
        return os.environ.get("NEXUM_NOTICE_VERSION", "").strip()

    def category_id(self) -> str:
        if self.datos.get("category_id"):
            return str(self.datos["category_id"])
        respuesta = self.pedir("E022")
        categorias = respuesta.campo("categories")
        if isinstance(categorias, list):
            for item in categorias:
                if isinstance(item, dict) and isinstance(item.get("id"), str) and item["id"]:
                    self.datos["category_id"] = item["id"]
                    return item["id"]
        return os.environ.get("NEXUM_CATEGORY_ID", "otros").strip() or "otros"


# --------------------------------------------------------------------------- #
# Código de salida y resumen
# --------------------------------------------------------------------------- #

def calcular_codigo_salida(
    resultados: Mapping[str, Resultado],
    inventario: Mapping[str, Operacion],
    completo: bool,
) -> int:
    """0 certificación completa sin fallos; 1 algún fallo; 2 incompleta."""
    faltan = set(inventario) - set(resultados)
    sobran = set(resultados) - set(inventario)
    estados = [resultado.estado_final() for resultado in resultados.values()]
    if any(estado in ESTADOS_FALLO for estado in estados):
        return 1
    if faltan or sobran:
        return 2
    if not completo:
        return 2
    if len(resultados) != len(inventario):
        return 2
    if any(estado == NOT_RUN for estado in estados):
        return 2
    if not any(estado == PASS for estado in estados):
        return 2
    return 0


def motivo_codigo(codigo: int, resultados: Mapping[str, Resultado], inventario: Mapping[str, Operacion], completo: bool) -> str:
    if codigo == 0:
        return "CERTIFICACION_COMPLETA"
    if codigo == 1:
        fallos = [eid for eid, r in sorted(resultados.items()) if r.estado_final() in ESTADOS_FALLO]
        return "FALLOS: " + ", ".join(fallos)
    faltan = sorted(set(inventario) - set(resultados))
    if faltan:
        return "INCOMPLETA: faltan E-ID " + ", ".join(faltan)
    if not completo:
        return "INCOMPLETA: ejecución por fases o dry-run"
    pendientes = [eid for eid, r in sorted(resultados.items()) if r.estado_final() == NOT_RUN]
    if pendientes:
        return "INCOMPLETA: NOT_RUN " + ", ".join(pendientes)
    return "INCOMPLETA"


def contar_estados(resultados: Mapping[str, Resultado]) -> dict[str, int]:
    conteo = {estado: 0 for estado in ESTADOS}
    for resultado in resultados.values():
        conteo[resultado.estado_final()] = conteo.get(resultado.estado_final(), 0) + 1
    return conteo


def construir_resumen(
    resultados: Mapping[str, Resultado],
    inventario: Mapping[str, Operacion],
    *,
    fecha: str,
    base_url: str,
    openapi_ruta: str,
    openapi_sha256: str,
    fases: Iterable[str],
    solo: Iterable[str],
    mutaciones: bool,
    eliminacion: bool,
    completo: bool,
    codigo: int,
    bloqueos: Iterable[str],
    advertencias: Iterable[str],
    llamadas: Iterable[Mapping[str, Any]],
    reanudado_de: str = "",
) -> dict[str, Any]:
    return {
        "version": VERSION_RESULTADOS,
        "fecha": fecha,
        "base_url": base_url,
        "openapi": {"ruta": openapi_ruta, "sha256": openapi_sha256, "operaciones": len(inventario)},
        "inventario_digest": digest_inventario(inventario),
        "modo": {
            "fases": list(fases),
            "solo": list(solo),
            "mutaciones": mutaciones,
            "eliminacion": eliminacion,
            "completo": completo,
            "reanudado_de": reanudado_de,
        },
        "total_operaciones": len(inventario),
        "evaluadas": len(resultados),
        "estados": contar_estados(resultados),
        "completa": completo,
        "codigo_salida": codigo,
        "motivo": motivo_codigo(codigo, resultados, inventario, completo),
        "bloqueos": list(bloqueos),
        "advertencias": list(advertencias),
        "llamadas": list(llamadas),
        "resultados": {eid: resultado.dict() for eid, resultado in sorted(resultados.items())},
        "nota": (
            "No se persisten contraseñas, tokens, correos ni cuerpos completos. "
            "E014 exige verificación independiente de PURGED. Los BLOCKED no son PASS."
        ),
    }


def escribir_resumen(ruta: str, resumen: Mapping[str, Any], secretos: Secretos) -> None:
    fugas = secretos.auditar(resumen)
    if fugas:
        raise FugaDeSecreto("; ".join(sorted(set(fugas))))
    with open(ruta, "w", encoding="utf-8") as fh:
        json.dump(resumen, fh, ensure_ascii=False, indent=2, sort_keys=False)
        fh.write("\n")


def sha256_archivo(ruta: str | os.PathLike[str]) -> str:
    digest = hashlib.sha256()
    with open(ruta, "rb") as fh:
        for bloque in iter(lambda: fh.read(65536), b""):
            digest.update(bloque)
    return digest.hexdigest()


# --------------------------------------------------------------------------- #
# Reanudación
# --------------------------------------------------------------------------- #

@dataclass
class InformeReanudacion:
    fusionados: dict[str, Resultado] = field(default_factory=dict)
    problemas: list[str] = field(default_factory=list)
    sha256: str = ""


def _resultado_desde_dict(datos: Mapping[str, Any]) -> Resultado:
    resultado = Resultado(
        str(datos.get("eid", "")),
        str(datos.get("titulo", "")),
        str(datos.get("metodo", "")),
        str(datos.get("ruta", "")),
    )
    checks = datos.get("checks") or []
    for check in checks:
        if not isinstance(check, dict):
            continue
        estado = check.get("estado", FAIL)
        if estado not in ESTADOS:
            estado = FAIL
        resultado.checks.append(
            {
                "check": str(check.get("check", ""))[:120],
                "ok": bool(check.get("ok", False)),
                "estado": estado,
                "detalle": str(check.get("detalle", ""))[:300],
            }
        )
    resultado.motivo = str(datos.get("motivo", ""))
    resultado.origen = "reanudado"
    return resultado


def fusionar_reanudacion(
    previo: Mapping[str, Any],
    seleccion: Iterable[str],
    inventario: Mapping[str, Operacion],
    base_url: str,
    digest: str,
) -> InformeReanudacion:
    """Recupera de una corrida previa solo PASS/BLOCKED de E-ID no seleccionados.

    Nunca hereda FAIL, CONTRACT_MISMATCH, CONTENT_MISMATCH ni NOT_RUN: esos deben
    volver a ejecutarse. Un archivo de otro entorno, otro inventario o una versión
    distinta se rechaza completo.
    """
    informe = InformeReanudacion()
    if previo.get("version") != VERSION_RESULTADOS:
        informe.problemas.append("versión de resultados distinta")
        return informe
    if previo.get("inventario_digest") != digest:
        informe.problemas.append("inventario distinto: no se puede reanudar")
        return informe
    if str(previo.get("base_url", "")).rstrip("/") != base_url.rstrip("/"):
        informe.problemas.append("base_url distinta: la evidencia es de otro entorno")
        return informe
    resultados = previo.get("resultados")
    if not isinstance(resultados, dict):
        informe.problemas.append("resultados ausentes o malformados")
        return informe
    seleccionados = set(seleccion)
    for eid, datos in resultados.items():
        if eid in seleccionados:
            continue
        if eid not in inventario:
            informe.problemas.append(f"E-ID desconocido en el archivo previo: {eid}")
            continue
        if not isinstance(datos, dict):
            informe.problemas.append(f"{eid}: resultado malformado")
            continue
        estado = datos.get("estado")
        if estado not in (PASS, BLOCKED_EXTERNAL, BLOCKED_DEPENDENCY):
            continue
        resultado = _resultado_desde_dict(datos)
        if resultado.estado_final() != estado:
            informe.problemas.append(f"{eid}: checks incoherentes con el estado {estado}")
            continue
        informe.fusionados[eid] = resultado
    if not informe.fusionados and not informe.problemas:
        informe.problemas.append("el archivo previo no aporta resultados heredables")
    return informe


# --------------------------------------------------------------------------- #
# Entrada de secretos sin argumentos visibles
# --------------------------------------------------------------------------- #

def leer_secreto(nombre_env: str, descripcion: str, permitir_prompt: bool = True) -> str:
    """Lee un secreto del entorno o de la terminal sin eco. Nunca de argv."""
    valor = os.environ.get(nombre_env, "").strip()
    if valor:
        return valor
    if permitir_prompt and sys.stdin.isatty():
        try:
            return getpass(f"{descripcion} ({nombre_env}, sin eco): ").strip()
        except (EOFError, KeyboardInterrupt):
            return ""
    return ""


def leer_texto(nombre_env: str, descripcion: str, permitir_prompt: bool = True) -> str:
    valor = os.environ.get(nombre_env, "").strip()
    if valor:
        return valor
    if permitir_prompt and sys.stdin.isatty():
        try:
            return input(f"{descripcion} ({nombre_env}): ").strip()
        except (EOFError, KeyboardInterrupt):
            return ""
    return ""


# --------------------------------------------------------------------------- #
# Utilidades de fecha/dinero
# --------------------------------------------------------------------------- #

def ahora_utc() -> datetime:
    return datetime.now(timezone.utc)


def iso_utc(instante: datetime | None = None) -> str:
    valor = (instante or ahora_utc()).astimezone(timezone.utc)
    return valor.strftime("%Y-%m-%dT%H:%M:%S.000Z")


def fecha_desplazada(dias: int) -> str:
    from datetime import timedelta

    return (ahora_utc() + timedelta(days=dias)).strftime("%Y-%m-%d")


def periodo_desplazado(meses: int) -> str:
    hoy = ahora_utc()
    total = hoy.year * 12 + (hoy.month - 1) + meses
    return f"{total // 12:04d}-{total % 12 + 1:02d}"


def instante_en_periodo(periodo: str, dia: int = 15, hora: int = 12) -> str:
    anio, mes = periodo.split("-")
    return f"{anio}-{mes}-{dia:02d}T{hora:02d}:00:00.000Z"


def decimal_exacto(valor: Any) -> Decimal | None:
    if not isinstance(valor, str):
        return None
    try:
        return Decimal(valor)
    except (InvalidOperation, ValueError):
        return None


def comparar_decimal(obtenido: Any, esperado: str) -> bool:
    numero = decimal_exacto(obtenido)
    if numero is None:
        return False
    return numero == Decimal(esperado)


def idempotencia() -> str:
    return os.urandom(16).hex()


def nuevo_uuid() -> str:
    import uuid

    return str(uuid.uuid4())
