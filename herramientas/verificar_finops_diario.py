"""Control FinOps diario LOCAL de Nexum-Backend (I-120, seccion 10).

Evalua un archivo de evidencia y decide si se pueden abrir ventanas auxiliares.
Nunca consulta AWS, nunca apaga el core, nunca cambia el plan Free.

Reglas:

- Sin evidencia de creditos, cuenta o SCP el resultado es ``BLOQUEADO_SIN_DATOS``
  (falla cerrado). No se inventa saldo ni se declara gasto cero.
- Umbrales vigentes (spec I-120 seccion 10 e I-000): ``<= 45`` aviso,
  ``<= 35`` bloqueo de cambios auxiliares, ``<= 30`` revision urgente,
  ``< 20`` incidente critico, forecast > credito restante bloquea despliegues.
- Los umbrales son alertas, no un hard cap: no detienen recursos.
- El core 24x7 nunca se apaga por decision de este programa.

Codigos: 0 sin bloqueo, 1 bloqueado/alerta, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import date, datetime, timezone
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
INVENTARIO_DEFECTO = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"

CUATRO_DECIMALES = Decimal("0.0001")

CAMPOS_OBLIGATORIOS = (
    "tipo_cuenta",
    "plan",
    "plan_activo",
    "creditos_usd",
    "consumo_usd",
    "forecast_usd",
    "horizonte_dias",
)

# Una cifra sin bandera de verificacion no sirve para decidir.
CAMPOS_VERIFICABLES = ("plan_activo", "creditos_usd", "consumo_usd", "forecast_usd")

# Requisitos de cuenta para autorizar cambios auxiliares (I-000 Gate).
CAMPOS_CUENTA = ("cuenta_verificada", "scp_verificado")

PLAN_APROBADO = "FREE"

NIVELES_BLOQUEANTES = (
    "BLOQUEADO_SIN_DATOS",
    "BLOQUEADO_PLAN_NO_APROBADO",
    "BLOQUEADO_CREDITOS_VENCIDOS",
    "INCIDENTE_CRITICO",
    "REVISION_URGENTE",
    "BLOQUEADO",
    "BLOQUEO_FORECAST",
)

NO_HACE = [
    "no apaga el core ni reduce instancias",
    "no cambia el plan Free ni autoriza Paid",
    "no ejecuta consultas AWS ni de facturacion",
    "no actua como hard cap de gasto",
]


class ErrorEvidencia(Exception):
    """Evidencia ausente o incoherente (codigo 2)."""


def cuantizar(valor: Decimal) -> Decimal:
    return valor.quantize(CUATRO_DECIMALES, rounding=ROUND_HALF_UP)


def cargar_json(ruta: Path | str) -> Any:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorEvidencia(f"No se puede leer la evidencia {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorEvidencia(f"La evidencia {ruta} no es JSON valido: {exc}") from exc


def umbrales_de_inventario(ruta: Path | str = INVENTARIO_DEFECTO) -> dict[str, Any]:
    datos = cargar_json(ruta)
    return dict(datos.get("umbrales_credito_usd", {}))


def _decimal_obligatorio(evidencia: Mapping[str, Any], campo: str) -> Decimal:
    valor = evidencia.get(campo)
    if valor is None:
        raise ErrorEvidencia(f"Falta '{campo}': sin dato no se asume saldo ni gasto.")
    if isinstance(valor, bool):
        raise ErrorEvidencia(f"'{campo}' no puede ser booleano.")
    texto = str(valor).strip().lower()
    if texto in ("nan", "inf", "+inf", "-inf", "infinity", "-infinity"):
        raise ErrorEvidencia(f"'{campo}' no admite NaN ni infinitos.")
    try:
        numero = Decimal(texto)
    except Exception as exc:
        raise ErrorEvidencia(f"'{campo}' no es decimal: {exc}") from exc
    if not numero.is_finite():
        raise ErrorEvidencia(f"'{campo}' no admite NaN ni infinitos.")
    if numero < 0:
        raise ErrorEvidencia(f"'{campo}' no puede ser negativo: {numero}.")
    return numero


def _campos_sin_verificar(evidencia: Mapping[str, Any]) -> list[str]:
    return [
        campo for campo in CAMPOS_VERIFICABLES
        if evidencia.get("verificado_" + campo) is not True
    ]


def _campos_cuenta_sin_verificar(evidencia: Mapping[str, Any]) -> list[str]:
    return [campo for campo in CAMPOS_CUENTA if evidencia.get(campo) is not True]


def datos_faltantes(evidencia: Mapping[str, Any]) -> list[str]:
    faltan = [campo for campo in CAMPOS_OBLIGATORIOS if evidencia.get(campo) in (None, "")]
    if evidencia.get("plan_activo") is False:
        faltan.append("plan_activo=false")
    return faltan


def nivel_por_saldo(valor: Decimal, umbrales: Mapping[str, Any]) -> str:
    aviso = Decimal(str(umbrales.get("aviso", "45")))
    bloqueo = Decimal(str(umbrales.get("bloqueo_auxiliares", "35")))
    revision = Decimal(str(umbrales.get("revision_urgente", "30")))
    critico = Decimal(str(umbrales.get("incidente_critico", "20")))
    if valor < critico:
        return "INCIDENTE_CRITICO"
    if valor <= revision:
        return "REVISION_URGENTE"
    if valor <= bloqueo:
        return "BLOQUEADO"
    if valor <= aviso:
        return "AVISO"
    return "OK"


def evaluar(
    evidencia: Mapping[str, Any],
    umbrales: Mapping[str, Any],
    hoy: date | None = None,
) -> dict[str, Any]:
    """Devuelve nivel, bloqueo de auxiliares y acciones. Falla cerrado."""
    faltan = datos_faltantes(evidencia)
    base = {
        "autoriza_ejecucion_real": False,
        "procedencia_evidencia": "DECLARADA_LOCAL",
        "es_prueba_independiente_de_facturacion": False,
        "motivo_ejecucion": (
            "I-120 LOCAL: un JSON con verificado=true no es prueba independiente de "
            "facturacion AWS; la ejecucion real exige Gate de despliegue y datos de "
            "facturacion autorizados."
        ),
        "no_hace": list(NO_HACE),
        "umbrales": dict(umbrales),
        "fuente_datos": evidencia.get("fuente", "NO_DECLARADA"),
        "datos_no_verificados": [
            campo
            for campo in ("creditos_usd", "consumo_usd", "forecast_usd", "plan_activo")
            if evidencia.get(f"verificado_{campo}") is not True
        ],
    }

    sin_verificar = _campos_sin_verificar(evidencia)
    if faltan or sin_verificar:
        motivo = "Evidencia incompleta: " + ", ".join(faltan) if faltan else (
            "Cifras sin verificacion: " + ", ".join(sin_verificar)
        )
        return {
            **base,
            "nivel": "BLOQUEADO_SIN_DATOS",
            "bloquea_auxiliares": True,
            "requiere_escalado": True,
            "motivo": motivo,
            "datos_no_verificados": sin_verificar or base["datos_no_verificados"],
            "acciones": [
                "obtener evidencia autorizada de cuenta, creditos y forecast",
                "mantener cerradas las ventanas auxiliares",
                "no declarar AWS_CREDITS ni AWS_COST_REAL como verificados",
            ],
        }

    plan = str(evidencia.get("plan", "")).strip().upper()
    if plan != PLAN_APROBADO:
        return {
            **base,
            "nivel": "BLOQUEADO_PLAN_NO_APROBADO",
            "bloquea_auxiliares": True,
            "requiere_escalado": True,
            "motivo": f"El plan declarado ({plan or 'VACIO'}) no es {PLAN_APROBADO}: no se autoriza nada.",
            "acciones": ["no cambiar el plan AWS y escalar al responsable"],
        }

    creditos = _decimal_obligatorio(evidencia, "creditos_usd")
    consumo = _decimal_obligatorio(evidencia, "consumo_usd")
    forecast = _decimal_obligatorio(evidencia, "forecast_usd")
    horizonte = int(evidencia["horizonte_dias"])
    if horizonte <= 0:
        raise ErrorEvidencia("'horizonte_dias' debe ser positivo.")

    # Semantica: si existe saldo restante explicito se usa tal cual; no se
    # descuenta dos veces el consumo.
    restante_explicito = evidencia.get("creditos_restante_usd")
    if restante_explicito is not None:
        restante = _decimal_obligatorio(evidencia, "creditos_restante_usd")
        restante_declarado = "creditos_restante_usd"
    else:
        restante = creditos - consumo
        restante_declarado = "creditos_usd-consumo_usd"
    if restante < 0:
        raise ErrorEvidencia("El saldo restante resulta negativo: revisar la evidencia.")
    nivel = nivel_por_saldo(restante, umbrales)
    acciones: list[str] = []
    bloquea = nivel in ("INCIDENTE_CRITICO", "REVISION_URGENTE", "BLOQUEADO")

    if forecast > restante:
        nivel = "BLOQUEO_FORECAST"
        bloquea = True
        acciones.append("forecast > credito restante: bloquear nuevos despliegues auxiliares")
    if nivel == "AVISO":
        acciones.append("saldo en aviso: reducir ventanas y revisar consumo diario")
    if nivel == "BLOQUEADO":
        acciones.append("bloquear cambios auxiliares no esenciales")
    if nivel == "REVISION_URGENTE":
        acciones.append("escalar revision urgente al responsable")
    if nivel == "INCIDENTE_CRITICO":
        acciones.append("incidente critico: escalar de inmediato, sin tocar el core")
    if nivel in ("OK", "AVISO") and not acciones:
        acciones.append("sin accion: el core permanece encendido 24x7")

    vencimiento = evidencia.get("creditos_vencimiento")
    dias_para_vencer = None
    vencido = False
    if vencimiento:
        try:
            fecha = datetime.strptime(str(vencimiento), "%Y-%m-%d").date()
            dias_para_vencer = (fecha - (hoy or date.today())).days
        except ValueError as exc:
            raise ErrorEvidencia(f"creditos_vencimiento invalido: {exc}") from exc
        if dias_para_vencer < 0:
            vencido = True
            nivel = "BLOQUEADO_CREDITOS_VENCIDOS"
            bloquea = True
            acciones.append("los creditos estan vencidos: no se autoriza ningun cambio auxiliar")
        elif dias_para_vencer < horizonte:
            bloquea = True
            acciones.append(
                f"los creditos vencen en {dias_para_vencer} dias y el horizonte es {horizonte}"
            )

    if _campos_cuenta_sin_verificar(evidencia):
        bloquea = True
        acciones.append(
            "cuenta/SCP sin verificar: bloqueados los cambios auxiliares dependientes"
        )

    return {
        **base,
        "nivel": nivel,
        "bloquea_auxiliares": bloquea,
        "requiere_escalado": nivel in ("REVISION_URGENTE", "INCIDENTE_CRITICO", "BLOQUEO_FORECAST"),
        "creditos_usd": str(cuantizar(creditos)),
        "consumo_usd": str(cuantizar(consumo)),
        "restante_usd": str(cuantizar(restante)),
        "forecast_usd": str(cuantizar(forecast)),
        "horizonte_dias": horizonte,
        "dias_para_vencer": dias_para_vencer,
        "creditos_vencidos": vencido,
        "saldo_calculado_desde": restante_declarado,
        "cuenta_sin_verificar": _campos_cuenta_sin_verificar(evidencia),
        "targets_sanos": evidencia.get("targets_sanos"),
        "dos_nodos_ok": _dos_nodos(evidencia.get("targets_sanos")),
        "acciones": acciones,
        "motivo": "Evaluacion determinista sobre evidencia declarada; no es costo observado.",
    }


def _dos_nodos(targets: Any) -> dict[str, Any]:
    if targets is None:
        return {"cumple": False, "motivo": "targets sanos no verificados (TWO_NODES_AWS_NOT_VERIFIED)"}
    return {
        "cumple": int(targets) >= 2,
        "motivo": "2 o mas targets sanos" if int(targets) >= 2 else "menos de 2 targets sanos",
    }


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="verificar_finops_diario.py",
        description="Control FinOps diario local (I-120). No consulta AWS.",
    )
    parser.add_argument("--evidencia", required=True, help="JSON con la evidencia declarada")
    parser.add_argument("--inventario", default=str(INVENTARIO_DEFECTO))
    parser.add_argument("--salida", default=None)
    parser.add_argument("--hoy", default=None, help="Fecha YYYY-MM-DD para pruebas deterministas")
    return parser


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    evidencia = cargar_json(args.evidencia)
    if not isinstance(evidencia, Mapping):
        raise ErrorEvidencia("La evidencia debe ser un objeto JSON.")
    hoy = datetime.strptime(args.hoy, "%Y-%m-%d").date() if args.hoy else None
    resultado = evaluar(evidencia, umbrales_de_inventario(args.inventario), hoy)
    resultado["generado_utc"] = datetime.now(timezone.utc).isoformat(timespec="seconds")
    resultado["aws_consultado"] = False
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
    return (1 if resultado["nivel"] in NIVELES_BLOQUEANTES else 0), resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except ErrorEvidencia as exc:
        print(f"EVIDENCIA_INVALIDA: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
