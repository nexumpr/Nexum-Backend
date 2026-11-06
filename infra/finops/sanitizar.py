"""Sanitización de salidas AWS antes de persistir evidencia (I-000).

La evidencia cruda vive fuera del repositorio; sólo se versiona la versión
sanitizada. Se eliminan identificadores de cuenta, ARN, correos, claves de
acceso, IPv4 y tokens con forma de secreto.
"""

from __future__ import annotations

import json
import re
from typing import Any

PATRONES: tuple[tuple[re.Pattern[str], str], ...] = (
    (re.compile(r"arn:aws[a-z\-]*:[^\s\"',]+"), "<ARN>"),
    (re.compile(r"\bAKIA[0-9A-Z]{16}\b"), "<AWS_ACCESS_KEY_ID>"),
    (re.compile(r"\bASIA[0-9A-Z]{16}\b"), "<AWS_SESSION_KEY_ID>"),
    (re.compile(r"\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b"), "<EMAIL>"),
    (re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b"), "<IPV4>"),
    (re.compile(r"\b(?:i|vol|snap|ami|vpc|subnet|sg|eni|igw|nat|eipalloc|eipassoc|fs|ar)-[0-9a-f]{8,17}\b"), "<RESOURCE_ID>"),
    (re.compile(r"\b[a-z0-9]{4}[a-z0-9]{4}[a-z0-9]{4}[a-z0-9]{4}\b(?=\s*$)"), "<ACCOUNT_ID>"),
    (re.compile(r"\b\d{12}\b"), "<ACCOUNT_ID>"),
    (re.compile(r"\bBearer\s+[A-Za-z0-9\-._~+/]+=*"), "Bearer <TOKEN>"),
    (re.compile(r"\beyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\b"), "<JWT>"),
)

CAMPOS_SENSIBLES = {
    "accesskeyid",
    "secretaccesskey",
    "sessiontoken",
    "account",
    "accountid",
    "account_id",
    "arn",
    "userid",
    "user_id",
    "password",
    "authorization",
}


def _sanitizar_texto(texto: str) -> str:
    resultado = texto
    for patron, reemplazo in PATRONES:
        resultado = patron.sub(reemplazo, resultado)
    return resultado


def sanitizar(valor: Any) -> Any:
    """Sanitiza recursivamente estructuras JSON-compatibles."""

    if isinstance(valor, dict):
        limpio: dict[str, Any] = {}
        for clave, contenido in valor.items():
            if clave.lower() in CAMPOS_SENSIBLES:
                if clave.lower() in ("account", "accountid", "account_id"):
                    limpio[clave] = "<ACCOUNT_ID>"
                elif clave.lower() in ("arn",):
                    limpio[clave] = "<ARN>"
                else:
                    limpio[clave] = "<REDACTED>"
            else:
                limpio[clave] = sanitizar(contenido)
        return limpio
    if isinstance(valor, list):
        return [sanitizar(v) for v in valor]
    if isinstance(valor, str):
        return _sanitizar_texto(valor)
    return valor


def sanitizar_json(texto: str) -> str:
    """Sanitiza un documento JSON serializado conservando su estructura."""

    try:
        datos = json.loads(texto)
    except json.JSONDecodeError:
        return _sanitizar_texto(texto)
    return json.dumps(sanitizar(datos), indent=2, ensure_ascii=False, sort_keys=True)


def contiene_datos_sensibles(texto: str) -> list[str]:
    """Devuelve los patrones sensibles todavía presentes (lista vacía si limpio)."""

    hallazgos: list[str] = []
    for patron, nombre in PATRONES:
        if patron.search(texto):
            hallazgos.append(nombre)
    for campo in ("AKIA", "aws_secret_access_key", "BEGIN RSA PRIVATE KEY"):
        if campo in texto:
            hallazgos.append(campo)
    return hallazgos
