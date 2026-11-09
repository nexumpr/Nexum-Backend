"""Recuperacion de I-130: RTO/RPO, vigencia de backups y state Terraform.

Complementa `verificar_backup_restore.py`. Todo es local: no se consulta AWS, no
se ejecuta Terraform y no se restaura ningun dato real.

Codigos: 0 correcto, 1 bloqueado/incumplido, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import date, datetime, timezone
from decimal import Decimal
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_backup_restore as br  # noqa: E402

INVENTARIO_DEFECTO = br.INVENTARIO_DEFECTO

RTO_RPO_PENDING = "RTO_RPO_PENDING_HUMAN_APPROVAL"
STATE_AWS_NOT_VERIFIED = "TERRAFORM_STATE_RECOVERY_AWS_NOT_VERIFIED"

FASES_RECUPERACION = (
    "incidente",
    "contencion",
    "pausa_escrituras",
    "identificacion_respaldo",
    "verificacion",
    "revision_finops",
    "gate_humano",
    "restore_aislado",
    "validacion_financiera",
    "reconciliacion",
    "reanudacion",
    "monitoreo",
    "cierre",
)


def matriz_rto_rpo(inventario: Mapping[str, Any]) -> dict[str, Any]:
    """Seis capas: propuesto, ratificado, medido, capacidad, simulacion y AWS."""
    objetivos = inventario.get("objetivos", {})
    ratificacion = inventario.get("ratificacion_objetivos", {})
    ratificado = bool(ratificacion.get("aprobado"))
    return {
        "comando": "matriz-rto-rpo",
        "objetivo_propuesto": {
            "rto_horas": str(objetivos.get("rto_horas")),
            "rpo_horas": str(objetivos.get("rpo_horas")),
            "fuente": "spec I-130 INF-130-01 (propuesta revisable)",
        },
        "objetivo_ratificado": {
            "aprobado": ratificado,
            "fecha": ratificacion.get("fecha"),
            "referencia": ratificacion.get("referencia"),
        },
        "objetivo_medido": {"medido": bool(objetivos.get("medido")), "fuente": "restore real en AWS"},
        "capacidad_teorica": {
            "rpo": "PITR continuo hasta 35 dias; 24 h si solo hay backup diario",
            "rto": "depende del tamano de la tabla y del tiempo de crear la tabla nueva",
            "limite": "no se promete mas de lo que el mecanismo conserva",
        },
        "simulacion_local": {"estado": "SIMULADO_CON_FIXTURES", "acredita_cumplimiento": False},
        "resultado_aws": {"estado": "NOT_RUN", "medido": False},
        "estado": "RATIFICADO" if ratificado else RTO_RPO_PENDING,
        "advertencia": (
            "Una simulacion local no declara RTO/RPO cumplidos; solo el restore real medido "
            "puede cambiar objetivo_medido."
        ),
    }


def medir_recuperacion(
    solicitud: Mapping[str, Any] | None, inventario: Mapping[str, Any]
) -> dict[str, Any]:
    """Mide fases declaradas y comprueba RTO/RPO de forma determinista."""
    if solicitud is None:
        raise br.ErrorUso("Se requiere --solicitud con 'fases' y 'perdida_maxima_horas'.")
    fases = solicitud.get("fases")
    if not isinstance(fases, list):
        raise br.ErrorUso("'fases' debe ser una lista de objetos con nombre y duracion_min.")

    nombres = [str(fase.get("nombre", "")) for fase in fases]
    faltantes = [nombre for nombre in FASES_RECUPERACION if nombre not in nombres]
    desconocidas = [nombre for nombre in nombres if nombre not in FASES_RECUPERACION]
    orden_ok = nombres == [n for n in FASES_RECUPERACION if n in nombres]

    total_min = Decimal("0")
    for fase in fases:
        duracion = Decimal(str(fase.get("duracion_min", "-1")))
        if duracion < 0:
            raise br.ErrorUso(f"duracion_min invalida en fase {fase.get('nombre')!r}")
        total_min += duracion

    objetivos = inventario.get("objetivos", {})
    rto = Decimal(str(objetivos.get("rto_horas", "4"))) * Decimal("60")
    rpo = Decimal(str(objetivos.get("rpo_horas", "24")))
    perdida = Decimal(str(solicitud.get("perdida_maxima_horas", "-1")))
    if perdida < 0:
        raise br.ErrorUso("'perdida_maxima_horas' debe ser >= 0.")

    cumple_rto = total_min <= rto and not faltantes and orden_ok and not desconocidas
    cumple_rpo = perdida <= rpo
    return {
        "comando": "medir-recuperacion",
        "duracion_total_min": str(br.cuantizar(total_min)),
        "duracion_total_horas": str(br.cuantizar(total_min / Decimal("60"))),
        "rto_objetivo_horas": str(objetivos.get("rto_horas")),
        "rpo_objetivo_horas": str(objetivos.get("rpo_horas")),
        "perdida_maxima_horas": str(perdida),
        "fases_faltantes": faltantes,
        "fases_desconocidas": desconocidas,
        "orden_correcto": orden_ok,
        "cumple_rto_simulado": cumple_rto,
        "cumple_rpo_simulado": cumple_rpo,
        "estado": "CUMPLE_SIMULADO" if (cumple_rto and cumple_rpo) else "INCUMPLE_SIMULADO",
        "alcance": "SIMULACION_LOCAL",
        "advertencia": "Simulacion con duraciones declaradas; no acredita RTO/RPO en AWS.",
    }


def evaluar_backup(
    solicitud: Mapping[str, Any] | None,
    hoy: date | None = None,
) -> dict[str, Any]:
    """Vigencia de un backup declarado: existencia, tabla, retencion y caducidad."""
    if solicitud is None:
        raise br.ErrorUso("Se requiere --solicitud con los datos del backup.")
    backup = solicitud.get("backup")
    if not isinstance(backup, Mapping) or not backup.get("id"):
        return {
            "comando": "estado-backup",
            "estado": "NO_EXISTE",
            "valido": False,
            "motivo": "backup inexistente",
        }

    retencion = int(backup.get("retencion_dias", 0))
    tabla_activa = solicitud.get("tabla_activa")
    fecha_txt = str(backup.get("fecha", ""))
    try:
        fecha = datetime.strptime(fecha_txt, "%Y-%m-%d").date()
    except ValueError as exc:
        raise br.ErrorUso(f"fecha de backup invalida: {fecha_txt!r} ({exc})") from exc
    edad = ((hoy or datetime.now(timezone.utc).date()) - fecha).days

    if backup.get("tabla") != tabla_activa:
        return {"comando": "estado-backup", "estado": "TABLA_NO_COINCIDE", "valido": False, "edad_dias": edad}
    if retencion < 1 or retencion > 35:
        return {"comando": "estado-backup", "estado": "RETENCION_INVALIDA", "valido": False, "edad_dias": edad}
    if edad > retencion:
        return {"comando": "estado-backup", "estado": "CADUCADO", "valido": False, "edad_dias": edad}
    return {
        "comando": "estado-backup",
        "estado": "DISPONIBLE",
        "valido": True,
        "edad_dias": edad,
        "retencion_dias": retencion,
        "advertencia": "Vigencia calculada sobre datos declarados; no se consulto AWS.",
    }


def plan_recuperacion_state(solicitud: Mapping[str, Any] | None) -> dict[str, Any]:
    """Ocho requisitos del procedimiento de recuperacion del state."""
    if solicitud is None:
        raise br.ErrorUso("Se requiere --solicitud con la recuperacion de state.")
    requisitos = {
        "version_identificada": bool(solicitud.get("version_id")),
        "integridad_verificada": solicitud.get("sha256_esperado") not in (None, "")
        and solicitud.get("sha256_esperado") == solicitud.get("sha256_observado"),
        "aprobacion_registrada": bool(solicitud.get("aprobacion")),
        "state_actual_preservado": bool(solicitud.get("copia_state_actual")),
        "reversion_posible": bool(solicitud.get("copia_state_actual")) and bool(solicitud.get("version_id")),
        "escritor_unico": bool(solicitud.get("token_escritura")),
        "sin_secretos_en_logs": solicitud.get("registra_secretos") is not True,
        "gate_despliegue": bool(solicitud.get("gate")),
    }
    faltan = sorted(clave for clave, valor in requisitos.items() if not valor)
    return {
        "comando": "plan-state",
        "estado": "PLAN_LOCAL_VALIDADO" if not faltan else "RECUPERACION_RECHAZADA",
        "requisitos": requisitos,
        "faltantes": faltan,
        "alcance_aws": STATE_AWS_NOT_VERIFIED,
        "version_id": solicitud.get("version_id"),
        "advertencia": (
            "Recuperar una version del state no es una operacion de datos: exige Gate, copia "
            "previa y escritor unico. No se ejecuto ningun comando S3."
        ),
    }


def verificar_concurrencia(solicitud: Mapping[str, Any] | None) -> dict[str, Any]:
    """Un solo escritor autorizado sobre el state."""
    if solicitud is None:
        raise br.ErrorUso("Se requiere --solicitud con 'token_activo' y 'solicitante'.")
    activo = solicitud.get("token_activo")
    solicitante = solicitud.get("solicitante")
    if activo and activo != solicitante:
        return {
            "comando": "concurrencia",
            "estado": "ESCRITOR_CONCURRENTE",
            "permite": False,
            "motivo": f"token activo de {activo}; solicitante {solicitante}",
        }
    return {
        "comando": "concurrencia",
        "estado": "ESCRITOR_UNICO",
        "permite": True,
        "motivo": "un solo escritor autorizado sobre el state",
    }


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="verificar_recuperacion.py",
        description="RTO/RPO, vigencia de backups y recuperacion de state (I-130). Sin AWS.",
    )
    parser.add_argument(
        "comando",
        help="matriz-rto-rpo | medir-recuperacion | estado-backup | plan-state | concurrencia",
    )
    parser.add_argument("--inventario", default=str(INVENTARIO_DEFECTO))
    parser.add_argument("--solicitud", default=None)
    parser.add_argument("--hoy", default=None, help="Fecha YYYY-MM-DD para pruebas deterministas")
    parser.add_argument("--salida", default=None)
    return parser


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    comando = args.comando.strip().lower()
    inventario = br.cargar_inventario(args.inventario)
    solicitud = br.cargar_json(args.solicitud) if args.solicitud else None

    if comando == "matriz-rto-rpo":
        resultado = matriz_rto_rpo(inventario)
    elif comando == "medir-recuperacion":
        resultado = medir_recuperacion(solicitud, inventario)
    elif comando == "estado-backup":
        hoy = datetime.strptime(args.hoy, "%Y-%m-%d").date() if args.hoy else None
        resultado = evaluar_backup(solicitud, hoy)
    elif comando == "plan-state":
        resultado = plan_recuperacion_state(solicitud)
    elif comando == "concurrencia":
        resultado = verificar_concurrencia(solicitud)
    else:
        raise br.ErrorUso(
            f"Comando desconocido: {comando!r}. Solo existen: matriz-rto-rpo, "
            "medir-recuperacion, estado-backup, plan-state, concurrencia."
        )

    resultado["aws_consultado"] = False
    resultado["terraform_ejecutado"] = False
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )

    bloqueantes = (
        "INCUMPLE_SIMULADO",
        "NO_EXISTE",
        "CADUCADO",
        "RETENCION_INVALIDA",
        "TABLA_NO_COINCIDE",
        "RECUPERACION_RECHAZADA",
        "ESCRITOR_CONCURRENTE",
    )
    return (1 if resultado.get("estado") in bloqueantes else 0), resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except (br.ErrorUso, br.ErrorInventario) as exc:
        print(f"DENEGADO: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
