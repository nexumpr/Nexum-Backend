#!/usr/bin/env python3
"""Certificación funcional de las 75 operaciones — inventario, cobertura y ejecución.

Por defecto hace dry-run: carga el OpenAPI, exige exactamente 75 `operationId`, comprueba
que cada operación tiene un escenario registrado con el mismo método y ruta, y devuelve:

    0  inventario y cobertura exactos (dry-run válido)
    2  certificación incompleta: falta/sobra una operación, método o ruta no coincide,
       la configuración no permite ejecutar o se pidió `--run` sin autorización

Con `--run --allow-mutations` construye el `nucleo.Contexto` real y ejecuta los escenarios
en `escenarios.ORDEN_EJECUCION`, respetando dependencias, selección por fases/grupos,
`--solo` y reanudación desde un archivo previo. Códigos de salida:

    0  las 75 operaciones tienen PASS (y no falta ninguna)
    1  algún FAIL / CONTRACT_MISMATCH / CONTENT_MISMATCH
    2  BLOCKED_*, NOT_RUN, cobertura incompleta, configuración inválida o fuga de secreto

Los tokens de correo (recuperación, verificación) y FCM se leen **solo** de variables de
entorno o de `getpass`; nunca de la línea de comandos. No se imprimen contraseñas ni
tokens y el resumen se audita antes de escribirse.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path
from typing import Any, Iterable, Mapping

AQUI = Path(__file__).resolve().parent
RAIZ = AQUI.parents[1]
sys.path.insert(0, str(AQUI))

import nucleo  # noqa: E402

OPENAPI_POR_DEFECTO = RAIZ / "sdd" / "03-endpoints" / "openapi" / "nexum-api-v1.json"

RC_OK = 0
RC_FALLO = 1
RC_INCOMPLETA = 2

ESTADOS_ESPERADOS = frozenset({nucleo.PASS})


# --------------------------------------------------------------------------- #
# Inventario y cobertura
# --------------------------------------------------------------------------- #

def cargar_escenarios():
    """Importa el registro de escenarios; un módulo ausente no se simula."""
    try:
        import escenarios  # type: ignore
    except Exception as exc:  # noqa: BLE001
        return None, f"no se pudo importar escenarios: {type(exc).__name__}: {exc}"
    return escenarios, ""


def inventario(ruta_openapi: Path) -> dict:
    openapi = nucleo.cargar_openapi(ruta_openapi)
    return nucleo.extraer_operaciones(openapi)


def cobertura(operaciones: dict, escenarios) -> tuple[bool, list[str]]:
    """Devuelve (exacta, problemas) sin imprimir secretos."""
    if escenarios is None:
        return False, ["registro de escenarios no disponible"]
    registro = getattr(escenarios, "ESCENARIOS", None)
    orden = getattr(escenarios, "ORDEN_EJECUCION", ())
    if not isinstance(registro, dict):
        return False, ["ESCENARIOS no es un mapping E-ID → escenario"]
    informe = nucleo.verificar_registro(operaciones, registro, orden)
    problemas: list[str] = []
    problemas.extend(f"FALTA {eid}" for eid in getattr(informe, "faltantes", []))
    problemas.extend(f"SOBRA {eid}" for eid in getattr(informe, "sobrantes", []))
    problemas.extend(getattr(informe, "duplicados", []))
    problemas.extend(getattr(informe, "discrepancias", []))
    exacta = (
        len(operaciones) == 75
        and len(registro) == 75
        and not problemas
    )
    return exacta, problemas


# --------------------------------------------------------------------------- #
# Código de salida
# --------------------------------------------------------------------------- #

def codigo_certificacion(
    resultados: Mapping[str, nucleo.Resultado],
    operaciones: Mapping[str, nucleo.Operacion],
    completo: bool,
) -> int:
    """0 solo con las 75 en PASS; 1 con fallo; 2 con bloqueo, NOT_RUN o cobertura parcial.

    `nucleo.calcular_codigo_salida` no distingue BLOCKED de PASS cuando están las 75
    entradas; aquí se exige PASS explícito, que es la condición de certificación.
    """
    estados = [resultado.estado_final() for resultado in resultados.values()]
    if any(estado in nucleo.ESTADOS_FALLO for estado in estados):
        return RC_FALLO
    if set(resultados) != set(operaciones) or not completo:
        return RC_INCOMPLETA
    if len(resultados) != len(operaciones):
        return RC_INCOMPLETA
    if any(estado not in ESTADOS_ESPERADOS for estado in estados):
        return RC_INCOMPLETA
    return RC_OK


def motivo_certificacion(
    codigo: int,
    resultados: Mapping[str, nucleo.Resultado],
    operaciones: Mapping[str, nucleo.Operacion],
) -> str:
    if codigo == RC_OK:
        return "CERTIFICACION_COMPLETA: 75/75 PASS"
    if codigo == RC_FALLO:
        fallos = [eid for eid, r in sorted(resultados.items()) if r.estado_final() in nucleo.ESTADOS_FALLO]
        return "FALLOS: " + ", ".join(fallos)
    faltan = sorted(set(operaciones) - set(resultados))
    if faltan:
        return "INCOMPLETA: faltan E-ID " + ", ".join(faltan)
    pendientes = [
        f"{eid}={r.estado_final()}"
        for eid, r in sorted(resultados.items())
        if r.estado_final() != nucleo.PASS
    ]
    return "INCOMPLETA: " + (", ".join(pendientes) if pendientes else "cobertura parcial")


# --------------------------------------------------------------------------- #
# Construcción de sesiones y opciones
# --------------------------------------------------------------------------- #

def _leer_entero(nombre: str, por_defecto: int) -> int:
    crudo = os.environ.get(nombre, "").strip()
    if not crudo:
        return por_defecto
    try:
        valor = int(crudo)
    except ValueError as exc:
        raise nucleo.ConfiguracionInvalida(f"{nombre} no es un entero") from exc
    return valor if valor > 0 else por_defecto


def _credenciales(args, secretos: nucleo.Secretos, advertencias: list[str]) -> tuple[nucleo.Sesion, nucleo.Sesion]:
    sesiones: list[nucleo.Sesion] = []
    for etiqueta, correo_arg, env_correo, env_password in (
        ("A", args.email_a, "NEXUM_EMAIL_A", "NEXUM_PASSWORD_A"),
        ("B", args.email_b, "NEXUM_EMAIL_B", "NEXUM_PASSWORD_B"),
    ):
        correo = (correo_arg or os.environ.get(env_correo, "")).strip()
        configurado = bool(correo)
        if not correo:
            correo = f"nexum.cert.{etiqueta.lower()}.{nucleo.idempotencia()[:8]}@example.invalid"
            advertencias.append(
                f"cuenta {etiqueta}: sin --email-{etiqueta.lower()} ni {env_correo}; "
                "se usará un correo sintético de dominio .invalid"
            )
        password = os.environ.get(env_password, "").strip()
        if not password and configurado:
            password = nucleo.leer_secreto(env_password, f"Contraseña de la cuenta {etiqueta}")
        origen = "entorno"
        if not password:
            password = "Nx7!" + nucleo.idempotencia()[:20]
            origen = "generado"
            advertencias.append(
                f"cuenta {etiqueta}: contraseña generada en memoria; "
                f"exporte {env_password} para reanudar fases posteriores"
            )
        secretos.registrar_correo(correo)
        secretos.registrar("contraseña", password)
        sesiones.append(
            nucleo.Sesion(
                etiqueta=etiqueta,
                email=correo,
                password=password,
                password_origen=origen,
                device_id=nucleo.nuevo_uuid(),
            )
        )
    return sesiones[0], sesiones[1]


def _token(nombre: str, descripcion: str, necesario: bool) -> str:
    """Token de correo/FCM: entorno o getpass; nunca argv ni impresión."""
    return nucleo.leer_secreto(nombre, descripcion, permitir_prompt=necesario)


def _opciones(args, base_url: str, fases: tuple[str, ...], solo: tuple[str, ...], seleccion: Iterable[str]) -> nucleo.Opciones:
    activos = set(seleccion)
    return nucleo.Opciones(
        fases=fases,
        solo=solo,
        permitir_mutaciones=bool(args.allow_mutations),
        permitir_eliminar_cuenta=bool(args.allow_delete_account),
        fcm_token=_token("NEXUM_FCM_TOKEN", "Token FCM para E059/E060", bool(activos & {"E059", "E060"})),
        reset_token=_token("NEXUM_RESET_TOKEN", "Token de recuperación del buzón (E006)", "E006" in activos),
        verify_token=_token("NEXUM_VERIFY_TOKEN", "Token de verificación del buzón (E011)", "E011" in activos),
        fcm_receipt_file=os.environ.get("NEXUM_FCM_RECEIPT_FILE", "").strip(),
        espera_ocurrencia_s=_leer_entero("NEXUM_ESPERA_OCURRENCIA_S", args.espera_ocurrencia),
        intervalo_poll_s=_leer_entero("NEXUM_INTERVALO_POLL_S", args.intervalo_poll),
        verbose=bool(args.verbose),
        reanudar=args.reanudar,
        out=args.out,
        base_url=base_url,
    )


# --------------------------------------------------------------------------- #
# Ejecución
# --------------------------------------------------------------------------- #

def _imprimir(documento: Mapping[str, Any], secretos: nucleo.Secretos | None = None) -> None:
    texto = json.dumps(documento, ensure_ascii=False)
    print(secretos.redactar(texto) if secretos is not None else texto)


def _invalido(estado: str, detalle: str, extra: Mapping[str, Any] | None = None) -> int:
    documento: dict[str, Any] = {
        "estado": estado,
        "detalle": detalle,
        "aws_consultado": False,
        "ejecutado_contra_produccion": False,
    }
    if extra:
        documento.update(extra)
    _imprimir(documento)
    return RC_INCOMPLETA


def _dependencias(ctx: nucleo.Contexto, escenario: Any, resultado: nucleo.Resultado) -> bool:
    for dependencia in getattr(escenario, "depende_de", ()):
        if not ctx.dependencia(dependencia, resultado):
            return False
    return True


def _autorizado(autorizacion: nucleo.Autorizacion, escenario: Any, resultado: nucleo.Resultado) -> bool:
    if getattr(escenario, "destructivo", False) and not autorizacion.permitir_eliminar_cuenta:
        resultado.bloqueo(
            "eliminación autorizada",
            "requiere --allow-delete-account",
            nucleo.BLOCKED_EXTERNAL,
        )
        return False
    if getattr(escenario, "muta", False) and not autorizacion.permitir_mutaciones:
        resultado.bloqueo(
            "mutaciones autorizadas",
            "requiere --allow-mutations",
            nucleo.BLOCKED_EXTERNAL,
        )
        return False
    return True


def _cargar_reanudacion(
    ruta: str,
    seleccion: set[str],
    operaciones: Mapping[str, nucleo.Operacion],
    base_url: str,
) -> tuple[dict[str, nucleo.Resultado], str]:
    try:
        previo = json.loads(Path(ruta).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise nucleo.ConfiguracionInvalida(f"no se pudo leer --reanudar: {type(exc).__name__}") from exc
    if not isinstance(previo, dict):
        raise nucleo.ConfiguracionInvalida("--reanudar no contiene un objeto JSON")
    informe = nucleo.fusionar_reanudacion(
        previo,
        seleccion,
        operaciones,
        base_url,
        nucleo.digest_inventario(operaciones),
    )
    if informe.problemas:
        raise nucleo.ConfiguracionInvalida("; ".join(informe.problemas[:5]))
    return dict(informe.fusionados), ruta


def _ejecutar(
    args,
    openapi: Mapping[str, Any],
    operaciones: Mapping[str, nucleo.Operacion],
    escenarios,
    exacta: bool,
    problemas: list[str],
) -> int:
    if escenarios is None:
        return _invalido("COBERTURA_INCOMPLETA", "registro de escenarios no disponible", {"problemas": problemas})
    if not exacta:
        return _invalido("COBERTURA_INCOMPLETA", "la cobertura no es 75/75", {"problemas": problemas[:50]})
    if args.allow_delete_account and not args.allow_mutations:
        return _invalido(
            "CONFIGURACION_INVALIDA",
            "--allow-delete-account exige --allow-mutations",
        )
    if not args.allow_mutations:
        return _invalido(
            "MUTACIONES_NO_AUTORIZADAS",
            "ejecutar la certificación requiere --allow-mutations",
        )
    try:
        base_url = nucleo.validar_base_url(args.base_url)
    except nucleo.ConfiguracionInvalida as exc:
        return _invalido("CONFIGURACION_INVALIDA", str(exc))

    fases = tuple(args.grupo or ("todo",))
    solo = tuple(args.solo or ())
    try:
        seleccion = set(escenarios.seleccionar(fases, solo))
    except nucleo.ConfiguracionInvalida as exc:
        return _invalido("CONFIGURACION_INVALIDA", str(exc))
    if not seleccion:
        return _invalido("SELECCION_VACIA", "la selección no contiene ningún E-ID")
    plan = escenarios.plan_de_ejecucion(seleccion)
    if set(plan) != seleccion:
        return _invalido(
            "ORDEN_INCOMPLETO",
            "ORDEN_EJECUCION no cubre exactamente la selección",
            {"faltan": sorted(seleccion - set(plan))},
        )

    # La reanudación se valida antes de pedir credenciales o tokens: un archivo
    # inválido debe fallar sin abrir ninguna entrada interactiva.
    reanudados: dict[str, nucleo.Resultado] = {}
    try:
        if args.reanudar:
            reanudados, _ = _cargar_reanudacion(args.reanudar, seleccion, operaciones, base_url)
    except nucleo.ConfiguracionInvalida as exc:
        return _invalido("REANUDACION_INVALIDA", str(exc))

    secretos = nucleo.Secretos()
    advertencias: list[str] = [
        "E005 envía correo por diseño; para certificar E006 sin un envío nuevo use "
        "--grupo recuperacion --solo E006 y NEXUM_RESET_TOKEN",
    ]
    try:
        a, b = _credenciales(args, secretos, advertencias)
        opciones = _opciones(args, base_url, fases, solo, seleccion)
    except nucleo.ConfiguracionInvalida as exc:
        return _invalido("CONFIGURACION_INVALIDA", str(exc))

    autorizacion = nucleo.Autorizacion(
        permitir_mutaciones=bool(args.allow_mutations),
        permitir_eliminar_cuenta=bool(args.allow_delete_account),
    )
    validador = nucleo.ValidadorEsquema(openapi, operaciones)
    try:
        cliente = nucleo.Cliente(
            base_url,
            autorizacion,
            escenarios.ESCENARIOS,
            validador,
            timeout=max(1, int(args.timeout)),
        )
    except (nucleo.ConfiguracionInvalida, OSError) as exc:
        return _invalido("CONFIGURACION_INVALIDA", f"no se pudo crear el cliente HTTPS: {type(exc).__name__}")

    resultados: dict[str, nucleo.Resultado] = dict(reanudados)
    ctx = nucleo.Contexto(
        cliente,
        autorizacion,
        a,
        b,
        opciones,
        resultados,
        operaciones,
        escenarios.ESCENARIOS,
        seleccion,
        secretos,
        reanudados,
    )
    ctx.advertencias.extend(advertencias)
    if reanudados:
        ctx.advertencias.append(
            f"reanudado desde {args.reanudar}: solo se heredan PASS/BLOCKED de E-ID no seleccionados"
        )

    for eid in plan:
        escenario = escenarios.ESCENARIOS[eid]
        resultado = ctx.reg(eid)
        if not _dependencias(ctx, escenario, resultado):
            resultado.cerrar()
            continue
        if not _autorizado(autorizacion, escenario, resultado):
            resultado.cerrar()
            continue
        try:
            escenario.funcion(ctx)
        except nucleo.MutacionDenegada as exc:
            resultado.bloqueo("mutación autorizada", secretos.redactar(str(exc))[:200], nucleo.BLOCKED_EXTERNAL)
        except (nucleo.SolicitudInvalida, nucleo.ConfiguracionInvalida) as exc:
            resultado.fallo(
                "solicitud conforme al contrato",
                secretos.redactar(str(exc))[:200],
                nucleo.CONTRACT_MISMATCH,
            )
        except Exception as exc:  # noqa: BLE001 - el harness no debe abortar el resto
            resultado.fallo("ejecución sin excepción", type(exc).__name__, nucleo.FAIL)
        if not resultado.checks:
            resultado.fallo("resultado producido", "el escenario no registró ningún check", nucleo.FAIL)
        resultado.cerrar()
        if opciones.verbose:
            print(f"{eid} {resultado.estado_final()}", file=sys.stderr)

    for eid in sorted(seleccion - set(ctx.resultados)):
        pendiente = ctx.reg(eid)
        pendiente.motivo = "NO_EJECUTADO"
        pendiente.cerrar()

    completo = set(ctx.resultados) == set(operaciones) and len(ctx.resultados) == len(operaciones)
    codigo = codigo_certificacion(ctx.resultados, operaciones, completo)
    resumen = nucleo.construir_resumen(
        ctx.resultados,
        operaciones,
        fecha=nucleo.iso_utc(),
        base_url=base_url,
        openapi_ruta=str(Path(args.openapi).resolve()),
        openapi_sha256=nucleo.sha256_archivo(args.openapi),
        fases=fases,
        solo=solo,
        mutaciones=bool(args.allow_mutations),
        eliminacion=bool(args.allow_delete_account),
        completo=completo,
        codigo=codigo,
        bloqueos=ctx.bloqueos,
        advertencias=ctx.advertencias,
        llamadas=cliente.llamadas,
        reanudado_de=args.reanudar if reanudados else "",
    )
    resumen["estado"] = (
        "CERTIFICACION_COMPLETA"
        if codigo == RC_OK
        else "CERTIFICACION_CON_FALLOS"
        if codigo == RC_FALLO
        else "CERTIFICACION_INCOMPLETA"
    )
    resumen["cobertura_exacta"] = exacta
    resumen["seleccion"] = sorted(seleccion)
    resumen["motivo"] = motivo_certificacion(codigo, ctx.resultados, operaciones)

    fugas = secretos.auditar(resumen)
    if fugas:
        return _invalido(
            "FUGA_DE_SECRETO",
            "los resultados contendrían datos sensibles; no se escriben ni se imprimen",
            {"hallazgos": sorted(set(fugas))},
        )
    if args.out:
        try:
            Path(args.out).parent.mkdir(parents=True, exist_ok=True)
            nucleo.escribir_resumen(args.out, resumen, secretos)
        except (OSError, nucleo.FugaDeSecreto) as exc:
            return _invalido("ESCRITURA_FALLIDA", f"no se pudo escribir --out: {type(exc).__name__}")
    _imprimir(resumen, secretos)
    return codigo


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #

def _parser() -> argparse.ArgumentParser:
    ap = argparse.ArgumentParser(description="Certificación funcional 75/75 (inventario, cobertura y ejecución)")
    ap.add_argument("--openapi", default=str(OPENAPI_POR_DEFECTO))
    ap.add_argument("--run", action="store_true", help="ejecutar escenarios (requiere --allow-mutations)")
    ap.add_argument("--allow-mutations", action="store_true", help="autoriza peticiones que mutan estado")
    ap.add_argument(
        "--allow-delete-account",
        action="store_true",
        help="autoriza E014 (elimina la cuenta A); exige --allow-mutations",
    )
    ap.add_argument("--base-url", default="", help="origen HTTPS, por ejemplo https://api.example")
    ap.add_argument("--out", default="", help="archivo JSON de resultados (se audita antes de escribir)")
    ap.add_argument(
        "--grupo",
        "--fase",
        dest="grupo",
        action="append",
        default=None,
        metavar="GRUPO",
        help="grupo/fase de escenarios: publico, auth, perfil, finanzas, analitica, "
        "notificaciones, contenido, correo-solicitud, recuperacion, verificacion, eliminacion, todo",
    )
    ap.add_argument(
        "--solo",
        action="append",
        default=None,
        metavar="EID",
        help="restringe a un E-ID (repetible); se combina con --grupo",
    )
    ap.add_argument("--reanudar", default="", help="archivo de resultados previo para heredar PASS")
    ap.add_argument("--email-a", default="", help="correo de la cuenta A (alternativa a NEXUM_EMAIL_A)")
    ap.add_argument("--email-b", default="", help="correo de la cuenta B (alternativa a NEXUM_EMAIL_B)")
    ap.add_argument("--timeout", type=int, default=20, help="timeout HTTP en segundos")
    ap.add_argument("--espera-ocurrencia", type=int, default=90, help="espera máxima de polling (s)")
    ap.add_argument("--intervalo-poll", type=int, default=10, help="intervalo de polling (s)")
    ap.add_argument("--verbose", action="store_true", help="progreso por E-ID en stderr")
    return ap


def main(argv: list[str]) -> int:
    args = _parser().parse_args(argv)

    try:
        openapi = nucleo.cargar_openapi(args.openapi)
        operaciones = nucleo.extraer_operaciones(openapi)
    except Exception as exc:  # noqa: BLE001
        return _invalido("INVENTARIO_INVALIDO", f"{type(exc).__name__}: {exc}")

    escenarios, error_escenarios = cargar_escenarios()
    exacta, problemas = cobertura(operaciones, escenarios)
    if error_escenarios:
        problemas.append(error_escenarios)

    if args.run:
        return _ejecutar(args, openapi, operaciones, escenarios, exacta, problemas)

    resumen = {
        "operaciones_openapi": len(operaciones),
        "escenarios": len(getattr(escenarios, "ESCENARIOS", {}) or {}),
        "cobertura_exacta": exacta,
        "problemas": problemas[:50],
        "modo": "dry-run",
        "aws_consultado": False,
        "ejecutado_contra_produccion": False,
    }
    resumen["estado"] = "COBERTURA_EXACTA" if exacta else "COBERTURA_INCOMPLETA"
    if args.out:
        try:
            Path(args.out).parent.mkdir(parents=True, exist_ok=True)
            Path(args.out).write_text(json.dumps(resumen, ensure_ascii=False, indent=2), encoding="utf-8")
        except OSError:
            return _invalido("ESCRITURA_FALLIDA", "no se pudo escribir --out")
    _imprimir(resumen)
    return RC_OK if exacta else RC_INCOMPLETA


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
