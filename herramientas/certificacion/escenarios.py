"""Registro de los 75 escenarios de certificación y sus flujos.

Cada E-ID del OpenAPI (`nexum_eNNN`) tiene exactamente un escenario con su método y
ruta. El registro se contrasta con el contrato antes de ejecutar: si falta, sobra o no
coincide una ruta, el harness falla cerrado (código 2) y no certifica nada.

Convenciones:
* Toda petición se hace con `ctx.pedir(eid, ...)`, que consulta la puerta única de
  autorización a partir del escenario. No existe una bandera de mutación por llamada.
* Los detalles de los checks nunca incluyen correos, tokens ni cuerpos completos.
"""

from __future__ import annotations

import json
import os
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Any, Callable, Iterable, Mapping

import nucleo as n

FASE_PUBLICO = "publico"
FASE_AUTH = "auth"
FASE_PERFIL = "perfil"
FASE_FINANZAS = "finanzas"
FASE_ANALITICA = "analitica"
FASE_NOTIFICACIONES = "notificaciones"
FASE_CONTENIDO = "contenido"
FASE_CORREO_SOLICITUD = "correo-solicitud"
FASE_RECUPERACION = "recuperacion"
FASE_VERIFICACION = "verificacion"
FASE_ELIMINACION = "eliminacion"
FASE_TODO = "todo"

FASES = (
    FASE_PUBLICO,
    FASE_AUTH,
    FASE_PERFIL,
    FASE_FINANZAS,
    FASE_ANALITICA,
    FASE_NOTIFICACIONES,
    FASE_CONTENIDO,
    FASE_CORREO_SOLICITUD,
    FASE_RECUPERACION,
    FASE_VERIFICACION,
    FASE_ELIMINACION,
    FASE_TODO,
)


@dataclass(frozen=True)
class Escenario:
    eid: str
    titulo: str
    metodo: str
    ruta: str
    fases: tuple[str, ...]
    funcion: Callable[[n.Contexto], None]
    muta: bool = False
    destructivo: bool = False
    depende_de: tuple[str, ...] = ()
    pasos: tuple[str, ...] = ()
    idempotencia: bool = False
    # `nucleo.Cliente.pedir` consulta este atributo para decidir si exige cuerpo.
    # El registro no duplica el schema: el validador lo toma del OpenAPI.
    solicitud: Any = None


# --------------------------------------------------------------------------- #
# Helpers
# --------------------------------------------------------------------------- #

def _exigir(ctx: n.Contexto, eid: str, respuesta: n.Respuesta, resultado: n.Resultado, esperado: Iterable[int]) -> bool:
    return ctx.exigir_contrato(eid, respuesta, resultado, esperado)


def _etag(respuesta: n.Respuesta) -> str:
    if respuesta.etag:
        return respuesta.etag
    version = respuesta.campo("version")
    if isinstance(version, int) and version > 0:
        return f'"v{version}"'
    return ""


def _identificador(respuesta: n.Respuesta, *nombres: str) -> str:
    for nombre in nombres:
        valor = respuesta.campo(nombre)
        if isinstance(valor, str) and valor:
            return valor
    return ""


def _lista(respuesta: n.Respuesta, campo: str = "items") -> list[Any]:
    valor = respuesta.campo(campo)
    return valor if isinstance(valor, list) else []


def _cuerpo(respuesta: n.Respuesta) -> dict[str, Any]:
    return respuesta.cuerpo if isinstance(respuesta.cuerpo, dict) else {}


def _sesion_a(ctx: n.Contexto, resultado: n.Resultado) -> bool:
    return ctx.login_auxiliar(ctx.a, resultado)


def _sesion_b(ctx: n.Contexto, resultado: n.Resultado) -> bool:
    return ctx.login_auxiliar(ctx.b, resultado)


def _mutaciones(ctx: n.Contexto, resultado: n.Resultado, eid: str) -> bool:
    escenario = ctx.escenarios[eid]
    if ctx.autorizacion.permite(escenario):
        return True
    resultado.bloqueo(
        "mutaciones autorizadas",
        f"{eid} requiere --allow-mutations",
        n.BLOCKED_EXTERNAL,
    )
    return False


def _error_de_red(ctx: n.Contexto, eid: str, respuesta: n.Respuesta, resultado: n.Resultado) -> bool:
    if respuesta.codigo == 0:
        resultado.fallo("respuesta HTTP", ctx.etiqueta_peticion(eid, respuesta), n.FAIL)
        return True
    return False


def _detalle_json(valor: Any) -> str:
    """Detalle de un valor no sensible (importes, booleanos, ids ya públicos)."""
    if isinstance(valor, (dict, list)):
        return json.dumps(valor, ensure_ascii=False)[:200]
    return str(valor)[:120]


def _instante(texto: Any) -> datetime | None:
    """Interpreta una marca ISO-8601 con zona; devuelve None si no es utilizable.

    Se usa para correlacionar la ocurrencia de E061 con el evento real del
    recordatorio (``dueAt``), sin depender del título del aviso.
    """
    if not isinstance(texto, str) or not texto.strip():
        return None
    valor = texto.strip()
    if valor.endswith("Z"):
        valor = valor[:-1] + "+00:00"
    try:
        instante = datetime.fromisoformat(valor)
    except ValueError:
        return None
    return instante if instante.tzinfo is not None else instante.replace(tzinfo=timezone.utc)


# --------------------------------------------------------------------------- #
# Público
# --------------------------------------------------------------------------- #

def e015(ctx: n.Contexto) -> None:
    r = ctx.reg("E015")
    p = ctx.pedir("E015")
    if not _exigir(ctx, "E015", p, r, (200,)):
        return
    version = p.campo("version")
    r.ok("version no vacía", str(version)[:40]) if isinstance(version, str) and version else r.fallo(
        "version no vacía", "ausente", n.CONTENT_MISMATCH
    )
    if isinstance(version, str) and version:
        ctx.datos["notice_version"] = version
    r.ok("effectiveAt presente") if p.campo("effectiveAt") else r.fallo(
        "effectiveAt presente", "ausente", n.CONTENT_MISMATCH
    )
    contenido = p.campo("content")
    r.ok("content no vacío", f"{len(contenido)} caracteres") if isinstance(contenido, str) and contenido else r.fallo(
        "content no vacío", "ausente", n.CONTENT_MISMATCH
    )
    r.ok("requiresAcceptance booleano") if isinstance(p.campo("requiresAcceptance"), bool) else r.fallo(
        "requiresAcceptance booleano", "ausente", n.CONTENT_MISMATCH
    )


def e022(ctx: n.Contexto) -> None:
    r = ctx.reg("E022")
    p = ctx.pedir("E022")
    if not _exigir(ctx, "E022", p, r, (200,)):
        return
    items = _lista(p)
    r.ok("catálogo no vacío", f"{len(items)} categorías") if items else r.fallo(
        "catálogo no vacío", "items vacío", n.CONTENT_MISMATCH
    )
    ids = [i.get("id") for i in items if isinstance(i, dict)]
    r.ok("ids únicos") if len(ids) == len(set(ids)) else r.fallo(
        "ids únicos", "hay ids repetidos", n.CONTENT_MISMATCH
    )
    if ids and isinstance(ids[0], str):
        ctx.datos["category_id"] = ids[0]
    r.ok("categorías de gasto presentes") if any(
        isinstance(i, dict) and i.get("type") == "expense" for i in items
    ) else r.fallo("categorías de gasto presentes", "no hay type=expense", n.CONTENT_MISMATCH)


def e023(ctx: n.Contexto) -> None:
    r = ctx.reg("E023")
    p = ctx.pedir("E023")
    if not _exigir(ctx, "E023", p, r, (200,)):
        return
    items = _lista(p)
    ids = {i.get("id") for i in items if isinstance(i, dict)}
    canonicas = {"weekly", "fortnightly", "monthly", "bimonthly", "quarterly", "semiannual", "annual"}
    alias = {"biweekly", "yearly"}
    r.ok("7 frecuencias canónicas", ",".join(sorted(canonicas & ids))) if canonicas <= ids else r.fallo(
        "7 frecuencias canónicas", f"faltan {sorted(canonicas - ids)}", n.CONTENT_MISMATCH
    )
    r.ok("sin alias duplicados", "biweekly/yearly no se emiten") if not (alias & ids) else r.fallo(
        "sin alias duplicados", f"alias emitidos {sorted(alias & ids)}", n.CONTENT_MISMATCH
    )


def e070(ctx: n.Contexto) -> None:
    r = ctx.reg("E070")
    p = ctx.pedir("E070")
    if not _exigir(ctx, "E070", p, r, (200,)):
        return
    items = _lista(p)
    r.ok("FAQ no vacío", f"{len(items)} entradas") if items else r.fallo(
        "FAQ no vacío", "items vacío", n.CONTENT_MISMATCH
    )
    texto = " ".join(
        f"{i.get('question','')} {i.get('answer','')}" for i in items if isinstance(i, dict)
    ).lower()
    falsas = (
        ("cambiar el correo desde la app", "cambiar el correo desde la app"),
        ("editar el correo desde la app", "editar el correo desde la app"),
        ("cuatro frecuencias", "cuatro frecuencias"),
    )
    for etiqueta, aguja in falsas:
        r.ok(f"sin promesa: {etiqueta}") if aguja not in texto else r.fallo(
            f"sin promesa: {etiqueta}", "el contenido publicado la afirma", n.CONTENT_MISMATCH
        )


def e071(ctx: n.Contexto) -> None:
    r = ctx.reg("E071")
    p = ctx.pedir("E071")
    if not _exigir(ctx, "E071", p, r, (200,)):
        return
    items = _lista(p)
    r.ok("tips no vacío", f"{len(items)} tips") if items else r.fallo(
        "tips no vacío", "items vacío", n.CONTENT_MISMATCH
    )
    ids = [i.get("id") for i in items if isinstance(i, dict)]
    r.ok("ids únicos") if len(ids) == len(set(ids)) else r.fallo(
        "ids únicos", "ids repetidos", n.CONTENT_MISMATCH
    )


def e072(ctx: n.Contexto) -> None:
    r = ctx.reg("E072")
    p = ctx.pedir("E072")
    if not _exigir(ctx, "E072", p, r, (200,)):
        return
    r.ok("status ok|degraded", str(p.campo("status"))) if p.campo("status") in ("ok", "degraded") else r.fallo(
        "status ok|degraded", str(p.campo("status")), n.CONTENT_MISMATCH
    )
    claves = set(_cuerpo(p))
    r.ok("sin dependencias en liveness") if "dependencies" not in claves else r.fallo(
        "sin dependencias en liveness", ",".join(sorted(claves)), n.CONTENT_MISMATCH
    )


def e073(ctx: n.Contexto) -> None:
    r = ctx.reg("E073")
    p = ctx.pedir("E073")
    if not _exigir(ctx, "E073", p, r, (200, 503)):
        return
    if p.codigo == 200:
        r.ok("status ok|degraded", str(p.campo("status"))) if p.campo("status") in ("ok", "degraded") else r.fallo(
            "status ok|degraded", str(p.campo("status")), n.CONTENT_MISMATCH
        )
        r.ok("readiness lista") if p.campo("status") == "ok" else r.bloqueo(
            "readiness ok", f"status={p.campo('status')}", n.BLOCKED_DEPENDENCY
        )
    else:
        r.bloqueo("readiness 503", "dependencia degradada en la ventana", n.BLOCKED_DEPENDENCY)


# --------------------------------------------------------------------------- #
# Autenticación
# --------------------------------------------------------------------------- #

def e001(ctx: n.Contexto) -> None:
    r = ctx.reg("E001")
    if not _mutaciones(ctx, r, "E001"):
        return
    aviso = ctx.notice_version()
    if not aviso:
        r.bloqueo("versión del aviso disponible", "E015 no respondió y NEXUM_NOTICE_VERSION está vacío", n.BLOCKED_EXTERNAL)
        return
    for sesion, etiqueta in ((ctx.a, "A"), (ctx.b, "B")):
        cuerpo = {
            "name": f"Cuenta sintética {etiqueta}",
            "email": sesion.email,
            "password": sesion.password,
            "income": {"amount": "12000.00", "frequency": "monthly"},
            "recurringExpenses": [],
            "privacy": {"accepted": True, "noticeVersion": aviso},
            "client": {"locale": "es-MX", "timeZone": "America/Mexico_City"},
        }
        clave = n.idempotencia()
        p = ctx.pedir("E001", cuerpo=cuerpo, idempotencia=clave)
        if _error_de_red(ctx, "E001", p, r):
            return
        if p.codigo == 409:
            r.bloqueo(
                "registro 201",
                f"cuenta {etiqueta} ya existente; no se recrea ni se adivina su contraseña",
                n.BLOCKED_EXTERNAL,
            )
        elif _exigir(ctx, "E001", p, r, (201,)):
            r.ok(f"registro 201 cuenta {etiqueta}")
            user_id = p.campo("user", "id")
            if isinstance(user_id, str) and user_id:
                sesion.user_id = user_id
                r.ok(f"userId presente {etiqueta}")
            else:
                r.fallo(f"userId presente {etiqueta}", "ausente", n.CONTENT_MISMATCH)
            replay = ctx.pedir("E001", cuerpo=cuerpo, idempotencia=clave)
            if _exigir(ctx, "E001", replay, r, (201,)):
                r.ok(f"replay 201 {etiqueta}")
                r.ok(f"cabecera de replay {etiqueta}") if "x-idempotent-replay" in replay.cabeceras else r.fallo(
                    f"cabecera de replay {etiqueta}", "ausente", n.CONTENT_MISMATCH
                )
            distinto = dict(cuerpo)
            distinto["name"] = f"Cuenta sintética {etiqueta} 2"
            conflicto = ctx.pedir("E001", cuerpo=distinto, idempotencia=clave)
            if _exigir(ctx, "E001", conflicto, r, (409,)):
                r.ok(f"409 con otra carga y la misma clave {etiqueta}")
    if r.bloqueado() and any(c["check"].startswith("registro 201") for c in r.checks):
        ctx.bloqueos.append("E001: cuenta preexistente")


def e002(ctx: n.Contexto) -> None:
    r = ctx.reg("E002")
    if not _mutaciones(ctx, r, "E002"):
        return
    for sesion, etiqueta in ((ctx.a, "A"), (ctx.b, "B")):
        p = ctx.pedir(
            "E002",
            cuerpo={"email": sesion.email, "password": sesion.password, "deviceId": sesion.device_id},
        )
        if not _exigir(ctx, "E002", p, r, (200,)):
            continue
        ctx.cargar_sesion(sesion, p)
        r.ok(f"login {etiqueta} 200")
        r.ok(f"tokenType Bearer {etiqueta}") if (p.campo("tokenType") or "") == "Bearer" else r.fallo(
            f"tokenType Bearer {etiqueta}", str(p.campo("tokenType")), n.CONTENT_MISMATCH
        )
        r.ok(f"expiresIn 900 {etiqueta}") if p.campo("expiresIn") == 900 else r.fallo(
            f"expiresIn 900 {etiqueta}", str(p.campo("expiresIn")), n.CONTENT_MISMATCH
        )
        r.ok(f"session.id == sessionId {etiqueta}") if p.campo("session", "id") == p.campo("sessionId") else r.fallo(
            f"session.id == sessionId {etiqueta}", "no coinciden", n.CONTENT_MISMATCH
        )
    p = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": "Incorrecta#Sintetica#2026"})
    if _exigir(ctx, "E002", p, r, (401,)):
        r.ok("401 con contraseña incorrecta")
        r.ok("401 sin distinguir motivo") if p.campo("error", "code") else r.fallo(
            "401 sin distinguir motivo", "falta code", n.CONTENT_MISMATCH
        )
    if not ctx.a.access:
        ctx.bloqueos.append("E002: sin sesión A")


def e003(ctx: n.Contexto) -> None:
    r = ctx.reg("E003")
    if not _mutaciones(ctx, r, "E003"):
        return
    if not _sesion_a(ctx, r):
        return
    original = ctx.a.refresh
    if not original:
        r.bloqueo("refresh disponible", "la sesión no tiene refresh", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E003", cuerpo={"refreshToken": original})
    if not _exigir(ctx, "E003", p, r, (200,)):
        return
    nuevo_access = p.campo("accessToken") or ""
    nuevo_refresh = p.campo("refreshToken") or ""
    r.ok("tokens nuevos") if nuevo_access and nuevo_refresh and nuevo_refresh != original else r.fallo(
        "tokens nuevos", "no rotaron", n.CONTENT_MISMATCH
    )
    r.ok("misma sesión") if (p.campo("session", "id") or p.campo("sessionId")) == ctx.a.session_id else r.fallo(
        "misma sesión", "sessionId distinto", n.CONTENT_MISMATCH
    )
    ctx.a.access, ctx.a.refresh = nuevo_access, nuevo_refresh
    ctx.secretos.registrar("token-acceso", nuevo_access)
    ctx.secretos.registrar("token-refresh", nuevo_refresh)
    uso = ctx.pedir("E012", token=ctx.a.access)
    if _exigir(ctx, "E012", uso, r, (200,)):
        r.ok("access nuevo usable")
    replay = ctx.pedir("E003", cuerpo={"refreshToken": original})
    if _exigir(ctx, "E003", replay, r, (409,)):
        r.ok("replay del refresh consumido 409")
    r.ok("tokens fuera de resultados")  # la auditoría final lo comprueba de nuevo


def e004(ctx: n.Contexto) -> None:
    r = ctx.reg("E004")
    if not _mutaciones(ctx, r, "E004"):
        return
    if not _sesion_a(ctx, r):
        return
    login = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": ctx.a.password})
    if not _exigir(ctx, "E002", login, r, (200,)):
        r.bloqueo("sesión auxiliar", "no se pudo crear la sesión a cerrar", n.BLOCKED_DEPENDENCY)
        return
    aux = login.campo("accessToken") or ""
    aux_refresh = login.campo("refreshToken") or ""
    ctx.secretos.registrar("token-acceso", aux)
    ctx.secretos.registrar("token-refresh", aux_refresh)
    p = ctx.pedir("E004", token=aux)
    if _exigir(ctx, "E004", p, r, (204,)):
        r.ok("logout 204")
        r.ok("cuerpo vacío") if not (p.texto or "").strip() else r.fallo(
            "cuerpo vacío", f"len={len(p.texto)}", n.CONTENT_MISMATCH
        )
    refresh = ctx.pedir("E003", cuerpo={"refreshToken": aux_refresh})
    if _exigir(ctx, "E003", refresh, r, (401,)):
        r.ok("refresh posterior 401")
    acceso = ctx.pedir("E012", token=aux)
    if _exigir(ctx, "E012", acceso, r, (401,)):
        r.ok("access posterior 401")


def e007(ctx: n.Contexto) -> None:
    r = ctx.reg("E007")
    if not _mutaciones(ctx, r, "E007"):
        return
    if not _sesion_a(ctx, r):
        return
    anterior = ctx.a.password
    access_anterior = ctx.a.access
    refresh_anterior = ctx.a.refresh
    nueva = "Nx7!" + n.idempotencia()[:20]
    ctx.secretos.registrar("contraseña", nueva)
    incorrecta = ctx.pedir(
        "E007",
        token=ctx.a.access,
        cuerpo={"currentPassword": "NoEsLaActual#2026", "newPassword": nueva},
    )
    if _exigir(ctx, "E007", incorrecta, r, (422,)):
        r.ok("anterior incorrecta rechazada 422")
    cambio = ctx.pedir(
        "E007",
        token=ctx.a.access,
        cuerpo={"currentPassword": anterior, "newPassword": nueva},
    )
    if not _exigir(ctx, "E007", cambio, r, (200,)):
        return
    r.ok("cambio aceptado 200")
    r.ok("requestId presente") if cambio.campo("requestId") else r.fallo(
        "requestId presente", "ausente", n.CONTENT_MISMATCH
    )
    viejo_login = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": anterior})
    if _exigir(ctx, "E002", viejo_login, r, (401,)):
        r.ok("login con la contraseña anterior 401")
    nuevo_login = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": nueva})
    if _exigir(ctx, "E002", nuevo_login, r, (200,)):
        r.ok("login con la contraseña nueva 200")
        ctx.cargar_sesion(ctx.a, nuevo_login)
    ctx.a.password = nueva
    if ctx.a.password_origen == "generado":
        ctx.advertencias.append(
            "E007 cambió la contraseña generada en memoria: para reanudar otra fase exporte NEXUM_PASSWORD_A"
        )
    viejo_access = ctx.pedir("E012", token=access_anterior)
    if _exigir(ctx, "E012", viejo_access, r, (401,)):
        r.ok("contexto invalidado: access anterior 401")
    if refresh_anterior:
        viejo_refresh = ctx.pedir("E003", cuerpo={"refreshToken": refresh_anterior})
        if _exigir(ctx, "E003", viejo_refresh, r, (401,)):
            r.ok("contexto invalidado: refresh anterior 401")
    nuevo_access = ctx.pedir("E012", token=ctx.a.access)
    if _exigir(ctx, "E012", nuevo_access, r, (200,)):
        r.ok("contexto actualizado: access nuevo 200")
    sesiones = ctx.pedir("E008", token=ctx.a.access)
    if _exigir(ctx, "E008", sesiones, r, (200,)):
        r.ok("contexto de sesiones coherente") if any(
            isinstance(s, dict) and s.get("id") == ctx.a.session_id for s in _lista(sesiones)
        ) else r.fallo("contexto de sesiones coherente", "sesión actual ausente", n.CONTENT_MISMATCH)


def e008(ctx: n.Contexto) -> None:
    r = ctx.reg("E008")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E008", token=ctx.a.access)
    if not _exigir(ctx, "E008", p, r, (200,)):
        return
    sesiones = _lista(p)
    r.ok("sesiones no vacías", f"{len(sesiones)}") if sesiones else r.fallo(
        "sesiones no vacías", "items vacío", n.CONTENT_MISMATCH
    )
    actual = next((s for s in sesiones if isinstance(s, dict) and s.get("current") is True), None)
    r.ok("sesión actual marcada") if actual else r.fallo(
        "sesión actual marcada", "ninguna con current=true", n.CONTENT_MISMATCH
    )
    if actual:
        r.ok("current coincide con el token") if actual.get("id") == ctx.a.session_id else r.fallo(
            "current coincide con el token", "id distinto", n.CONTENT_MISMATCH
        )
    r.ok("sin tokens en el listado") if "refreshToken" not in json.dumps(p.cuerpo) else r.fallo(
        "sin tokens en el listado", "aparece refreshToken", n.CONTENT_MISMATCH
    )


def e009(ctx: n.Contexto) -> None:
    r = ctx.reg("E009")
    if not _mutaciones(ctx, r, "E009"):
        return
    if not _sesion_a(ctx, r):
        return
    if not _sesion_b(ctx, r):
        return
    login = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": ctx.a.password})
    if not _exigir(ctx, "E002", login, r, (200,)):
        r.bloqueo("sesión secundaria", "no se creó", n.BLOCKED_DEPENDENCY)
        return
    objetivo = login.campo("sessionId") or login.campo("session", "id")
    ctx.secretos.registrar("token-acceso", login.campo("accessToken") or "")
    if not isinstance(objetivo, str) or not objetivo:
        r.bloqueo("sesión secundaria", "sin sessionId", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E009", ruta=f"/api/v1/auth/sessions/{objetivo}", token=ctx.a.access)
    if _exigir(ctx, "E009", p, r, (204,)):
        r.ok("revocación 204")
        r.ok("cuerpo vacío") if not (p.texto or "").strip() else r.fallo(
            "cuerpo vacío", f"len={len(p.texto)}", n.CONTENT_MISMATCH
        )
    lista = ctx.pedir("E008", token=ctx.a.access)
    if _exigir(ctx, "E008", lista, r, (200,)):
        revocada = next((s for s in _lista(lista) if isinstance(s, dict) and s.get("id") == objetivo), None)
        r.ok("revoked=true persistido") if isinstance(revocada, dict) and revocada.get("revoked") is True else r.fallo(
            "revoked=true persistido", "no aparece revocada", n.CONTENT_MISMATCH
        )
    ajeno = ctx.pedir("E009", ruta=f"/api/v1/auth/sessions/{objetivo}", token=ctx.b.access)
    if _exigir(ctx, "E009", ajeno, r, (404,)):
        r.ok("sesión de A invisible para B")
    inexistente = ctx.pedir("E009", ruta="/api/v1/auth/sessions/00000000-0000-4000-8000-000000000000", token=ctx.a.access)
    if _exigir(ctx, "E009", inexistente, r, (404,)):
        r.ok("404 sesión inexistente")


def e010(ctx: n.Contexto) -> None:
    r = ctx.reg("E010")
    if not _mutaciones(ctx, r, "E010"):
        return
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E010", token=ctx.a.access, cuerpo={})
    if _exigir(ctx, "E010", p, r, (202,)):
        r.ok("solicitud 202")
        r.ok("requestId presente") if p.campo("requestId") else r.fallo(
            "requestId presente", "ausente", n.CONTENT_MISMATCH
        )
        r.ok("sin token en la respuesta") if "token" not in json.dumps(_cuerpo(p)).lower() else r.fallo(
            "sin token en la respuesta", "aparece token", n.CONTENT_MISMATCH
        )


def e011(ctx: n.Contexto) -> None:
    r = ctx.reg("E011")
    if not _mutaciones(ctx, r, "E011"):
        return
    token = ctx.opciones.verify_token
    if not token:
        r.bloqueo(
            "token de verificación aportado",
            "defina NEXUM_VERIFY_TOKEN o ejecute con terminal interactiva; no se reenvía E010",
            n.BLOCKED_EXTERNAL,
        )
        return
    p = ctx.pedir("E011", cuerpo={"token": token})
    if not _exigir(ctx, "E011", p, r, (200,)):
        return
    r.ok("confirmación 200")
    r.ok("emailVerified=true") if p.campo("emailVerified") is True else r.fallo(
        "emailVerified=true", str(p.campo("emailVerified")), n.CONTENT_MISMATCH
    )
    replay = ctx.pedir("E011", cuerpo={"token": token})
    if _exigir(ctx, "E011", replay, r, (409,)):
        r.ok("replay 409")
    if _sesion_a(ctx, r):
        perfil = ctx.pedir("E012", token=ctx.a.access)
        if _exigir(ctx, "E012", perfil, r, (200,)):
            r.ok("perfil refleja emailVerified") if perfil.campo("emailVerified") is True else r.fallo(
                "perfil refleja emailVerified", str(perfil.campo("emailVerified")), n.CONTENT_MISMATCH
            )


# --------------------------------------------------------------------------- #
# Perfil
# --------------------------------------------------------------------------- #

def e012(ctx: n.Contexto) -> None:
    r = ctx.reg("E012")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E012", token=ctx.a.access)
    if not _exigir(ctx, "E012", p, r, (200,)):
        return
    identificador = p.campo("id")
    if isinstance(identificador, str) and identificador:
        ctx.a.user_id = identificador
        r.ok("id presente")
    else:
        r.fallo("id presente", "ausente", n.CONTENT_MISMATCH)
    correo = p.campo("email")
    r.ok("email coincide con la sesión") if isinstance(correo, str) and correo.lower() == ctx.a.email.lower() else r.fallo(
        "email coincide con la sesión", "no coincide", n.CONTENT_MISMATCH
    )
    version = p.campo("version")
    r.ok("version >= 1") if isinstance(version, int) and version >= 1 else r.fallo(
        "version >= 1", str(version), n.CONTENT_MISMATCH
    )
    ctx.datos["profile_etag"] = _etag(p)


def e013(ctx: n.Contexto) -> None:
    r = ctx.reg("E013")
    if not _mutaciones(ctx, r, "E013"):
        return
    if not _sesion_a(ctx, r):
        return
    sin = ctx.pedir("E013", token=ctx.a.access, cuerpo={"displayName": "Nombre sintético A"})
    if _exigir(ctx, "E013", sin, r, (428,)):
        r.ok("428 sin If-Match")
    etag = ctx.datos.get("profile_etag") or ""
    if not etag:
        perfil = ctx.pedir("E012", token=ctx.a.access)
        if _exigir(ctx, "E012", perfil, r, (200,)):
            etag = _etag(perfil)
    if not etag:
        r.bloqueo("ETag del perfil", "no se pudo obtener el validador", n.BLOCKED_DEPENDENCY)
        return
    ok = ctx.pedir(
        "E013", token=ctx.a.access, cuerpo={"displayName": "Nombre sintético A"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E013", ok, r, (200,)):
        r.ok("200 con If-Match")
        r.ok("displayName persistido") if ok.campo("displayName") == "Nombre sintético A" else r.fallo(
            "displayName persistido", "no coincide", n.CONTENT_MISMATCH
        )
        r.ok("versión incrementada") if ok.campo("version", default=0) >= 2 else r.fallo(
            "versión incrementada", str(ok.campo("version")), n.CONTENT_MISMATCH
        )
    obsoleto = ctx.pedir(
        "E013", token=ctx.a.access, cuerpo={"displayName": "Nombre sintético A2"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E013", obsoleto, r, (412,)):
        r.ok("412 con versión obsoleta")
    debil = ctx.pedir(
        "E013", token=ctx.a.access, cuerpo={"displayName": "Nombre sintético A3"}, cabeceras={"If-Match": 'W/"v2"'}
    )
    if _exigir(ctx, "E013", debil, r, (412,)):
        r.ok("412 con ETag débil")


def e016(ctx: n.Contexto) -> None:
    r = ctx.reg("E016")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E016", token=ctx.a.access)
    if not _exigir(ctx, "E016", p, r, (200,)):
        return
    items = _lista(p)
    r.ok("consentimientos listados", f"{len(items)}") if items else r.fallo(
        "consentimientos listados", "items vacío", n.CONTENT_MISMATCH
    )
    propósitos = {i.get("purpose") for i in items if isinstance(i, dict)}
    r.ok("consentimiento del aviso presente") if "privacyNotice" in propósitos else r.fallo(
        "consentimiento del aviso presente", str(sorted(propósitos)), n.CONTENT_MISMATCH
    )


def e017(ctx: n.Contexto) -> None:
    r = ctx.reg("E017")
    if not _mutaciones(ctx, r, "E017"):
        return
    if not _sesion_a(ctx, r):
        return
    aviso = ctx.notice_version()
    if not aviso:
        r.bloqueo("versión del aviso", "no disponible", n.BLOCKED_DEPENDENCY)
        return
    cuerpo = {"noticeVersion": aviso, "purpose": "analytics", "granted": True}
    p = ctx.pedir("E017", token=ctx.a.access, cuerpo=cuerpo)
    if not _exigir(ctx, "E017", p, r, (201,)):
        return
    consentimiento = p.campo("id")
    r.ok("201 con id") if consentimiento else r.fallo("201 con id", "ausente", n.CONTENT_MISMATCH)
    r.ok("Location presente") if "location" in p.cabeceras else r.fallo(
        "Location presente", "ausente", n.CONTENT_MISMATCH
    )
    lista = ctx.pedir("E016", token=ctx.a.access)
    if _exigir(ctx, "E016", lista, r, (200,)):
        r.ok("consentimiento persistido") if any(
            isinstance(i, dict) and i.get("id") == consentimiento for i in _lista(lista)
        ) else r.fallo("consentimiento persistido", "no aparece en el listado", n.CONTENT_MISMATCH)


def e018(ctx: n.Contexto) -> None:
    r = ctx.reg("E018")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E018", token=ctx.a.access)
    if not _exigir(ctx, "E018", p, r, (200,)):
        return
    r.ok("configured booleano") if isinstance(p.campo("configured"), bool) else r.fallo(
        "configured booleano", "ausente", n.CONTENT_MISMATCH
    )
    r.ok("done booleano") if isinstance(p.campo("done"), bool) else r.fallo(
        "done booleano", "ausente", n.CONTENT_MISMATCH
    )
    ctx.datos["onboarding_version"] = p.campo("version")


def e019(ctx: n.Contexto) -> None:
    r = ctx.reg("E019")
    if not _mutaciones(ctx, r, "E019"):
        return
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir(
        "E019",
        token=ctx.a.access,
        cuerpo={"completedSteps": ["profile", "income"], "notifications": False, "configured": True},
    )
    if not _exigir(ctx, "E019", p, r, (200,)):
        return
    r.ok("completedSteps persistido") if "profile" in (p.campo("completedSteps") or []) else r.fallo(
        "completedSteps persistido", "no coincide", n.CONTENT_MISMATCH
    )
    r.ok("configured persistido") if p.campo("configured") is True else r.fallo(
        "configured persistido", str(p.campo("configured")), n.CONTENT_MISMATCH
    )
    anterior = ctx.datos.get("onboarding_version")
    if isinstance(anterior, int) and isinstance(p.campo("version"), int):
        r.ok("versión incrementada") if p.campo("version") > anterior else r.fallo(
            "versión incrementada", f"{anterior}->{p.campo('version')}", n.CONTENT_MISMATCH
        )


def e068(ctx: n.Contexto) -> None:
    r = ctx.reg("E068")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E068", token=ctx.a.access)
    if not _exigir(ctx, "E068", p, r, (200,)):
        return
    r.ok("locale presente") if p.campo("locale") else r.fallo("locale presente", "ausente", n.CONTENT_MISMATCH)
    r.ok("timezone presente") if p.campo("timezone") else r.fallo(
        "timezone presente", "ausente", n.CONTENT_MISMATCH
    )
    ctx.datos["prefs_etag"] = _etag(p)


def e069(ctx: n.Contexto) -> None:
    r = ctx.reg("E069")
    if not _mutaciones(ctx, r, "E069"):
        return
    if not _sesion_a(ctx, r):
        return
    sin = ctx.pedir("E069", token=ctx.a.access, cuerpo={"locale": "es-MX"})
    if _exigir(ctx, "E069", sin, r, (428,)):
        r.ok("428 sin If-Match")
    etag = ctx.datos.get("prefs_etag") or ""
    if not etag:
        prefs = ctx.pedir("E068", token=ctx.a.access)
        if _exigir(ctx, "E068", prefs, r, (200,)):
            etag = _etag(prefs)
    if not etag:
        r.bloqueo("ETag de preferencias", "no disponible", n.BLOCKED_DEPENDENCY)
        return
    ok = ctx.pedir(
        "E069", token=ctx.a.access, cuerpo={"locale": "es-MX", "timezone": "America/Mexico_City"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E069", ok, r, (200,)):
        r.ok("200 con If-Match")
        r.ok("locale persistido") if ok.campo("locale") == "es-MX" else r.fallo(
            "locale persistido", str(ok.campo("locale")), n.CONTENT_MISMATCH
        )
        r.ok("timezone persistida") if ok.campo("timezone") == "America/Mexico_City" else r.fallo(
            "timezone persistida", str(ok.campo("timezone")), n.CONTENT_MISMATCH
        )
    obsoleto = ctx.pedir(
        "E069", token=ctx.a.access, cuerpo={"locale": "es-MX"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E069", obsoleto, r, (412,)):
        r.ok("412 con versión obsoleta")


# --------------------------------------------------------------------------- #
# Finanzas: ingreso y movimientos
# --------------------------------------------------------------------------- #

def e024(ctx: n.Contexto) -> None:
    r = ctx.reg("E024")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E024", token=ctx.a.access)
    if not _exigir(ctx, "E024", p, r, (200,)):
        return
    r.ok("amount decimal", str(p.campo("amount"))) if isinstance(p.campo("amount"), str) else r.fallo(
        "amount decimal", "no es string", n.CONTENT_MISMATCH
    )
    r.ok("currency MXN") if p.campo("currency") == "MXN" else r.fallo(
        "currency MXN", str(p.campo("currency")), n.CONTENT_MISMATCH
    )
    ctx.datos["income_etag"] = _etag(p)


def e025(ctx: n.Contexto) -> None:
    r = ctx.reg("E025")
    if not _mutaciones(ctx, r, "E025"):
        return
    if not _sesion_a(ctx, r):
        return
    cuerpo = {"amount": "12500.00", "currency": "MXN", "frequency": "monthly"}
    sin = ctx.pedir("E025", token=ctx.a.access, cuerpo=cuerpo)
    if _exigir(ctx, "E025", sin, r, (428,)):
        r.ok("428 sin If-Match")
    flotante = ctx.pedir(
        "E025",
        token=ctx.a.access,
        cuerpo={"amount": 12500.5, "currency": "MXN", "frequency": "monthly"},
        cabeceras={"If-Match": ctx.datos.get("income_etag") or '"v1"'},
        validar_payload=False,
    )
    if _exigir(ctx, "E025", flotante, r, (400, 422)):
        r.ok("número flotante rechazado")
    etag = ctx.datos.get("income_etag") or '"v1"'
    ok = ctx.pedir("E025", token=ctx.a.access, cuerpo=cuerpo, cabeceras={"If-Match": etag})
    if _exigir(ctx, "E025", ok, r, (200,)):
        r.ok("200 con If-Match")
        r.ok("importe exacto") if n.comparar_decimal(ok.campo("amount"), "12500.00") else r.fallo(
            "importe exacto", str(ok.campo("amount")), n.CONTENT_MISMATCH
        )
        obsoleto = ctx.pedir(
            "E025",
            token=ctx.a.access,
            cuerpo={"amount": "12600.00", "currency": "MXN", "frequency": "monthly"},
            cabeceras={"If-Match": etag},
        )
        if _exigir(ctx, "E025", obsoleto, r, (412,)):
            r.ok("412 con versión obsoleta")
    leido = ctx.pedir("E024", token=ctx.a.access)
    if _exigir(ctx, "E024", leido, r, (200,)):
        r.ok("persistencia del ingreso") if n.comparar_decimal(leido.campo("amount"), "12500.00") else r.fallo(
            "persistencia del ingreso", str(leido.campo("amount")), n.CONTENT_MISMATCH
        )


def e028(ctx: n.Contexto) -> None:
    r = ctx.reg("E028")
    if not _mutaciones(ctx, r, "E028"):
        return
    if not _sesion_a(ctx, r):
        return
    categoria = ctx.category_id()
    cuerpo = {
        "type": "expense",
        "amount": "150.00",
        "currency": "MXN",
        "categoryId": categoria,
        "description": "Gasto sintético de certificación",
        "occurredAt": n.iso_utc(),
    }
    clave = n.idempotencia()
    p = ctx.pedir("E028", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if not _exigir(ctx, "E028", p, r, (201,)):
        return
    mid = _identificador(p, "id")
    if not mid:
        r.fallo("id presente", "ausente", n.CONTENT_MISMATCH)
        return
    r.ok("movimiento 201")
    r.ok("Location presente") if "location" in p.cabeceras else r.fallo(
        "Location presente", "ausente", n.CONTENT_MISMATCH
    )
    ctx.datos.update({"movement_id": mid, "movement_etag": _etag(p), "movement_category": categoria})
    replay = ctx.pedir("E028", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if _exigir(ctx, "E028", replay, r, (201,)):
        r.ok("replay estable 201")
        r.ok("mismo id en el replay") if _identificador(replay, "id") == mid else r.fallo(
            "mismo id en el replay", "id distinto", n.CONTENT_MISMATCH
        )
    cambio = dict(cuerpo)
    cambio["amount"] = "151.00"
    conflicto = ctx.pedir("E028", token=ctx.a.access, cuerpo=cambio, idempotencia=clave)
    if _exigir(ctx, "E028", conflicto, r, (409,)):
        r.ok("409 con la misma clave y otra carga")


def e029(ctx: n.Contexto) -> None:
    r = ctx.reg("E029")
    if not _sesion_a(ctx, r) or not _sesion_b(ctx, r):
        return
    mid = ctx.datos.get("movement_id")
    if not mid:
        r.bloqueo("movimiento disponible", "E028 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E029", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access)
    if not _exigir(ctx, "E029", p, r, (200,)):
        return
    r.ok("importe exacto") if n.comparar_decimal(p.campo("amount"), "150.00") else r.fallo(
        "importe exacto", str(p.campo("amount")), n.CONTENT_MISMATCH
    )
    ctx.datos["movement_etag"] = _etag(p) or ctx.datos.get("movement_etag")
    ajeno = ctx.pedir("E029", ruta=f"/api/v1/movements/{mid}", token=ctx.b.access)
    if _exigir(ctx, "E029", ajeno, r, (404,)):
        r.ok("404 para otra cuenta (no 403 enumerable)")


def e027(ctx: n.Contexto) -> None:
    r = ctx.reg("E027")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E027", ruta="/api/v1/movements?limit=5", token=ctx.a.access)
    if not _exigir(ctx, "E027", p, r, (200,)):
        return
    items = _lista(p)
    mid = ctx.datos.get("movement_id")
    r.ok("movimiento listado") if any(isinstance(i, dict) and i.get("id") == mid for i in items) else r.fallo(
        "movimiento listado", "no aparece", n.CONTENT_MISMATCH
    )
    filtrado = ctx.pedir("E027", ruta="/api/v1/movements?type=expense", token=ctx.a.access)
    if _exigir(ctx, "E027", filtrado, r, (200,)):
        r.ok("filtro type=expense") if all(
            isinstance(i, dict) and i.get("type") == "expense" for i in _lista(filtrado)
        ) else r.fallo("filtro type=expense", "hay otro tipo", n.CONTENT_MISMATCH)
    invalido = ctx.pedir("E027", ruta="/api/v1/movements?type=invalido", token=ctx.a.access)
    if _exigir(ctx, "E027", invalido, r, (400, 422)):
        r.ok("type inválido rechazado")
    pagina = ctx.pedir("E027", ruta="/api/v1/movements?limit=1", token=ctx.a.access)
    if _exigir(ctx, "E027", pagina, r, (200,)):
        siguiente = pagina.campo("nextCursor")
        r.ok("nextCursor tipado") if siguiente is None or isinstance(siguiente, str) else r.fallo(
            "nextCursor tipado", "tipo inválido", n.CONTRACT_MISMATCH
        )


def e030(ctx: n.Contexto) -> None:
    r = ctx.reg("E030")
    if not _mutaciones(ctx, r, "E030"):
        return
    if not _sesion_a(ctx, r):
        return
    mid = ctx.datos.get("movement_id")
    etag = ctx.datos.get("movement_etag") or '"v1"'
    if not mid:
        r.bloqueo("movimiento disponible", "E028 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    sin = ctx.pedir("E030", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access, cuerpo={"description": "Gasto 2"})
    if _exigir(ctx, "E030", sin, r, (428,)):
        r.ok("428 sin If-Match")
    debil = ctx.pedir(
        "E030", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access, cuerpo={"description": "Gasto 2"}, cabeceras={"If-Match": 'W/"v1"'}
    )
    if _exigir(ctx, "E030", debil, r, (412,)):
        r.ok("412 con ETag débil")
    ok = ctx.pedir(
        "E030", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access, cuerpo={"description": "Gasto 2"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E030", ok, r, (200,)):
        r.ok("200 con If-Match")
        r.ok("descripción persistida") if ok.campo("description") == "Gasto 2" else r.fallo(
            "descripción persistida", str(ok.campo("description")), n.CONTENT_MISMATCH
        )
        r.ok("importe intacto") if n.comparar_decimal(ok.campo("amount"), "150.00") else r.fallo(
            "importe intacto", str(ok.campo("amount")), n.CONTENT_MISMATCH
        )
        nuevo_etag = _etag(ok) or etag
        obsoleto = ctx.pedir(
            "E030", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access, cuerpo={"description": "Gasto 3"}, cabeceras={"If-Match": etag}
        )
        if _exigir(ctx, "E030", obsoleto, r, (412,)):
            r.ok("412 con versión obsoleta")
        movido = ctx.pedir(
            "E030",
            ruta=f"/api/v1/movements/{mid}",
            token=ctx.a.access,
            cuerpo={"occurredAt": n.iso_utc(n.ahora_utc() - timedelta(days=1))},
            cabeceras={"If-Match": nuevo_etag},
        )
        if _exigir(ctx, "E030", movido, r, (200,)):
            r.ok("cambio de occurredAt mueve el SK")
            lectura = ctx.pedir("E029", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access)
            if _exigir(ctx, "E029", lectura, r, (200,)):
                r.ok("movimiento legible tras mover occurredAt")
        ctx.datos["movement_etag"] = nuevo_etag


def e031(ctx: n.Contexto) -> None:
    r = ctx.reg("E031")
    if not _mutaciones(ctx, r, "E031"):
        return
    if not _sesion_a(ctx, r):
        return
    mid = ctx.datos.get("movement_id")
    if not mid:
        r.bloqueo("movimiento disponible", "E028 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E031", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access)
    if _exigir(ctx, "E031", p, r, (204,)):
        r.ok("204 sin cuerpo")
        r.ok("cuerpo vacío") if not (p.texto or "").strip() else r.fallo(
            "cuerpo vacío", f"len={len(p.texto)}", n.CONTENT_MISMATCH
        )
    lectura = ctx.pedir("E029", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access)
    if _exigir(ctx, "E029", lectura, r, (404,)):
        r.ok("lectura posterior 404")
    replay = ctx.pedir("E031", ruta=f"/api/v1/movements/{mid}", token=ctx.a.access)
    if _exigir(ctx, "E031", replay, r, (204,)):
        r.ok("replay idempotente 204")
        if not (replay.texto or "").strip():
            r.ok("replay sin cuerpo")
        else:
            r.fallo("replay sin cuerpo", f"len={len(replay.texto)}", n.CONTENT_MISMATCH)


# --------------------------------------------------------------------------- #
# Finanzas: recurrentes
# --------------------------------------------------------------------------- #

def e033(ctx: n.Contexto) -> None:
    r = ctx.reg("E033")
    if not _mutaciones(ctx, r, "E033"):
        return
    if not _sesion_a(ctx, r):
        return
    cuerpo = {
        "name": "Recurrente sintético",
        "amount": "699.00",
        "currency": "MXN",
        "frequency": "monthly",
        "nextDueDate": n.fecha_desplazada(5),
        "type": "expense",
        "startDate": n.fecha_desplazada(-30),
        "dueDay": 5,
    }
    clave = n.idempotencia()
    p = ctx.pedir("E033", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if not _exigir(ctx, "E033", p, r, (201,)):
        return
    rid = _identificador(p, "id")
    if not rid:
        r.fallo("id presente", "ausente", n.CONTENT_MISMATCH)
        return
    r.ok("201 con id")
    r.ok("Location presente") if "location" in p.cabeceras else r.fallo(
        "Location presente", "ausente", n.CONTENT_MISMATCH
    )
    ctx.datos.update({"recurring_id": rid, "recurring_etag": _etag(p)})
    replay = ctx.pedir("E033", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if _exigir(ctx, "E033", replay, r, (201,)):
        r.ok("replay estable 201")


def e034(ctx: n.Contexto) -> None:
    r = ctx.reg("E034")
    if not _sesion_a(ctx, r):
        return
    rid = ctx.datos.get("recurring_id")
    if not rid:
        r.bloqueo("recurrente disponible", "E033 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E034", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access)
    if not _exigir(ctx, "E034", p, r, (200,)):
        return
    r.ok("importe exacto") if n.comparar_decimal(p.campo("amount"), "699.00") else r.fallo(
        "importe exacto", str(p.campo("amount")), n.CONTENT_MISMATCH
    )
    r.ok("nextDueDate fecha") if isinstance(p.campo("nextDueDate"), str) else r.fallo(
        "nextDueDate fecha", "ausente", n.CONTENT_MISMATCH
    )
    ctx.datos["recurring_etag"] = _etag(p) or ctx.datos.get("recurring_etag")


def e032(ctx: n.Contexto) -> None:
    r = ctx.reg("E032")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E032", token=ctx.a.access)
    if not _exigir(ctx, "E032", p, r, (200,)):
        return
    rid = ctx.datos.get("recurring_id")
    r.ok("recurrente listado") if any(
        isinstance(i, dict) and i.get("id") == rid for i in _lista(p)
    ) else r.fallo("recurrente listado", "no aparece", n.CONTENT_MISMATCH)
    filtrado = ctx.pedir("E032", ruta="/api/v1/recurring-payments?active=true", token=ctx.a.access)
    if _exigir(ctx, "E032", filtrado, r, (200,)):
        r.ok("filtro active") if all(
            isinstance(i, dict) and i.get("active") is True for i in _lista(filtrado)
        ) else r.fallo("filtro active", "hay inactivos", n.CONTENT_MISMATCH)


def e035(ctx: n.Contexto) -> None:
    r = ctx.reg("E035")
    if not _mutaciones(ctx, r, "E035"):
        return
    if not _sesion_a(ctx, r):
        return
    rid = ctx.datos.get("recurring_id")
    etag = ctx.datos.get("recurring_etag") or '"v1"'
    if not rid:
        r.bloqueo("recurrente disponible", "E033 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    sin = ctx.pedir("E035", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access, cuerpo={"name": "Recurrente 2"})
    if _exigir(ctx, "E035", sin, r, (428,)):
        r.ok("428 sin If-Match")
    ok = ctx.pedir(
        "E035", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access, cuerpo={"name": "Recurrente 2"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E035", ok, r, (200,)):
        r.ok("200 con If-Match")
        r.ok("nombre persistido") if ok.campo("name") == "Recurrente 2" else r.fallo(
            "nombre persistido", str(ok.campo("name")), n.CONTENT_MISMATCH
        )
        inmutable = ctx.pedir(
            "E035", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access,
            cuerpo={"frequency": "weekly"}, cabeceras={"If-Match": _etag(ok) or etag},
        )
        if _exigir(ctx, "E035", inmutable, r, (400,)):
            r.ok("frequency no editable rechazada")
        ctx.datos["recurring_etag"] = _etag(ok) or etag
    obsoleto = ctx.pedir(
        "E035", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access, cuerpo={"name": "Recurrente 3"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E035", obsoleto, r, (412,)):
        r.ok("412 con versión obsoleta")


def e037(ctx: n.Contexto) -> None:
    r = ctx.reg("E037")
    if not _mutaciones(ctx, r, "E037"):
        return
    if not _sesion_a(ctx, r):
        return
    rid = ctx.datos.get("recurring_id")
    if not rid:
        r.bloqueo("recurrente disponible", "E033 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    cuerpo = {"amount": "699.00", "currency": "MXN", "paidAt": n.iso_utc()}
    clave = n.idempotencia()
    p = ctx.pedir("E037", ruta=f"/api/v1/recurring-payments/{rid}/payments", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if not _exigir(ctx, "E037", p, r, (200,)):
        return
    movimiento = _identificador(p, "id")
    r.ok("pago genera movimiento") if movimiento else r.fallo(
        "pago genera movimiento", "sin id", n.CONTENT_MISMATCH
    )
    r.ok("importe exacto") if n.comparar_decimal(p.campo("amount"), "699.00") else r.fallo(
        "importe exacto", str(p.campo("amount")), n.CONTENT_MISMATCH
    )
    r.ok("recurringPaymentId correcto") if p.campo("recurringPaymentId") == rid else r.fallo(
        "recurringPaymentId correcto", str(p.campo("recurringPaymentId")), n.CONTENT_MISMATCH
    )
    r.ok("categoría de gasto existente") if p.campo("categoryId") == "otros" else r.fallo(
        "categoría de gasto existente", str(p.campo("categoryId")), n.CONTENT_MISMATCH
    )
    replay = ctx.pedir("E037", ruta=f"/api/v1/recurring-payments/{rid}/payments", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if _exigir(ctx, "E037", replay, r, (200,)):
        r.ok("replay devuelve el mismo movimiento") if _identificador(replay, "id") == movimiento else r.fallo(
            "replay devuelve el mismo movimiento", "id distinto", n.CONTENT_MISMATCH
        )
    distinto = dict(cuerpo)
    distinto["amount"] = "700.00"
    conflicto = ctx.pedir("E037", ruta=f"/api/v1/recurring-payments/{rid}/payments", token=ctx.a.access, cuerpo=distinto, idempotencia=clave)
    if _exigir(ctx, "E037", conflicto, r, (409,)):
        r.ok("409 misma clave y otra carga")


def e036(ctx: n.Contexto) -> None:
    r = ctx.reg("E036")
    if not _mutaciones(ctx, r, "E036"):
        return
    if not _sesion_a(ctx, r):
        return
    rid = ctx.datos.get("recurring_id")
    if not rid:
        r.bloqueo("recurrente disponible", "E033 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E036", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access)
    if _exigir(ctx, "E036", p, r, (204,)):
        r.ok("204 sin cuerpo")
    lectura = ctx.pedir("E034", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access)
    if _exigir(ctx, "E034", lectura, r, (404,)):
        r.ok("lectura posterior 404")
    replay = ctx.pedir("E036", ruta=f"/api/v1/recurring-payments/{rid}", token=ctx.a.access)
    if _exigir(ctx, "E036", replay, r, (204, 404)):
        r.ok("replay idempotente")


# --------------------------------------------------------------------------- #
# Finanzas: metas y aportaciones
# --------------------------------------------------------------------------- #

def e039(ctx: n.Contexto) -> None:
    r = ctx.reg("E039")
    if not _mutaciones(ctx, r, "E039"):
        return
    if not _sesion_a(ctx, r):
        return
    cuerpo = {
        "name": "Meta sintética",
        "targetAmount": "5000.00",
        "currency": "MXN",
        "targetDate": n.fecha_desplazada(120),
        "initialSavedAmount": "100.00",
    }
    clave = n.idempotencia()
    p = ctx.pedir("E039", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if not _exigir(ctx, "E039", p, r, (201,)):
        return
    gid = _identificador(p, "id")
    if not gid:
        r.fallo("id presente", "ausente", n.CONTENT_MISMATCH)
        return
    r.ok("201 con id")
    r.ok("openingSaved expuesto") if n.comparar_decimal(p.campo("openingSaved"), "100.00") else r.fallo(
        "openingSaved expuesto", str(p.campo("openingSaved")), n.CONTENT_MISMATCH
    )
    r.ok("savedAmount inicial") if n.comparar_decimal(p.campo("savedAmount"), "100.00") else r.fallo(
        "savedAmount inicial", str(p.campo("savedAmount")), n.CONTENT_MISMATCH
    )
    ctx.datos.update({"goal_id": gid, "goal_etag": _etag(p)})
    replay = ctx.pedir("E039", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if _exigir(ctx, "E039", replay, r, (201,)):
        r.ok("replay estable 201")
    distinto = dict(cuerpo)
    distinto["name"] = "Meta sintética 2"
    conflicto = ctx.pedir("E039", token=ctx.a.access, cuerpo=distinto, idempotencia=clave)
    if _exigir(ctx, "E039", conflicto, r, (409,)):
        r.ok("409 misma clave y otra carga")


def e040(ctx: n.Contexto) -> None:
    r = ctx.reg("E040")
    if not _sesion_a(ctx, r) or not _sesion_b(ctx, r):
        return
    gid = ctx.datos.get("goal_id")
    if not gid:
        r.bloqueo("meta disponible", "E039 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E040", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access)
    if not _exigir(ctx, "E040", p, r, (200,)):
        return
    r.ok("openingSaved visible") if n.comparar_decimal(p.campo("openingSaved"), "100.00") else r.fallo(
        "openingSaved visible", str(p.campo("openingSaved")), n.CONTENT_MISMATCH
    )
    ctx.datos["goal_etag"] = _etag(p) or ctx.datos.get("goal_etag")
    ajeno = ctx.pedir("E040", ruta=f"/api/v1/goals/{gid}", token=ctx.b.access)
    if _exigir(ctx, "E040", ajeno, r, (404,)):
        r.ok("404 para otra cuenta")


def e038(ctx: n.Contexto) -> None:
    r = ctx.reg("E038")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E038", token=ctx.a.access)
    if not _exigir(ctx, "E038", p, r, (200,)):
        return
    totals = p.campo("totals")
    r.ok("totals presente") if isinstance(totals, dict) else r.fallo(
        "totals presente", "ausente", n.CONTENT_MISMATCH
    )
    if isinstance(totals, dict):
        r.ok("activeCount >= 1") if isinstance(totals.get("activeCount"), int) and totals["activeCount"] >= 1 else r.fallo(
            "activeCount >= 1", str(totals.get("activeCount")), n.CONTENT_MISMATCH
        )
        r.ok("saved decimal") if n.decimal_exacto(totals.get("saved")) is not None else r.fallo(
            "saved decimal", str(totals.get("saved")), n.CONTENT_MISMATCH
        )
    filtrado = ctx.pedir("E038", ruta="/api/v1/goals?status=active", token=ctx.a.access)
    if _exigir(ctx, "E038", filtrado, r, (200,)):
        r.ok("filtro status=active") if all(
            isinstance(i, dict) and i.get("status") == "active" for i in _lista(filtrado)
        ) else r.fallo("filtro status=active", "hay otro estado", n.CONTENT_MISMATCH)


def e041(ctx: n.Contexto) -> None:
    r = ctx.reg("E041")
    if not _mutaciones(ctx, r, "E041"):
        return
    if not _sesion_a(ctx, r):
        return
    gid = ctx.datos.get("goal_id")
    etag = ctx.datos.get("goal_etag") or '"v1"'
    if not gid:
        r.bloqueo("meta disponible", "E039 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    sin = ctx.pedir("E041", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access, cuerpo={"targetAmount": "6000.00"})
    if _exigir(ctx, "E041", sin, r, (428,)):
        r.ok("428 sin If-Match")
    vacio = ctx.pedir("E041", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access, cuerpo={}, cabeceras={"If-Match": etag})
    if _exigir(ctx, "E041", vacio, r, (200,)):
        r.ok("{} es no-op") if _etag(vacio) == etag else r.fallo(
            "{} es no-op", f"{_etag(vacio)} != {etag}", n.CONTENT_MISMATCH
        )
    ok = ctx.pedir(
        "E041", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access, cuerpo={"targetAmount": "6000.00"}, cabeceras={"If-Match": etag}
    )
    if _exigir(ctx, "E041", ok, r, (200,)):
        r.ok("200 con If-Match")
        r.ok("targetAmount persistido") if n.comparar_decimal(ok.campo("targetAmount"), "6000.00") else r.fallo(
            "targetAmount persistido", str(ok.campo("targetAmount")), n.CONTENT_MISMATCH
        )
        debajo = ctx.pedir(
            "E041", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access,
            cuerpo={"targetAmount": "50.00"}, cabeceras={"If-Match": _etag(ok) or etag},
        )
        if _exigir(ctx, "E041", debajo, r, (422,)):
            r.ok("targetAmount por debajo de savedAmount rechazado")
        ctx.datos["goal_etag"] = _etag(ok) or etag


def e043(ctx: n.Contexto) -> None:
    r = ctx.reg("E043")
    if not _sesion_a(ctx, r):
        return
    gid = ctx.datos.get("goal_id")
    if not gid:
        r.bloqueo("meta disponible", "E039 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    cuerpo = {
        "amount": "300.00",
        "currency": "MXN",
        "allocations": [{"goalId": gid, "amount": "300.00"}],
    }
    p = ctx.pedir("E043", token=ctx.a.access, cuerpo=cuerpo)
    if not _exigir(ctx, "E043", p, r, (200,)):
        return
    r.ok("remaining 0.00") if n.comparar_decimal(p.campo("remaining"), "0.00") else r.fallo(
        "remaining 0.00", str(p.campo("remaining")), n.CONTENT_MISMATCH
    )
    r.ok("canCommit true") if p.campo("canCommit") is True else r.fallo(
        "canCommit true", str(p.campo("canCommit")), n.CONTENT_MISMATCH
    )
    incompleta = dict(cuerpo)
    incompleta["allocations"] = [{"goalId": gid, "amount": "100.00"}]
    q = ctx.pedir("E043", token=ctx.a.access, cuerpo=incompleta)
    if _exigir(ctx, "E043", q, r, (422,)):
        r.ok("suma incompleta rechazada")
    excedida = dict(cuerpo)
    excedida["allocations"] = [{"goalId": gid, "amount": "400.00"}]
    t = ctx.pedir("E043", token=ctx.a.access, cuerpo=excedida)
    if _exigir(ctx, "E043", t, r, (422,)):
        r.ok("suma excedida rechazada")


def e044(ctx: n.Contexto) -> None:
    r = ctx.reg("E044")
    if not _mutaciones(ctx, r, "E044"):
        return
    if not _sesion_a(ctx, r):
        return
    gid = ctx.datos.get("goal_id")
    if not gid:
        r.bloqueo("meta disponible", "E039 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    cuerpo = {
        "amount": "300.00",
        "currency": "MXN",
        "allocations": [{"goalId": gid, "amount": "300.00"}],
        "occurredAt": n.iso_utc(),
    }
    clave = n.idempotencia()
    p = ctx.pedir("E044", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if not _exigir(ctx, "E044", p, r, (201,)):
        return
    cid = _identificador(p, "id")
    if not cid:
        r.fallo("id presente", "ausente", n.CONTENT_MISMATCH)
        return
    r.ok("201 con id")
    r.ok("status active") if p.campo("status") == "active" else r.fallo(
        "status active", str(p.campo("status")), n.CONTENT_MISMATCH
    )
    ctx.datos["contribution_id"] = cid
    replay = ctx.pedir("E044", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if _exigir(ctx, "E044", replay, r, (201,)):
        r.ok("replay estable y mismo id") if _identificador(replay, "id") == cid else r.fallo(
            "replay estable y mismo id", "id distinto", n.CONTENT_MISMATCH
        )
    distinta = dict(cuerpo)
    distinta["amount"] = "301.00"
    distinta["allocations"] = [{"goalId": gid, "amount": "301.00"}]
    conflicto = ctx.pedir("E044", token=ctx.a.access, cuerpo=distinta, idempotencia=clave)
    if _exigir(ctx, "E044", conflicto, r, (409,)):
        r.ok("409 misma clave y otra carga")


def e045(ctx: n.Contexto) -> None:
    r = ctx.reg("E045")
    if not _sesion_a(ctx, r):
        return
    cid = ctx.datos.get("contribution_id")
    p = ctx.pedir("E045", token=ctx.a.access)
    if not _exigir(ctx, "E045", p, r, (200,)):
        return
    r.ok("aportación listada") if any(
        isinstance(i, dict) and i.get("id") == cid for i in _lista(p)
    ) else r.fallo("aportación listada", "no aparece", n.CONTENT_MISMATCH)


def e046(ctx: n.Contexto) -> None:
    r = ctx.reg("E046")
    if not _sesion_a(ctx, r) or not _sesion_b(ctx, r):
        return
    cid = ctx.datos.get("contribution_id")
    if not cid:
        r.bloqueo("aportación disponible", "E044 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E046", ruta=f"/api/v1/goal-contributions/{cid}", token=ctx.a.access)
    if not _exigir(ctx, "E046", p, r, (200,)):
        return
    r.ok("importe exacto") if n.comparar_decimal(p.campo("amount"), "300.00") else r.fallo(
        "importe exacto", str(p.campo("amount")), n.CONTENT_MISMATCH
    )
    ajeno = ctx.pedir("E046", ruta=f"/api/v1/goal-contributions/{cid}", token=ctx.b.access)
    if _exigir(ctx, "E046", ajeno, r, (404,)):
        r.ok("404 para otra cuenta")


def e047(ctx: n.Contexto) -> None:
    r = ctx.reg("E047")
    if not _mutaciones(ctx, r, "E047"):
        return
    if not _sesion_a(ctx, r):
        return
    cid = ctx.datos.get("contribution_id")
    gid = ctx.datos.get("goal_id")
    if not cid or not gid:
        r.bloqueo("aportación disponible", "E044 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E047", ruta=f"/api/v1/goal-contributions/{cid}/reverse", token=ctx.a.access, cuerpo={}, idempotencia=n.idempotencia())
    if not _exigir(ctx, "E047", p, r, (200,)):
        return
    r.ok("status reversed") if p.campo("status") == "reversed" else r.fallo(
        "status reversed", str(p.campo("status")), n.CONTENT_MISMATCH
    )
    meta = ctx.pedir("E040", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access)
    if _exigir(ctx, "E040", meta, r, (200,)):
        r.ok("savedAmount vuelve al inicial") if n.comparar_decimal(meta.campo("savedAmount"), "100.00") else r.fallo(
            "savedAmount vuelve al inicial", str(meta.campo("savedAmount")), n.CONTENT_MISMATCH
        )
    replay = ctx.pedir("E047", ruta=f"/api/v1/goal-contributions/{cid}/reverse", token=ctx.a.access, cuerpo={}, idempotencia=n.idempotencia())
    if _exigir(ctx, "E047", replay, r, (409,)):
        r.ok("reversión duplicada 409")


def e042(ctx: n.Contexto) -> None:
    r = ctx.reg("E042")
    if not _mutaciones(ctx, r, "E042"):
        return
    if not _sesion_a(ctx, r):
        return
    gid = ctx.datos.get("goal_id")
    if not gid:
        r.bloqueo("meta disponible", "E039 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E042", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access)
    if _exigir(ctx, "E042", p, r, (204,)):
        r.ok("204 sin cuerpo")
    lectura = ctx.pedir("E040", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access)
    if _exigir(ctx, "E040", lectura, r, (404,)):
        r.ok("lectura posterior 404")
    replay = ctx.pedir("E042", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access)
    if _exigir(ctx, "E042", replay, r, (204, 404, 409)):
        r.ok("replay idempotente")


# --------------------------------------------------------------------------- #
# Finanzas: resumen exacto (E026)
# --------------------------------------------------------------------------- #

def e026(ctx: n.Contexto) -> None:
    r = ctx.reg("E026")
    if not _sesion_a(ctx, r):
        return
    # E026 no muta por sí mismo: siembra con E028/E039/E044. La puerta se consulta
    # con el escenario sembrador real, no con E026 (que siempre permitiría).
    if not _mutaciones(ctx, r, "E028"):
        return
    ctx.advertencias.append(
        "E026 siembra movimientos sintéticos (ingreso, gasto y ahorro) que permanecen en la cuenta A: "
        "cada reejecución acumula datos y altera los agregados"
    )
    periodo = n.periodo_desplazado(-3)
    instante = n.instante_en_periodo(periodo, dia=15, hora=12)
    sembrados = (
        {"type": "income", "amount": "1000.00", "categoryId": "ingreso", "description": "Ingreso sintético E026"},
        {"type": "expense", "amount": "150.00", "categoryId": ctx.category_id(), "description": "Gasto sintético E026"},
        {"type": "saving", "amount": "200.00", "categoryId": "ahorro", "description": "Ahorro independiente E026"},
    )
    for base in sembrados:
        cuerpo = dict(base, currency="MXN", occurredAt=instante)
        p = ctx.pedir("E028", token=ctx.a.access, cuerpo=cuerpo, idempotencia=n.idempotencia())
        if not _exigir(ctx, "E028", p, r, (201,)):
            return
    meta = {
        "name": "Meta E026 sintética",
        "targetAmount": "2000.00",
        "currency": "MXN",
        "targetDate": n.fecha_desplazada(90),
    }
    p = ctx.pedir("E039", token=ctx.a.access, cuerpo=meta, idempotencia=n.idempotencia())
    if not _exigir(ctx, "E039", p, r, (201,)):
        return
    gid = _identificador(p, "id")
    if not gid:
        r.fallo("meta de siembra", "sin id", n.CONTENT_MISMATCH)
        return
    aportacion = {
        "amount": "300.00",
        "currency": "MXN",
        "allocations": [{"goalId": gid, "amount": "300.00"}],
        "occurredAt": instante,
    }
    p = ctx.pedir("E044", token=ctx.a.access, cuerpo=aportacion, idempotencia=n.idempotencia())
    if not _exigir(ctx, "E044", p, r, (201,)):
        return
    cid = _identificador(p, "id")
    ruta = f"/api/v1/financial-summary?period={periodo}"
    resumen = ctx.pedir("E026", ruta=ruta, token=ctx.a.access)
    if not _exigir(ctx, "E026", resumen, r, (200,)):
        return
    r.ok("periodo solicitado") if resumen.campo("period") == periodo else r.fallo(
        "periodo solicitado", str(resumen.campo("period")), n.CONTENT_MISMATCH
    )
    exactos = (
        ("income", "1000.00"),
        ("expenses", "150.00"),
        ("savings", "500.00"),
        ("net", "350.00"),
    )
    for campo, esperado in exactos:
        obtenido = resumen.campo(campo)
        r.ok(f"{campo} exacto") if n.comparar_decimal(obtenido, esperado) else r.fallo(
            f"{campo} exacto", f"esperado {esperado}, obtenido {obtenido}", n.CONTENT_MISMATCH
        )
    r.ok("currency MXN") if resumen.campo("currency") == "MXN" else r.fallo(
        "currency MXN", str(resumen.campo("currency")), n.CONTENT_MISMATCH
    )
    if _sesion_b(ctx, r):
        aislado = ctx.pedir("E026", ruta=ruta, token=ctx.b.access)
        if _exigir(ctx, "E026", aislado, r, (200,)):
            for campo in ("income", "expenses", "savings", "net"):
                r.ok(f"{campo} aislado en B") if n.comparar_decimal(aislado.campo(campo), "0.00") else r.fallo(
                    f"{campo} aislado en B", f"{campo}={aislado.campo(campo)}", n.CONTENT_MISMATCH
                )
    invalido = ctx.pedir("E026", ruta="/api/v1/financial-summary?period=2026-13", token=ctx.a.access)
    if _exigir(ctx, "E026", invalido, r, (400, 422)):
        r.ok("periodo inválido rechazado")
    # La meta y la aportación de siembra se archivan para no dejar residuos activos.
    if cid:
        ctx.pedir("E047", ruta=f"/api/v1/goal-contributions/{cid}/reverse", token=ctx.a.access, cuerpo={}, idempotencia=n.idempotencia())
    ctx.pedir("E042", ruta=f"/api/v1/goals/{gid}", token=ctx.a.access)


# --------------------------------------------------------------------------- #
# Finanzas: presupuestos y dashboard
# --------------------------------------------------------------------------- #

def e048(ctx: n.Contexto) -> None:
    r = ctx.reg("E048")
    if not _sesion_a(ctx, r):
        return
    periodo = n.periodo_desplazado(0)
    p = ctx.pedir("E048", ruta=f"/api/v1/budgets?period={periodo}", token=ctx.a.access)
    if not _exigir(ctx, "E048", p, r, (200,)):
        return
    r.ok("items[]") if isinstance(p.campo("items"), list) else r.fallo(
        "items[]", "ausente", n.CONTRACT_MISMATCH
    )
    ctx.datos["budgets_etag"] = _etag(p)


def e049(ctx: n.Contexto) -> None:
    r = ctx.reg("E049")
    if not _mutaciones(ctx, r, "E049"):
        return
    if not _sesion_a(ctx, r):
        return
    periodo = n.periodo_desplazado(0)
    categoria = ctx.datos.get("movement_category") or ctx.category_id()
    cuerpo = {
        "period": periodo,
        "currency": "MXN",
        "budgets": [{"categoryId": categoria, "limitAmount": "1000.00"}],
    }
    sin = ctx.pedir("E049", token=ctx.a.access, cuerpo=cuerpo)
    if _exigir(ctx, "E049", sin, r, (428,)):
        r.ok("428 sin If-Match")
    etag = ctx.datos.get("budgets_etag") or '"v1"'
    ok = ctx.pedir("E049", token=ctx.a.access, cuerpo=cuerpo, cabeceras={"If-Match": etag})
    if not _exigir(ctx, "E049", ok, r, (200,)):
        return
    r.ok("200 con If-Match")
    items = _lista(ok)
    presupuesto = next((i for i in items if isinstance(i, dict) and i.get("categoryId") == categoria), None)
    r.ok("presupuesto persistido") if presupuesto else r.fallo(
        "presupuesto persistido", "no aparece la categoría", n.CONTENT_MISMATCH
    )
    if presupuesto:
        r.ok("limitAmount exacto") if n.comparar_decimal(presupuesto.get("limitAmount"), "1000.00") else r.fallo(
            "limitAmount exacto", str(presupuesto.get("limitAmount")), n.CONTENT_MISMATCH
        )
    ctx.datos["budget_category"] = categoria
    ctx.datos["budgets_etag"] = _etag(ok) or etag


def e050(ctx: n.Contexto) -> None:
    r = ctx.reg("E050")
    if not _sesion_a(ctx, r):
        return
    periodo = n.periodo_desplazado(0)
    p = ctx.pedir("E050", ruta=f"/api/v1/budgets/usage?period={periodo}", token=ctx.a.access)
    if not _exigir(ctx, "E050", p, r, (200,)):
        return
    categoria = ctx.datos.get("budget_category")
    uso = next((i for i in _lista(p) if isinstance(i, dict) and i.get("categoryId") == categoria), None)
    if not uso:
        r.fallo("uso de la categoría presupuestada", "no aparece", n.CONTENT_MISMATCH)
        return
    r.ok("limitAmount exacto") if n.comparar_decimal(uso.get("limitAmount"), "1000.00") else r.fallo(
        "limitAmount exacto", str(uso.get("limitAmount")), n.CONTENT_MISMATCH
    )
    # El único gasto de esa categoría en el mes es el movimiento de E028 (150.00).
    r.ok("usedAmount exacto") if n.comparar_decimal(uso.get("usedAmount"), "150.00") else r.fallo(
        "usedAmount exacto", f"obtenido {uso.get('usedAmount')}", n.CONTENT_MISMATCH
    )


def e051(ctx: n.Contexto) -> None:
    r = ctx.reg("E051")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E051", token=ctx.a.access)
    if not _exigir(ctx, "E051", p, r, (200,)):
        return
    periodo = p.campo("period")
    resumen = p.campo("summary")
    r.ok("summary presente") if isinstance(resumen, dict) else r.fallo(
        "summary presente", "ausente", n.CONTENT_MISMATCH
    )
    if isinstance(resumen, dict) and isinstance(periodo, str):
        directo = ctx.pedir("E026", ruta=f"/api/v1/financial-summary?period={periodo}", token=ctx.a.access)
        if _exigir(ctx, "E026", directo, r, (200,)):
            for campo in ("period", "income", "expenses", "savings", "net"):
                r.ok(f"dashboard.{campo} == E026") if str(resumen.get(campo)) == str(directo.campo(campo)) else r.fallo(
                    f"dashboard.{campo} == E026",
                    f"{resumen.get(campo)} != {directo.campo(campo)}",
                    n.CONTENT_MISMATCH,
                )
    r.ok("goals[]") if isinstance(p.campo("goals"), list) else r.fallo(
        "goals[]", "ausente", n.CONTRACT_MISMATCH
    )
    r.ok("latestMovements[]") if isinstance(p.campo("latestMovements"), list) else r.fallo(
        "latestMovements[]", "ausente", n.CONTRACT_MISMATCH
    )
    cache = (p.cabeceras.get("cache-control") or "").lower()
    r.ok("sin caché (no-store)") if "no-store" in cache else r.fallo(
        "sin caché (no-store)", cache or "ausente", n.CONTENT_MISMATCH
    )


# --------------------------------------------------------------------------- #
# Analítica y ML
# --------------------------------------------------------------------------- #

def _sembrar_analitica(ctx: n.Contexto, sesion: n.Sesion, resultado: n.Resultado, periodo: str, categorias: list[str]) -> int:
    total = 0
    for indice in range(14):
        categoria = categorias[indice % len(categorias)]
        cuerpo = {
            "type": "expense",
            "amount": f"{10 + indice}.00",
            "currency": "MXN",
            "categoryId": categoria,
            "description": f"Gasto analítico {indice}",
            "occurredAt": n.instante_en_periodo(periodo, dia=indice + 1),
        }
        p = ctx.pedir("E028", token=sesion.access, cuerpo=cuerpo, idempotencia=n.idempotencia())
        if not _exigir(ctx, "E028", p, resultado, (201,)):
            return -1
        total += 10 + indice
    return total


def e052(ctx: n.Contexto) -> None:
    r = ctx.reg("E052")
    if not _sesion_a(ctx, r):
        return
    # E052 siembra con E028; la autorización se decide con el escenario que muta.
    if not _mutaciones(ctx, r, "E028"):
        return
    ctx.advertencias.append(
        "E052 siembra 14 gastos sintéticos en el periodo analizado: cada reejecución los acumula "
        "en la cuenta A y cambia los totales de E053/E054"
    )
    periodo = n.periodo_desplazado(-1)
    categorias = [ctx.category_id()]
    catalogo = ctx.pedir("E022")
    if catalogo.codigo == 200:
        gastos = [i.get("id") for i in _lista(catalogo) if isinstance(i, dict) and i.get("type") == "expense"]
        if len(gastos) >= 2:
            categorias = gastos[:2]
    if len(categorias) < 2:
        r.bloqueo("dos categorías de gasto", "el catálogo no las ofrece", n.BLOCKED_DEPENDENCY)
        return
    total = _sembrar_analitica(ctx, ctx.a, r, periodo, categorias)
    if total < 0:
        return
    ctx.datos.update({"analitica_periodo": periodo, "analitica_categorias": categorias, "analitica_total": total})
    p = ctx.pedir("E052", ruta=f"/api/v1/analytics/insights?period={periodo}", token=ctx.a.access)
    if not _exigir(ctx, "E052", p, r, (200,)):
        return
    r.ok("enoughData true") if p.campo("enoughData") is True else r.fallo(
        "enoughData true", str(p.campo("enoughData")), n.CONTENT_MISMATCH
    )
    r.ok("minimumSamples 12") if p.campo("minimumSamples") == 12 else r.fallo(
        "minimumSamples 12", str(p.campo("minimumSamples")), n.CONTENT_MISMATCH
    )
    items = _lista(p)
    r.ok("insights con evidencia") if items and all(
        isinstance(i, dict) and isinstance(i.get("evidenceCount"), int) and i["evidenceCount"] > 0 for i in items
    ) else r.fallo("insights con evidencia", "items vacío o sin evidenceCount", n.CONTENT_MISMATCH)
    invalido = ctx.pedir("E052", ruta="/api/v1/analytics/insights?period=2026-13", token=ctx.a.access)
    if _exigir(ctx, "E052", invalido, r, (400, 422)):
        r.ok("periodo inválido rechazado")


def e053(ctx: n.Contexto) -> None:
    r = ctx.reg("E053")
    if not _sesion_a(ctx, r):
        return
    periodo = ctx.datos.get("analitica_periodo")
    if not periodo:
        r.bloqueo("periodo sembrado", "E052 no sembró datos", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E053", ruta=f"/api/v1/analytics/categories?period={periodo}", token=ctx.a.access)
    if not _exigir(ctx, "E053", p, r, (200,)):
        return
    items = _lista(p)
    categorias = ctx.datos.get("analitica_categorias") or []
    r.ok("categorías analizadas") if len(items) == len(categorias) else r.fallo(
        "categorías analizadas", f"{len(items)} != {len(categorias)}", n.CONTENT_MISMATCH
    )
    suma = sum((n.decimal_exacto(i.get("amount")) or n.Decimal(0)) for i in items if isinstance(i, dict))
    esperado = n.Decimal(ctx.datos.get("analitica_total") or 0)
    r.ok("suma exacta de importes") if suma == esperado else r.fallo(
        "suma exacta de importes", f"{suma} != {esperado}", n.CONTENT_MISMATCH
    )
    porcentajes = sum(i.get("percentage", 0) for i in items if isinstance(i, dict))
    r.ok("porcentajes suman 100") if porcentajes == 100 else r.fallo(
        "porcentajes suman 100", str(porcentajes), n.CONTENT_MISMATCH
    )


def e054(ctx: n.Contexto) -> None:
    r = ctx.reg("E054")
    if not _sesion_a(ctx, r):
        return
    periodo = ctx.datos.get("analitica_periodo")
    if not periodo:
        r.bloqueo("periodo sembrado", "E052 no sembró datos", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E054", ruta=f"/api/v1/analytics/spending-patterns?period={periodo}", token=ctx.a.access)
    if not _exigir(ctx, "E054", p, r, (200,)):
        return
    items = _lista(p)
    r.ok("patrones calculados") if items else r.fallo(
        "patrones calculados", "items vacío", n.CONTENT_MISMATCH
    )
    r.ok("sampleSize coherente") if all(
        isinstance(i, dict) and i.get("sampleSize") == 7 for i in items
    ) else r.fallo("sampleSize coherente", "se esperaban 7 por categoría", n.CONTENT_MISMATCH)


def e055(ctx: n.Contexto) -> None:
    r = ctx.reg("E055")
    if not _sesion_a(ctx, r):
        return
    periodo = ctx.datos.get("analitica_periodo")
    if not periodo:
        r.bloqueo("periodo sembrado", "E052 no sembró datos", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E055", ruta=f"/api/v1/analytics/recommendations?period={periodo}", token=ctx.a.access)
    if not _exigir(ctx, "E055", p, r, (200,)):
        return
    r.ok("hasEnoughData booleano") if isinstance(p.campo("hasEnoughData"), bool) else r.fallo(
        "hasEnoughData booleano", "ausente", n.CONTRACT_MISMATCH
    )
    items = _lista(p)
    r.ok("hasEnoughData=true con historial") if p.campo("hasEnoughData") is True else r.fallo(
        "hasEnoughData=true con historial", str(p.campo("hasEnoughData")), n.CONTENT_MISMATCH
    )
    if items:
        r.ok("modelVersion en cada recomendación") if all(
            isinstance(i, dict) and isinstance(i.get("modelVersion"), str) and i["modelVersion"] for i in items
        ) else r.fallo("modelVersion en cada recomendación", "ausente o vacío", n.CONTENT_MISMATCH)
        r.ok("disclaimer en cada recomendación") if all(
            isinstance(i, dict) and isinstance(i.get("disclaimer"), str) and i["disclaimer"] for i in items
        ) else r.fallo("disclaimer en cada recomendación", "ausente", n.CONTENT_MISMATCH)
    else:
        # RFC-016 fija tres estados: hasEnoughData=false + items=[] (historial
        # insuficiente), true + [] (suficiente, sin patrones) y true + items.
        # El segundo es un resultado válido, no un bloqueo.
        r.ok(
            "estado sin patrones documentado",
            "hasEnoughData=true e items=[]: historial suficiente sin patrones que recomendar",
        )
    repetida = ctx.pedir("E055", ruta=f"/api/v1/analytics/recommendations?period={periodo}", token=ctx.a.access)
    if _exigir(ctx, "E055", repetida, r, (200,)):
        r.ok("determinismo en dos llamadas") if repetida.cuerpo == p.cuerpo else r.fallo(
            "determinismo en dos llamadas", "respuestas distintas", n.CONTENT_MISMATCH
        )
    # Ventana: B con historial solo fuera de los últimos 90 días no debe tener datos suficientes.
    if not _mutaciones(ctx, r, "E028") or not _sesion_b(ctx, r):
        return
    antiguo = n.periodo_desplazado(-8)
    categorias = ctx.datos.get("analitica_categorias") or [ctx.category_id()]
    total_b = _sembrar_analitica(ctx, ctx.b, r, antiguo, categorias)
    if total_b < 0:
        return
    fuera = ctx.pedir("E055", token=ctx.b.access)
    if _exigir(ctx, "E055", fuera, r, (200,)):
        r.ok("ventana de 90 días respetada") if fuera.campo("hasEnoughData") is False else r.fallo(
            "ventana de 90 días respetada", "B con datos antiguos tiene hasEnoughData=true", n.CONTENT_MISMATCH
        )
        r.ok("items vacío sin datos suficientes") if _lista(fuera) == [] else r.fallo(
            "items vacío sin datos suficientes", "items no vacío", n.CONTENT_MISMATCH
        )
    con_periodo = ctx.pedir("E055", ruta=f"/api/v1/analytics/recommendations?period={antiguo}", token=ctx.b.access)
    if _exigir(ctx, "E055", con_periodo, r, (200,)):
        r.ok("period explícito amplía la ventana") if con_periodo.campo("hasEnoughData") is True else r.fallo(
            "period explícito amplía la ventana", str(con_periodo.campo("hasEnoughData")), n.CONTENT_MISMATCH
        )


def e056(ctx: n.Contexto) -> None:
    r = ctx.reg("E056")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E056", token=ctx.a.access)
    if not _exigir(ctx, "E056", p, r, (200,)):
        return
    dias = p.campo("days")
    r.ok("days >= 0") if isinstance(dias, int) and dias >= 0 else r.fallo(
        "days >= 0", str(dias), n.CONTENT_MISMATCH
    )
    r.ok("timezone presente") if isinstance(p.campo("timezone"), str) and p.campo("timezone") else r.fallo(
        "timezone presente", "ausente", n.CONTENT_MISMATCH
    )


# --------------------------------------------------------------------------- #
# Notificaciones, recordatorios, FCM
# --------------------------------------------------------------------------- #

def e057(ctx: n.Contexto) -> None:
    r = ctx.reg("E057")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E057", token=ctx.a.access)
    if not _exigir(ctx, "E057", p, r, (200,)):
        return
    for campo in ("pushEnabled", "emailEnabled", "reminderEnabled"):
        r.ok(f"{campo} booleano") if isinstance(p.campo(campo), bool) else r.fallo(
            f"{campo} booleano", "ausente", n.CONTRACT_MISMATCH
        )


def e058(ctx: n.Contexto) -> None:
    r = ctx.reg("E058")
    if not _mutaciones(ctx, r, "E058"):
        return
    if not _sesion_a(ctx, r):
        return
    cuerpo = {
        "pushEnabled": True,
        "emailEnabled": False,
        "reminderEnabled": True,
        "quietHours": {"enabled": False, "start": "22:00", "end": "07:00"},
        "timeZone": "America/Mexico_City",
    }
    p = ctx.pedir("E058", token=ctx.a.access, cuerpo=cuerpo)
    if not _exigir(ctx, "E058", p, r, (200,)):
        return
    r.ok("pushEnabled persistido") if p.campo("pushEnabled") is True else r.fallo(
        "pushEnabled persistido", str(p.campo("pushEnabled")), n.CONTENT_MISMATCH
    )
    r.ok("quietHours objeto") if isinstance(p.campo("quietHours"), dict) else r.fallo(
        "quietHours objeto", "ausente", n.CONTENT_MISMATCH
    )
    r.ok("timeZone persistida") if p.campo("timeZone") == "America/Mexico_City" else r.fallo(
        "timeZone persistida", str(p.campo("timeZone")), n.CONTENT_MISMATCH
    )


def e059(ctx: n.Contexto) -> None:
    r = ctx.reg("E059")
    if not _mutaciones(ctx, r, "E059"):
        return
    if not _sesion_a(ctx, r):
        return
    if not ctx.opciones.fcm_token:
        r.bloqueo("token FCM aportado", "defina NEXUM_FCM_TOKEN; no se inventa un token", n.BLOCKED_EXTERNAL)
        return
    cuerpo = {"provider": "fcm", "token": ctx.opciones.fcm_token, "deviceLabel": "Dispositivo sintético"}
    p = ctx.pedir("E059", token=ctx.a.access, cuerpo=cuerpo, idempotencia=n.idempotencia())
    if not _exigir(ctx, "E059", p, r, (201,)):
        return
    device = _identificador(p, "id")
    if not device:
        r.fallo("id del dispositivo", "ausente", n.CONTENT_MISMATCH)
        return
    r.ok("dispositivo 201")
    r.ok("Location presente") if "location" in p.cabeceras else r.fallo(
        "Location presente", "ausente", n.CONTENT_MISMATCH
    )
    r.ok("respuesta sin token FCM") if "token" not in json.dumps(_cuerpo(p)).lower() else r.fallo(
        "respuesta sin token FCM", "aparece token", n.CONTENT_MISMATCH
    )
    ctx.datos["device_id"] = device


def e065(ctx: n.Contexto) -> None:
    r = ctx.reg("E065")
    if not _mutaciones(ctx, r, "E065"):
        return
    if not _sesion_a(ctx, r):
        return
    # Captura previa para identificar la notificación de la ocurrencia por ID nuevo.
    previas = ctx.pedir("E061", ruta="/api/v1/notifications?limit=100", token=ctx.a.access)
    if previas.codigo == 200:
        ctx.datos["notificaciones_previas"] = {
            i.get("id") for i in _lista(previas) if isinstance(i, dict) and i.get("id")
        }
    cuerpo = {
        "title": "Recordatorio sintético de certificación",
        "dueAt": n.iso_utc(n.ahora_utc() + timedelta(seconds=45)),
        "timezone": "America/Mexico_City",
    }
    clave = n.idempotencia()
    p = ctx.pedir("E065", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if not _exigir(ctx, "E065", p, r, (201,)):
        return
    rid = _identificador(p, "id")
    if not rid:
        r.fallo("id del recordatorio", "ausente", n.CONTENT_MISMATCH)
        return
    r.ok("recordatorio 201")
    r.ok("Location presente") if "location" in p.cabeceras else r.fallo(
        "Location presente", "ausente", n.CONTENT_MISMATCH
    )
    ctx.datos.update({"reminder_id": rid, "reminder_etag": _etag(p), "reminder_due": cuerpo["dueAt"]})
    replay = ctx.pedir("E065", token=ctx.a.access, cuerpo=cuerpo, idempotencia=clave)
    if _exigir(ctx, "E065", replay, r, (201,)):
        r.ok("replay estable y mismo id") if _identificador(replay, "id") == rid else r.fallo(
            "replay estable y mismo id", "id distinto", n.CONTENT_MISMATCH
        )


def e064(ctx: n.Contexto) -> None:
    r = ctx.reg("E064")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E064", ruta="/api/v1/reminders?limit=100", token=ctx.a.access)
    if not _exigir(ctx, "E064", p, r, (200,)):
        return
    rid = ctx.datos.get("reminder_id")
    r.ok("recordatorio listado") if any(
        isinstance(i, dict) and i.get("id") == rid for i in _lista(p)
    ) else r.fallo("recordatorio listado", "no aparece", n.CONTENT_MISMATCH)
    siguiente = p.campo("nextCursor")
    if isinstance(siguiente, str) and siguiente:
        continuacion = ctx.pedir("E064", ruta=f"/api/v1/reminders?limit=1&cursor={siguiente}", token=ctx.a.access)
        if _exigir(ctx, "E064", continuacion, r, (200,)):
            r.ok("nextCursor es continuación válida")
    filtrado = ctx.pedir("E064", ruta="/api/v1/reminders?active=true", token=ctx.a.access)
    if _exigir(ctx, "E064", filtrado, r, (200,)):
        r.ok("filtro active") if all(
            isinstance(i, dict) and i.get("active") is True for i in _lista(filtrado)
        ) else r.fallo("filtro active", "hay inactivos", n.CONTENT_MISMATCH)


def _esperar_ocurrencia(ctx: n.Contexto, resultado: n.Resultado) -> dict[str, Any] | None:
    """Polling acotado de E061. No muta y no reenvía correo ni push.

    Devuelve la notificación nueva completa: la correlación posterior usa su ID
    (nuevo respecto de la captura previa) y su ``createdAt`` frente al ``dueAt``
    del recordatorio, nunca el título del aviso.
    """
    previas = ctx.datos.get("notificaciones_previas")
    if previas is None:
        previas = set()
    limite = max(0, int(ctx.opciones.espera_ocurrencia_s))
    intervalo = max(1, int(ctx.opciones.intervalo_poll_s))
    inicio = time.monotonic()
    while time.monotonic() - inicio <= limite:
        p = ctx.pedir("E061", ruta="/api/v1/notifications?limit=100", token=ctx.a.access)
        if p.codigo == 200:
            for item in _lista(p):
                if isinstance(item, dict) and isinstance(item.get("id"), str) and item["id"] not in previas:
                    return item
        time.sleep(intervalo)
    return None


def e061(ctx: n.Contexto) -> None:
    r = ctx.reg("E061")
    if not _sesion_a(ctx, r):
        return
    rid = ctx.datos.get("reminder_id")
    if not rid:
        r.bloqueo("recordatorio próximo", "E065 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    if ctx.datos.get("notificaciones_previas") is None:
        r.bloqueo(
            "captura previa de la bandeja",
            "E065 no pudo capturarla; no se puede distinguir la ocurrencia de avisos previos",
            n.BLOCKED_DEPENDENCY,
        )
        return
    ocurrencia = _esperar_ocurrencia(ctx, r)
    if not ocurrencia:
        r.bloqueo(
            "ocurrencia observada",
            "sin notificación nueva en la ventana; revise scheduler/FCM (no es un fallo de contrato)",
            n.BLOCKED_DEPENDENCY,
        )
        return
    notificacion = str(ocurrencia.get("id") or "")
    ctx.datos["notification_id"] = notificacion
    r.ok("id nuevo respecto de la captura previa")
    debido = _instante(ctx.datos.get("reminder_due"))
    if debido is None:
        r.bloqueo(
            "evento del recordatorio disponible",
            "E065 no aportó dueAt en esta corrida (¿reanudado?); no se correlaciona por título",
            n.BLOCKED_DEPENDENCY,
        )
        return
    creado = _instante(ocurrencia.get("createdAt"))
    correlaciona = creado is not None and creado >= debido - timedelta(seconds=5)
    r.ok("ocurrencia correlacionada con el evento", "createdAt >= dueAt") if correlaciona else r.fallo(
        "ocurrencia correlacionada con el evento", "createdAt ausente o anterior a dueAt", n.CONTENT_MISMATCH
    )
    p = ctx.pedir("E061", ruta="/api/v1/notifications?limit=100", token=ctx.a.access)
    if not _exigir(ctx, "E061", p, r, (200,)):
        return
    r.ok("bandeja 200")
    r.ok("unreadCount entero") if isinstance(p.campo("unreadCount"), int) and p.campo("unreadCount") >= 1 else r.fallo(
        "unreadCount entero", str(p.campo("unreadCount")), n.CONTENT_MISMATCH
    )
    r.ok("notificación de la ocurrencia presente") if any(
        isinstance(i, dict) and i.get("id") == notificacion for i in _lista(p)
    ) else r.fallo("notificación de la ocurrencia presente", "no aparece", n.CONTENT_MISMATCH)
    if _sesion_b(ctx, r):
        ajeno = ctx.pedir("E061", ruta="/api/v1/notifications?limit=100", token=ctx.b.access)
        if _exigir(ctx, "E061", ajeno, r, (200,)):
            r.ok("aislamiento de B") if not any(
                isinstance(i, dict) and i.get("id") == notificacion for i in _lista(ajeno)
            ) else r.fallo("aislamiento de B", "B ve la notificación de A", n.CONTENT_MISMATCH)


def e062(ctx: n.Contexto) -> None:
    r = ctx.reg("E062")
    if not _mutaciones(ctx, r, "E062"):
        return
    if not _sesion_a(ctx, r):
        return
    notificacion = ctx.datos.get("notification_id")
    if not notificacion:
        r.bloqueo("notificación de la ocurrencia", "no se identificó un ID exacto", n.BLOCKED_DEPENDENCY)
        return
    antes = ctx.pedir("E061", ruta="/api/v1/notifications?limit=100", token=ctx.a.access)
    unread_antes = antes.campo("unreadCount") if antes.codigo == 200 else None
    p = ctx.pedir("E062", ruta=f"/api/v1/notifications/{notificacion}/read", token=ctx.a.access, cuerpo={})
    if not _exigir(ctx, "E062", p, r, (200,)):
        return
    r.ok("isRead true") if p.campo("isRead") is True else r.fallo(
        "isRead true", str(p.campo("isRead")), n.CONTENT_MISMATCH
    )
    r.ok("mismo id") if p.campo("id") == notificacion else r.fallo(
        "mismo id", str(p.campo("id")), n.CONTENT_MISMATCH
    )
    despues = ctx.pedir("E061", ruta="/api/v1/notifications?limit=100", token=ctx.a.access)
    if _exigir(ctx, "E061", despues, r, (200,)) and isinstance(unread_antes, int):
        r.ok("unreadCount decrementa") if despues.campo("unreadCount") == unread_antes - 1 else r.fallo(
            "unreadCount decrementa", f"{unread_antes}->{despues.campo('unreadCount')}", n.CONTENT_MISMATCH
        )
    repetida = ctx.pedir("E062", ruta=f"/api/v1/notifications/{notificacion}/read", token=ctx.a.access, cuerpo={})
    if _exigir(ctx, "E062", repetida, r, (200,)):
        r.ok("marcar leída es idempotente")
    if _sesion_b(ctx, r):
        ajeno = ctx.pedir("E062", ruta=f"/api/v1/notifications/{notificacion}/read", token=ctx.b.access, cuerpo={})
        if _exigir(ctx, "E062", ajeno, r, (404,)):
            r.ok("ID de A no afectable por B")
    invalido = ctx.pedir("E062", ruta="/api/v1/notifications/no-uuid/read", token=ctx.a.access, cuerpo={})
    if _exigir(ctx, "E062", invalido, r, (400,)):
        r.ok("UUID inválido rechazado")


def e063(ctx: n.Contexto) -> None:
    r = ctx.reg("E063")
    if not _mutaciones(ctx, r, "E063"):
        return
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E063", token=ctx.a.access, cuerpo={})
    if not _exigir(ctx, "E063", p, r, (202,)):
        return
    r.ok("202 de aceptación")
    r.ok("status accepted") if p.campo("status") == "accepted" else r.fallo(
        "status accepted", str(p.campo("status")), n.CONTENT_MISMATCH
    )
    # 202 no promete ejecución inmediata: se comprueba con polling acotado.
    limite = max(1, int(ctx.opciones.espera_ocurrencia_s))
    intervalo = max(1, int(ctx.opciones.intervalo_poll_s))
    inicio = time.monotonic()
    while time.monotonic() - inicio <= limite:
        lista = ctx.pedir("E061", ruta="/api/v1/notifications?limit=100", token=ctx.a.access)
        if lista.codigo == 200 and lista.campo("unreadCount") == 0:
            r.ok("todas marcadas como leídas")
            return
        time.sleep(intervalo)
    r.bloqueo(
        "todas marcadas como leídas",
        "202 aceptado sin confirmación en la ventana; procesamiento asíncrono no observado",
        n.BLOCKED_DEPENDENCY,
    )


def e066(ctx: n.Contexto) -> None:
    r = ctx.reg("E066")
    if not _mutaciones(ctx, r, "E066"):
        return
    if not _sesion_a(ctx, r):
        return
    rid = ctx.datos.get("reminder_id")
    etag = ctx.datos.get("reminder_etag") or '"v1"'
    if not rid:
        r.bloqueo("recordatorio disponible", "E065 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    sin = ctx.pedir("E066", ruta=f"/api/v1/reminders/{rid}", token=ctx.a.access, cuerpo={"title": "Recordatorio 2"})
    if _exigir(ctx, "E066", sin, r, (428,)):
        r.ok("428 sin If-Match")
    ok = ctx.pedir(
        "E066", ruta=f"/api/v1/reminders/{rid}", token=ctx.a.access, cuerpo={"title": "Recordatorio 2"}, cabeceras={"If-Match": etag}
    )
    if not _exigir(ctx, "E066", ok, r, (200,)):
        return
    r.ok("título persistido") if ok.campo("title") == "Recordatorio 2" else r.fallo(
        "título persistido", str(ok.campo("title")), n.CONTENT_MISMATCH
    )
    nuevo_etag = _etag(ok) or etag
    nuevo_due = n.iso_utc(n.ahora_utc() + timedelta(minutes=30))
    movido = ctx.pedir(
        "E066", ruta=f"/api/v1/reminders/{rid}", token=ctx.a.access, cuerpo={"dueAt": nuevo_due}, cabeceras={"If-Match": nuevo_etag}
    )
    if _exigir(ctx, "E066", movido, r, (200,)):
        r.ok("dueAt movido") if movido.campo("dueAt") != ctx.datos.get("reminder_due") else r.fallo(
            "dueAt movido", "sin cambio", n.CONTENT_MISMATCH
        )
        ctx.datos["reminder_etag"] = _etag(movido) or nuevo_etag
    obsoleto = ctx.pedir(
        "E066", ruta=f"/api/v1/reminders/{rid}", token=ctx.a.access, cuerpo={"title": "Recordatorio 3"}, cabeceras={"If-Match": etiqueta_vieja(ok, etag)}
    )
    if _exigir(ctx, "E066", obsoleto, r, (412,)):
        r.ok("412 con versión obsoleta")


def etiqueta_vieja(respuesta: n.Respuesta, por_defecto: str) -> str:
    """ETag de la versión anterior, para forzar 412."""
    version = respuesta.campo("version")
    if isinstance(version, int) and version > 1:
        return f'"v{version - 1}"'
    return por_defecto


def e067(ctx: n.Contexto) -> None:
    r = ctx.reg("E067")
    if not _mutaciones(ctx, r, "E067"):
        return
    if not _sesion_a(ctx, r):
        return
    rid = ctx.datos.get("reminder_id")
    if not rid:
        r.bloqueo("recordatorio disponible", "E065 no aportó id", n.BLOCKED_DEPENDENCY)
        return
    p = ctx.pedir("E067", ruta=f"/api/v1/reminders/{rid}", token=ctx.a.access)
    if _exigir(ctx, "E067", p, r, (204,)):
        r.ok("204 sin cuerpo")
    lectura = ctx.pedir("E066", ruta=f"/api/v1/reminders/{rid}", token=ctx.a.access, cuerpo={}, cabeceras={"If-Match": '"v1"'})
    if _exigir(ctx, "E066", lectura, r, (404,)):
        r.ok("lectura posterior 404")
    replay = ctx.pedir("E067", ruta=f"/api/v1/reminders/{rid}", token=ctx.a.access)
    if _exigir(ctx, "E067", replay, r, (204, 404)):
        r.ok("replay idempotente")
    if _sesion_b(ctx, r):
        ajeno = ctx.pedir("E067", ruta=f"/api/v1/reminders/{rid}", token=ctx.b.access)
        if _exigir(ctx, "E067", ajeno, r, (404,)):
            r.ok("recordatorio de A invisible para B")


def e060(ctx: n.Contexto) -> None:
    r = ctx.reg("E060")
    if not _mutaciones(ctx, r, "E060"):
        return
    if not _sesion_a(ctx, r):
        return
    device = ctx.datos.get("device_id")
    if not device:
        if not ctx.opciones.fcm_token:
            r.bloqueo("dispositivo registrado", "defina NEXUM_FCM_TOKEN para registrar y revocar", n.BLOCKED_EXTERNAL)
            return
        registro = ctx.pedir(
            "E059",
            token=ctx.a.access,
            cuerpo={"provider": "fcm", "token": ctx.opciones.fcm_token, "deviceLabel": "Dispositivo E060"},
            idempotencia=n.idempotencia(),
        )
        if not _exigir(ctx, "E059", registro, r, (201,)):
            return
        device = _identificador(registro, "id")
        if not device:
            r.fallo("id del dispositivo", "ausente", n.CONTENT_MISMATCH)
            return
        ctx.datos["device_id"] = device
    p = ctx.pedir("E060", ruta=f"/api/v1/devices/push-tokens/{device}", token=ctx.a.access)
    if _exigir(ctx, "E060", p, r, (204,)):
        r.ok("revocación 204")
        r.ok("cuerpo vacío") if not (p.texto or "").strip() else r.fallo(
            "cuerpo vacío", f"len={len(p.texto)}", n.CONTENT_MISMATCH
        )
    replay = ctx.pedir("E060", ruta=f"/api/v1/devices/push-tokens/{device}", token=ctx.a.access)
    # El contrato declara 204 (revocación idempotente), 404 y 409 (ya revocado).
    # Exigir sólo 404/409 convertiría en fallo una respuesta 204 válida.
    if _exigir(ctx, "E060", replay, r, (204, 404, 409)):
        r.ok("segunda revocación idempotente")
    if _sesion_b(ctx, r):
        ajeno = ctx.pedir("E060", ruta=f"/api/v1/devices/push-tokens/{device}", token=ctx.b.access)
        if _exigir(ctx, "E060", ajeno, r, (404,)):
            r.ok("dispositivo de A invisible para B")
    # La no-recepción de push no es observable por la API: exige un receptor FCM externo.
    if ctx.opciones.fcm_receipt_file:
        try:
            with open(ctx.opciones.fcm_receipt_file, encoding="utf-8") as fh:
                recibos = [json.loads(linea) for linea in fh if linea.strip()]
        except (OSError, json.JSONDecodeError):
            r.bloqueo("receptor FCM externo", "no se pudo leer el archivo de recibos", n.BLOCKED_EXTERNAL)
            return
        posteriores = [
            rec for rec in recibos
            if isinstance(rec, dict) and rec.get("deviceId") == device and rec.get("afterRevoke") is True
        ]
        r.ok("sin entrega tras revocar") if not posteriores else r.fallo(
            "sin entrega tras revocar", f"{len(posteriores)} recibos posteriores", n.CONTENT_MISMATCH
        )
    else:
        r.bloqueo(
            "sin entrega push tras revocar",
            "no observable por HTTP; aporte NEXUM_FCM_RECEIPT_FILE o verifique en el dispositivo",
            n.BLOCKED_EXTERNAL,
        )


# --------------------------------------------------------------------------- #
# Exportación, bootstrap, demo y voz
# --------------------------------------------------------------------------- #

def e020(ctx: n.Contexto) -> None:
    r = ctx.reg("E020")
    if not _sesion_a(ctx, r):
        return
    agregado: list[dict[str, Any]] = []
    perfil: dict[str, Any] = {}
    familias: set[str] = set()
    textos: list[str] = []
    cursor = ""
    vistos: set[str] = set()
    for _ in range(50):
        ruta = "/api/v1/users/me/export?limit=100" + (f"&cursor={cursor}" if cursor else "")
        p = ctx.pedir("E020", ruta=ruta, token=ctx.a.access)
        if not _exigir(ctx, "E020", p, r, (200,)):
            return
        contenido = _cuerpo(p)
        textos.append(json.dumps(contenido, ensure_ascii=False))
        if not perfil and isinstance(contenido.get("profile"), dict):
            perfil = contenido["profile"]
        for familia, valor in contenido.items():
            if isinstance(valor, list):
                familias.add(familia)
                for elemento in valor:
                    if isinstance(elemento, dict) and isinstance(elemento.get("id"), str):
                        if elemento["id"] in vistos:
                            r.fallo("sin duplicados entre páginas", elemento["id"][:8] + "…", n.CONTENT_MISMATCH)
                        vistos.add(elemento["id"])
                        agregado.append(elemento)
        siguiente = p.campo("nextCursor")
        if not isinstance(siguiente, str) or not siguiente:
            break
        if siguiente == cursor:
            r.fallo("cursor no repetido", "nextCursor igual al anterior", n.CONTRACT_MISMATCH)
            return
        cursor = siguiente
    r.ok("profile presente") if perfil else r.fallo(
        "profile presente", "ausente en todas las páginas", n.CONTENT_MISMATCH
    )
    r.ok("profile.id coincide") if perfil.get("id") == ctx.a.user_id else r.fallo(
        "profile.id coincide", "distinto", n.CONTENT_MISMATCH
    )
    correo = perfil.get("email")
    r.ok("profile.email coincide") if isinstance(correo, str) and correo.lower() == ctx.a.email.lower() else r.fallo(
        "profile.email coincide", "distinto", n.CONTENT_MISMATCH
    )
    for familia in ("movements", "goals", "consents"):
        r.ok(f"{familia}[]") if familia in familias else r.fallo(
            f"{familia}[]", "ausente en todas las páginas", n.CONTRACT_MISMATCH
        )
    texto = " ".join(textos).lower()
    for prohibido in ("accesstoken", "refreshtoken", "passwordhash", "tokencipher", "tokenfingerprint", "leaseuntil", "refreshhash"):
        r.ok(f"sin {prohibido}") if prohibido not in texto else r.fallo(
            f"sin {prohibido}", "aparece en la exportación", n.CONTENT_MISMATCH
        )
    if ctx.b.email:
        r.ok("sin datos de B") if ctx.b.email.lower() not in texto else r.fallo(
            "sin datos de B", "aparece el correo de B", n.CONTENT_MISMATCH
        )
    r.ok("exportación agregada", f"{len(agregado)} elementos") if agregado or vistos else r.ok(
        "exportación vacía es válida", "cuenta sin datos"
    )


def e021(ctx: n.Contexto) -> None:
    r = ctx.reg("E021")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E021", token=ctx.a.access)
    if not _exigir(ctx, "E021", p, r, (200,)):
        return
    perfil = p.campo("profile")
    r.ok("profile presente") if isinstance(perfil, dict) else r.fallo(
        "profile presente", "ausente", n.CONTENT_MISMATCH
    )
    if isinstance(perfil, dict):
        r.ok("profile.id coincide") if perfil.get("id") == ctx.a.user_id else r.fallo(
            "profile.id coincide", "distinto", n.CONTENT_MISMATCH
        )
        correo = perfil.get("email")
        r.ok("profile.email coincide") if isinstance(correo, str) and correo.lower() == ctx.a.email.lower() else r.fallo(
            "profile.email coincide", "distinto", n.CONTENT_MISMATCH
        )
    r.ok("financialSummary presente") if isinstance(p.campo("financialSummary"), dict) else r.fallo(
        "financialSummary presente", "ausente", n.CONTENT_MISMATCH
    )
    r.ok("catalogVersion presente") if isinstance(p.campo("catalogVersion"), str) and p.campo("catalogVersion") else r.fallo(
        "catalogVersion presente", "ausente", n.CONTENT_MISMATCH
    )


def e074(ctx: n.Contexto) -> None:
    r = ctx.reg("E074")
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E074", token=ctx.a.access)
    if not _exigir(ctx, "E074", p, r, (200,)):
        return
    r.ok("demo apagado por defecto") if p.campo("enabled") is False else r.fallo(
        "demo apagado por defecto", f"enabled={p.campo('enabled')}", n.CONTENT_MISMATCH
    )
    r.ok("scope sandbox") if p.campo("scope") == "sandbox" else r.fallo(
        "scope sandbox", str(p.campo("scope")), n.CONTENT_MISMATCH
    )


def e075(ctx: n.Contexto) -> None:
    r = ctx.reg("E075")
    if not _sesion_a(ctx, r):
        return
    antes = ctx.pedir("E027", ruta="/api/v1/movements?limit=5", token=ctx.a.access)
    ids_antes = [i.get("id") for i in _lista(antes) if isinstance(i, dict)]
    p = ctx.pedir("E075", token=ctx.a.access, cuerpo={"text": "gasté 150 pesos en café", "locale": "es-MX"})
    if not _exigir(ctx, "E075", p, r, (200,)):
        return
    r.ok("intent createMovement") if p.campo("intent") == "createMovement" else r.fallo(
        "intent createMovement", str(p.campo("intent")), n.CONTENT_MISMATCH
    )
    r.ok("importe exacto 150.00") if n.comparar_decimal(p.campo("parsedAmount"), "150.00") else r.fallo(
        "importe exacto 150.00", str(p.campo("parsedAmount")), n.CONTENT_MISMATCH
    )
    r.ok("requiresConfirmation true") if p.campo("requiresConfirmation") is True else r.fallo(
        "requiresConfirmation true", str(p.campo("requiresConfirmation")), n.CONTENT_MISMATCH
    )
    ambiguo = ctx.pedir("E075", token=ctx.a.access, cuerpo={"text": "mil doscientos treinta y cuatro pesos", "locale": "es-MX"})
    if _exigir(ctx, "E075", ambiguo, r, (200,)):
        r.ok("ambigüedad explícita") if ambiguo.campo("intent") in ("createMovement", "unknown") else r.fallo(
            "ambigüedad explícita", str(ambiguo.campo("intent")), n.CONTENT_MISMATCH
        )
        r.ok("ambigüedad requiere confirmación") if ambiguo.campo("requiresConfirmation") is True else r.fallo(
            "ambigüedad requiere confirmación", str(ambiguo.campo("requiresConfirmation")), n.CONTENT_MISMATCH
        )
    despues = ctx.pedir("E027", ruta="/api/v1/movements?limit=5", token=ctx.a.access)
    ids_despues = [i.get("id") for i in _lista(despues) if isinstance(i, dict)] if despues.codigo == 200 else ids_antes
    r.ok("sin persistencia de movimiento") if ids_antes == ids_despues else r.fallo(
        "sin persistencia de movimiento", "la lista cambió", n.CONTENT_MISMATCH
    )


# --------------------------------------------------------------------------- #
# Correo y eliminación
# --------------------------------------------------------------------------- #

def e005(ctx: n.Contexto) -> None:
    r = ctx.reg("E005")
    if not _mutaciones(ctx, r, "E005"):
        return
    p = ctx.pedir("E005", cuerpo={"email": ctx.a.email})
    if _exigir(ctx, "E005", p, r, (202,)):
        r.ok("202 accepted")
        r.ok("requestId presente") if p.campo("requestId") else r.fallo(
            "requestId presente", "ausente", n.CONTENT_MISMATCH
        )
        r.ok("sin token en la respuesta") if "token" not in json.dumps(_cuerpo(p)).lower() else r.fallo(
            "sin token en la respuesta", "aparece token", n.CONTENT_MISMATCH
        )
        ctx.advertencias.append(
            "E005 envió un correo: no se reenvía automáticamente; para E006 use NEXUM_RESET_TOKEN con el token del buzón"
        )


def e006(ctx: n.Contexto) -> None:
    r = ctx.reg("E006")
    if not _mutaciones(ctx, r, "E006"):
        return
    token = ctx.opciones.reset_token
    if not token:
        r.bloqueo(
            "token de recuperación aportado",
            "defina NEXUM_RESET_TOKEN o ejecute con terminal interactiva; no se reenvía E005",
            n.BLOCKED_EXTERNAL,
        )
        return
    nueva = "Nx7!" + n.idempotencia()[:20]
    ctx.secretos.registrar("contraseña", nueva)
    anterior = ctx.a.password
    p = ctx.pedir("E006", cuerpo={"token": token, "newPassword": nueva})
    if not _exigir(ctx, "E006", p, r, (200,)):
        return
    r.ok("restablecimiento 200")
    ctx.a.password = nueva
    replay = ctx.pedir("E006", cuerpo={"token": token, "newPassword": nueva})
    if _exigir(ctx, "E006", replay, r, (409,)):
        r.ok("replay del token 409")
    viejo = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": anterior})
    if _exigir(ctx, "E002", viejo, r, (401,)):
        r.ok("contraseña anterior 401")
    nuevo = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": nueva})
    if _exigir(ctx, "E002", nuevo, r, (200,)):
        r.ok("login con la contraseña nueva 200")
        ctx.cargar_sesion(ctx.a, nuevo)
    if ctx.a.password_origen == "generado":
        ctx.advertencias.append(
            "E006 cambió la contraseña generada en memoria: exporte NEXUM_PASSWORD_A para reanudar otra fase"
        )


def e014(ctx: n.Contexto) -> None:
    r = ctx.reg("E014")
    if not ctx.autorizacion.permitir_eliminar_cuenta:
        r.bloqueo(
            "eliminación autorizada",
            "requiere --allow-delete-account (destruye la cuenta A)",
            n.BLOCKED_EXTERNAL,
        )
        return
    if not _mutaciones(ctx, r, "E014"):
        return
    if not _sesion_a(ctx, r):
        return
    p = ctx.pedir("E014", token=ctx.a.access)
    if not _exigir(ctx, "E014", p, r, (202,)):
        return
    solicitud = _identificador(p, "requestId", "deletionRequestId")
    r.ok("202 con requestId") if solicitud else r.fallo(
        "202 con requestId", "ausente", n.CONTENT_MISMATCH
    )
    r.ok("sessionsRevoked=true") if p.campo("sessionsRevoked") is True else r.fallo(
        "sessionsRevoked=true", str(p.campo("sessionsRevoked")), n.CONTENT_MISMATCH
    )
    login = ctx.pedir("E002", cuerpo={"email": ctx.a.email, "password": ctx.a.password})
    if _exigir(ctx, "E002", login, r, (401,)):
        r.ok("login posterior 401")
    acceso = ctx.pedir("E012", token=ctx.a.access)
    if _exigir(ctx, "E012", acceso, r, (401,)):
        r.ok("access posterior 401")
    ruta = os.environ.get("NEXUM_E014_PURGED_EVIDENCE", "").strip()
    if not ruta:
        r.bloqueo(
            "PURGED verificado",
            "no observable por HTTP; aporte NEXUM_E014_PURGED_EVIDENCE con requestId, state=PURGED y verificación",
            n.BLOCKED_EXTERNAL,
        )
        return
    try:
        with open(ruta, encoding="utf-8") as fh:
            prueba = json.load(fh)
    except (OSError, json.JSONDecodeError):
        r.bloqueo("PURGED verificado", "evidencia ilegible", n.BLOCKED_EXTERNAL)
        return
    coincide = (
        isinstance(prueba, dict)
        and prueba.get("requestId") == solicitud
        and prueba.get("state") == "PURGED"
        and prueba.get("verificadoPorHumano") is True
    )
    r.ok("PURGED verificado") if coincide else r.fallo(
        "PURGED verificado", "la evidencia no acredita PURGED para este requestId", n.CONTENT_MISMATCH
    )


# --------------------------------------------------------------------------- #
# Registro y orden
# --------------------------------------------------------------------------- #

def _esc(
    eid: str,
    titulo: str,
    metodo: str,
    ruta: str,
    fases: tuple[str, ...],
    funcion: Callable[[n.Contexto], None],
    *,
    muta: bool = False,
    destructivo: bool = False,
    depende_de: tuple[str, ...] = (),
    pasos: tuple[str, ...] = (),
    idempotencia: bool = False,
) -> Escenario:
    return Escenario(
        eid=eid,
        titulo=titulo,
        metodo=metodo,
        ruta=ruta,
        fases=fases,
        funcion=funcion,
        muta=muta,
        destructivo=destructivo,
        depende_de=depende_de,
        pasos=pasos,
        idempotencia=idempotencia,
    )


_ESCENARIOS_LISTA: tuple[Escenario, ...] = (
    _esc("E015", "Aviso de privacidad vigente", "GET", "/api/v1/privacy/notices/current", (FASE_PUBLICO,), e015),
    _esc("E022", "Catálogo de categorías", "GET", "/api/v1/catalogs/categories", (FASE_PUBLICO,), e022),
    _esc("E023", "Catálogo de frecuencias", "GET", "/api/v1/catalogs/frequencies", (FASE_PUBLICO,), e023),
    _esc("E070", "FAQ", "GET", "/api/v1/help/faq", (FASE_PUBLICO,), e070),
    _esc("E071", "Tips educativos", "GET", "/api/v1/education/tips", (FASE_PUBLICO,), e071),
    _esc("E072", "Health liveness", "GET", "/api/v1/health/live", (FASE_PUBLICO,), e072),
    _esc("E073", "Health readiness", "GET", "/api/v1/health/ready", (FASE_PUBLICO,), e073),
    _esc("E001", "Registro", "POST", "/api/v1/auth/register", (FASE_AUTH,), e001, muta=True, idempotencia=True),
    _esc("E002", "Login", "POST", "/api/v1/auth/login", (FASE_AUTH,), e002, muta=True, depende_de=("E001",)),
    _esc("E003", "Rotación de refresh", "POST", "/api/v1/auth/refresh", (FASE_AUTH,), e003, muta=True, depende_de=("E002",)),
    _esc("E004", "Logout", "POST", "/api/v1/auth/logout", (FASE_AUTH,), e004, muta=True, depende_de=("E002",)),
    _esc("E007", "Cambio de contraseña", "POST", "/api/v1/auth/password/change", (FASE_AUTH,), e007, muta=True, depende_de=("E002",)),
    _esc("E008", "Listado de sesiones", "GET", "/api/v1/auth/sessions", (FASE_AUTH,), e008, depende_de=("E002",)),
    _esc("E009", "Revocación de sesión", "DELETE", "/api/v1/auth/sessions/{sessionId}", (FASE_AUTH,), e009, muta=True, depende_de=("E008",)),
    _esc("E012", "Perfil", "GET", "/api/v1/users/me", (FASE_PERFIL,), e012, depende_de=("E002",)),
    _esc("E013", "Actualización de perfil", "PATCH", "/api/v1/users/me", (FASE_PERFIL,), e013, muta=True, depende_de=("E012",)),
    _esc("E016", "Consentimientos", "GET", "/api/v1/users/me/consents", (FASE_PERFIL,), e016, depende_de=("E002",)),
    _esc("E017", "Registro de consentimiento", "POST", "/api/v1/users/me/consents", (FASE_PERFIL,), e017, muta=True, depende_de=("E016",)),
    _esc("E018", "Onboarding", "GET", "/api/v1/users/me/onboarding", (FASE_PERFIL,), e018, depende_de=("E002",)),
    _esc("E019", "Actualización de onboarding", "PATCH", "/api/v1/users/me/onboarding", (FASE_PERFIL,), e019, muta=True, depende_de=("E018",)),
    _esc("E068", "Preferencias", "GET", "/api/v1/users/me/preferences", (FASE_PERFIL,), e068, depende_de=("E002",)),
    _esc("E069", "Actualización de preferencias", "PATCH", "/api/v1/users/me/preferences", (FASE_PERFIL,), e069, muta=True, depende_de=("E068",)),
    _esc("E024", "Ingreso", "GET", "/api/v1/income", (FASE_FINANZAS,), e024, depende_de=("E002",)),
    _esc("E025", "Actualización de ingreso", "PUT", "/api/v1/income", (FASE_FINANZAS,), e025, muta=True, depende_de=("E024",)),
    _esc("E028", "Crear movimiento", "POST", "/api/v1/movements", (FASE_FINANZAS,), e028, muta=True, depende_de=("E002",), idempotencia=True),
    _esc("E029", "Detalle de movimiento", "GET", "/api/v1/movements/{movementId}", (FASE_FINANZAS,), e029, depende_de=("E028",)),
    _esc("E027", "Listar movimientos", "GET", "/api/v1/movements", (FASE_FINANZAS,), e027, depende_de=("E028",)),
    _esc("E030", "Editar movimiento", "PATCH", "/api/v1/movements/{movementId}", (FASE_FINANZAS,), e030, muta=True, depende_de=("E029",)),
    _esc("E031", "Eliminar movimiento", "DELETE", "/api/v1/movements/{movementId}", (FASE_FINANZAS,), e031, muta=True, depende_de=("E030",)),
    _esc("E033", "Crear recurrente", "POST", "/api/v1/recurring-payments", (FASE_FINANZAS,), e033, muta=True, depende_de=("E002",), idempotencia=True),
    _esc("E034", "Detalle de recurrente", "GET", "/api/v1/recurring-payments/{recurringPaymentId}", (FASE_FINANZAS,), e034, depende_de=("E033",)),
    _esc("E032", "Listar recurrentes", "GET", "/api/v1/recurring-payments", (FASE_FINANZAS,), e032, depende_de=("E033",)),
    _esc("E035", "Editar recurrente", "PATCH", "/api/v1/recurring-payments/{recurringPaymentId}", (FASE_FINANZAS,), e035, muta=True, depende_de=("E034",)),
    _esc("E037", "Pago de recurrente", "POST", "/api/v1/recurring-payments/{recurringPaymentId}/payments", (FASE_FINANZAS,), e037, muta=True, depende_de=("E033",), idempotencia=True),
    _esc("E036", "Eliminar recurrente", "DELETE", "/api/v1/recurring-payments/{recurringPaymentId}", (FASE_FINANZAS,), e036, muta=True, depende_de=("E037",)),
    _esc("E039", "Crear meta", "POST", "/api/v1/goals", (FASE_FINANZAS,), e039, muta=True, depende_de=("E002",), idempotencia=True),
    _esc("E040", "Detalle de meta", "GET", "/api/v1/goals/{goalId}", (FASE_FINANZAS,), e040, depende_de=("E039",)),
    _esc("E038", "Listar metas", "GET", "/api/v1/goals", (FASE_FINANZAS,), e038, depende_de=("E039",)),
    _esc("E041", "Editar meta", "PATCH", "/api/v1/goals/{goalId}", (FASE_FINANZAS,), e041, muta=True, depende_de=("E040",)),
    _esc("E043", "Preview de aportación", "POST", "/api/v1/goal-contributions/preview", (FASE_FINANZAS,), e043, depende_de=("E039",)),
    _esc("E044", "Crear aportación", "POST", "/api/v1/goal-contributions", (FASE_FINANZAS,), e044, muta=True, depende_de=("E039",), idempotencia=True),
    _esc("E045", "Listar aportaciones", "GET", "/api/v1/goal-contributions", (FASE_FINANZAS,), e045, depende_de=("E044",)),
    _esc("E046", "Detalle de aportación", "GET", "/api/v1/goal-contributions/{contributionId}", (FASE_FINANZAS,), e046, depende_de=("E044",)),
    _esc("E047", "Reversión de aportación", "POST", "/api/v1/goal-contributions/{contributionId}/reverse", (FASE_FINANZAS,), e047, muta=True, depende_de=("E046",), idempotencia=True),
    _esc("E042", "Eliminar meta", "DELETE", "/api/v1/goals/{goalId}", (FASE_FINANZAS,), e042, muta=True, depende_de=("E047",)),
    _esc("E026", "Resumen financiero exacto", "GET", "/api/v1/financial-summary", (FASE_FINANZAS,), e026, depende_de=("E002",)),
    _esc("E048", "Presupuestos", "GET", "/api/v1/budgets", (FASE_FINANZAS,), e048, depende_de=("E002",)),
    _esc("E049", "Definir presupuesto", "PUT", "/api/v1/budgets", (FASE_FINANZAS,), e049, muta=True, depende_de=("E048",)),
    _esc("E050", "Uso de presupuesto", "GET", "/api/v1/budgets/usage", (FASE_FINANZAS,), e050, depende_de=("E049",)),
    _esc("E051", "Dashboard", "GET", "/api/v1/dashboard", (FASE_FINANZAS,), e051, depende_de=("E026",)),
    _esc("E052", "Insights", "GET", "/api/v1/analytics/insights", (FASE_ANALITICA,), e052, depende_de=("E002",)),
    _esc("E053", "Categorías analíticas", "GET", "/api/v1/analytics/categories", (FASE_ANALITICA,), e053, depende_de=("E052",)),
    _esc("E054", "Patrones de gasto", "GET", "/api/v1/analytics/spending-patterns", (FASE_ANALITICA,), e054, depende_de=("E052",)),
    _esc("E055", "Recomendaciones ML", "GET", "/api/v1/analytics/recommendations", (FASE_ANALITICA,), e055, depende_de=("E052",)),
    _esc("E056", "Racha de ahorro", "GET", "/api/v1/analytics/savings-streak", (FASE_ANALITICA,), e056, depende_de=("E002",)),
    _esc("E057", "Preferencias de notificación", "GET", "/api/v1/notification-preferences", (FASE_NOTIFICACIONES,), e057, depende_de=("E002",), pasos=("preferencias",)),
    _esc("E058", "Actualizar preferencias de notificación", "PUT", "/api/v1/notification-preferences", (FASE_NOTIFICACIONES,), e058, muta=True, depende_de=("E057",), pasos=("preferencias",)),
    _esc("E059", "Registrar dispositivo push", "POST", "/api/v1/devices/push-tokens", (FASE_NOTIFICACIONES,), e059, muta=True, depende_de=("E002",), pasos=("dispositivo",), idempotencia=True),
    _esc("E065", "Crear recordatorio próximo", "POST", "/api/v1/reminders", (FASE_NOTIFICACIONES,), e065, muta=True, depende_de=("E002",), pasos=("recordatorio",), idempotencia=True),
    _esc("E064", "Listar recordatorios", "GET", "/api/v1/reminders", (FASE_NOTIFICACIONES,), e064, depende_de=("E065",), pasos=("listar",)),
    _esc("E061", "Bandeja tras la ocurrencia", "GET", "/api/v1/notifications", (FASE_NOTIFICACIONES,), e061, depende_de=("E065",), pasos=("polling", "bandeja")),
    _esc("E062", "Marcar la notificación exacta", "PATCH", "/api/v1/notifications/{notificationId}/read", (FASE_NOTIFICACIONES,), e062, muta=True, depende_de=("E061",), pasos=("id exacto",)),
    _esc("E063", "Marcar todas como leídas", "POST", "/api/v1/notifications/read-all", (FASE_NOTIFICACIONES,), e063, muta=True, depende_de=("E061",), pasos=("todas",)),
    _esc("E066", "Editar recordatorio", "PATCH", "/api/v1/reminders/{reminderId}", (FASE_NOTIFICACIONES,), e066, muta=True, depende_de=("E065",), pasos=("editar",)),
    _esc("E067", "Eliminar recordatorio", "DELETE", "/api/v1/reminders/{reminderId}", (FASE_NOTIFICACIONES,), e067, muta=True, depende_de=("E066",), pasos=("eliminar",)),
    _esc("E060", "Revocar dispositivo push", "DELETE", "/api/v1/devices/push-tokens/{deviceRegistrationId}", (FASE_NOTIFICACIONES,), e060, muta=True, depende_de=("E059",), pasos=("revocar",)),
    _esc("E020", "Exportación completa", "GET", "/api/v1/users/me/export", (FASE_PERFIL,), e020, depende_de=("E002",)),
    _esc("E021", "Bootstrap", "GET", "/api/v1/bootstrap", (FASE_PERFIL,), e021, depende_de=("E002",)),
    _esc("E074", "Demo bootstrap", "GET", "/api/v1/demo/bootstrap", (FASE_CONTENIDO,), e074, depende_de=("E002",)),
    _esc("E075", "Voice parse", "POST", "/api/v1/voice/parse", (FASE_CONTENIDO,), e075, depende_de=("E002",)),
    _esc("E005", "Recuperación de contraseña", "POST", "/api/v1/auth/password/forgot", (FASE_CORREO_SOLICITUD,), e005, muta=True),
    _esc("E010", "Solicitud de verificación de correo", "POST", "/api/v1/auth/email-verification/request", (FASE_CORREO_SOLICITUD,), e010, muta=True, depende_de=("E002",)),
    _esc("E006", "Restablecer contraseña", "POST", "/api/v1/auth/password/reset", (FASE_RECUPERACION,), e006, muta=True),
    _esc("E011", "Confirmar verificación de correo", "POST", "/api/v1/auth/email-verification/confirm", (FASE_VERIFICACION,), e011, muta=True),
    _esc("E014", "Eliminación definitiva de cuenta", "DELETE", "/api/v1/users/me", (FASE_ELIMINACION,), e014, muta=True, destructivo=True, depende_de=("E002",)),
)

# Orden funcional: dependencias antes que dependientes y E014 al final.
ORDEN_EJECUCION: tuple[str, ...] = (
    "E015", "E022", "E023", "E070", "E071", "E072", "E073",
    "E001", "E002", "E003", "E004", "E007", "E008", "E009",
    "E012", "E013", "E016", "E017", "E018", "E019", "E068", "E069",
    "E024", "E025",
    "E028", "E029", "E027", "E030", "E031",
    "E033", "E034", "E032", "E035", "E037", "E036",
    "E039", "E040", "E038", "E041", "E043", "E044", "E045", "E046", "E047", "E042",
    "E026",
    "E048", "E049", "E050", "E051",
    "E052", "E053", "E054", "E055", "E056",
    "E057", "E058", "E059", "E065", "E064", "E061", "E062", "E063", "E066", "E067", "E060",
    "E020", "E021", "E074", "E075",
    "E005", "E010", "E006", "E011",
    "E014",
)

ESCENARIOS: dict[str, Escenario] = {esc.eid: esc for esc in _ESCENARIOS_LISTA}

# Orden explícito de la secuencia FCM/scheduler. Se expone para que las pruebas
# verifiquen que no se adelanta E061/E062 ni se deja E060 antes de E066/E067.
ORDEN_FCM = ("E057", "E058", "E059", "E065", "E064", "E061", "E062", "E063", "E066", "E067", "E060")


def seleccionar(fases: Iterable[str], solo: Iterable[str] = ()) -> set[str]:
    """Calcula los E-ID activos para una ejecución por fases."""
    fases_limpias = [f.strip() for f in fases if f and f.strip()]
    if not fases_limpias:
        fases_limpias = [FASE_TODO]
    desconocidas = [f for f in fases_limpias if f not in FASES]
    if desconocidas:
        raise n.ConfiguracionInvalida("fases desconocidas: " + ", ".join(sorted(set(desconocidas))))
    if FASE_TODO in fases_limpias:
        activos = set(ESCENARIOS)
    else:
        activos = {
            eid
            for eid, esc in ESCENARIOS.items()
            if any(fase in esc.fases for fase in fases_limpias)
        }
    solo_limpio = {s.strip().upper() for s in solo if s and s.strip()}
    if solo_limpio:
        desconocidos = sorted(solo_limpio - set(ESCENARIOS))
        if desconocidos:
            raise n.ConfiguracionInvalida("E-ID desconocidos en --solo: " + ", ".join(desconocidos))
        activos &= solo_limpio
    return activos


def completo_por_fases(fases: Iterable[str], solo: Iterable[str] = ()) -> bool:
    return set(seleccionar(fases, solo)) == set(ESCENARIOS)


def plan_de_ejecucion(seleccion: Iterable[str]) -> list[str]:
    activos = set(seleccion)
    return [eid for eid in ORDEN_EJECUCION if eid in activos]
