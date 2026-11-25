"""FinOps determinista del egreso FCM/OAuth por NAT Gateway 1 AZ.

Qué es exacto y qué no:

- La aritmética es ``Decimal``; nunca se usa ``float``.
- Los precios de NAT Gateway (hora, procesamiento y salida a Internet) NO están
  verificados en el catálogo: se modelan con SKUs ``SUPUESTO`` y se marcan
  ``NOT_VERIFIED``. El SKU ``NAT_GATEWAY_HORA`` original sigue ``UNKNOWN``.
- La IPv4 pública del NAT usa ``IPV4_HORA`` (``VERIFIED_AWS``) pero se modela
  como condicional: la facturación de la EIP asociada al NAT no se revalidó en
  esta sesión. Se presentan ambos escenarios (con y sin ese cargo).
- La transferencia inter-AZ usa ``TRANSFERENCIA_INTERAZ_GB`` (``VERIFIED_AWS``)
  y solo aplica si el NAT y la EC2 están en AZ distintas.
- Los créditos son saldo, no presupuesto ni autorización. El límite de 20 USD
  es una propuesta de autorización para la certificación incremental de 8 días.

El tráfico se define con supuestos explícitos y se puede recalcular:
2 000 push/día x 1 KB + 1 000 OAuth/día x 2 KB; escenario alto = 10x.
"""

from __future__ import annotations

import argparse
import json
from decimal import Decimal
from pathlib import Path

import dinero
import modelo_costos

RAIZ = Path(__file__).resolve().parents[2]
RUTA_PRECIOS = Path(__file__).with_name("precios_us-east-2.json")

DIAS_CERTIFICACION = 8
DIAS_CONVENCION = 45
LIMITE_CERTIFICACION_USD = Decimal("20.00")

SUPUESTOS = {
    "push_por_dia": 2000,
    "bytes_por_push": 1024,
    "oauth_por_dia": 1000,
    "bytes_por_oauth": 2048,
    "factor_alto": 10,
}

# Referencia de infraestructura ya existente, documentada en I-000 y en la
# ampliación del endpoint SES. No es factura observada y no se usa para
# autorizar nada.
INFRA_EXISTENTE_I000_45D = Decimal("102.7830")
SES_ENDPOINT_2AZ_45D = Decimal("21.6000")  # 0.02 USD/h x 1080 h
INFRA_EXISTENTE_REFERENCIA_45D = dinero.sumar(
    [INFRA_EXISTENTE_I000_45D, SES_ENDPOINT_2AZ_45D]
)

CREDITOS_VERIFICADOS_2026_11_24 = Decimal("130.491975")

ESTADOS_VERIFICADOS = {"VERIFIED_AWS"}


def cargar_catalogo(ruta: Path | None = None) -> dict:
    """Carga el catálogo validado por el motor I-000."""

    return modelo_costos.cargar_precios(ruta or RUTA_PRECIOS)


def _item(catalogo: dict, sku: str) -> dict:
    items = catalogo["items"]
    if sku not in items:
        raise ValueError(f"SKU ausente del catalogo: {sku}")
    return items[sku]


def _precio(catalogo: dict, sku: str) -> Decimal | None:
    item = _item(catalogo, sku)
    if item.get("precio_usd") is None:
        return None
    return dinero.dec(item["precio_usd"])


def _estado(catalogo: dict, sku: str) -> str:
    return _item(catalogo, sku)["estado"]


def trafico_gb(dias: int, factor: int = 1) -> Decimal:
    """GB de ida y vuelta FCM/OAuth según los supuestos declarados."""

    if dias <= 0:
        raise ValueError("dias debe ser positivo")
    if factor <= 0:
        raise ValueError("factor debe ser positivo")
    bytes_dia = (
        SUPUESTOS["push_por_dia"] * SUPUESTOS["bytes_por_push"]
        + SUPUESTOS["oauth_por_dia"] * SUPUESTOS["bytes_por_oauth"]
    )
    return (
        Decimal(bytes_dia) * Decimal(dias) * Decimal(factor)
    ) / Decimal(1024**3)


def _partida(
    catalogo: dict,
    clave: str,
    concepto: str,
    cantidad: Decimal,
    unidad: str,
    sku: str,
    formula: str,
    nota: str = "",
    condicional: bool = False,
) -> dict:
    precio = _precio(catalogo, sku)
    estado = _estado(catalogo, sku)
    subtotal = None if precio is None else dinero.multiplicar(cantidad, precio)
    return {
        "clave": clave,
        "concepto": concepto,
        "cantidad": cantidad,
        "unidad": unidad,
        "sku": sku,
        "precio_unitario_usd": precio,
        "estado_evidencia": estado,
        "subtotal_usd": subtotal,
        "verificado": estado in ESTADOS_VERIFICADOS,
        "condicional": condicional,
        "formula": formula,
        "nota": nota,
    }


def calcular_escenario(
    catalogo: dict,
    dias: int,
    factor: int = 1,
    incluir_eip: bool = True,
    incluir_interaz: bool = True,
) -> dict:
    """Calcula un escenario (base o alto) con partidas y totales."""

    horas = Decimal(dias) * Decimal(24)
    trafico = trafico_gb(dias, factor)

    partidas = [
        _partida(
            catalogo,
            "NAT_HORA",
            "NAT Gateway (1 AZ)",
            horas,
            "gateway-hora",
            "NAT_GATEWAY_HORA_SUPUESTO",
            f"{horas} h x {_precio(catalogo, 'NAT_GATEWAY_HORA_SUPUESTO')} USD/h",
            "Precio de lista publica usado solo como SUPUESTO; NAT_GATEWAY_HORA sigue UNKNOWN.",
        ),
        _partida(
            catalogo,
            "EIP_NAT",
            "IPv4 publica del NAT (EIP)",
            horas if incluir_eip else Decimal("0"),
            "IP-hora",
            "IPV4_HORA",
            f"{horas if incluir_eip else Decimal('0')} h x {_precio(catalogo, 'IPV4_HORA')} USD/h",
            "Condicional: la facturacion de la EIP asociada al NAT no se revalido. Se presenta con y sin cargo.",
            condicional=True,
        ),
        _partida(
            catalogo,
            "NAT_PROCESAMIENTO",
            "Procesamiento de datos del NAT",
            trafico,
            "GB-procesado",
            "NAT_GATEWAY_PROCESAMIENTO_GB_SUPUESTO",
            f"{dinero.formatear(trafico, 9)} GB x {_precio(catalogo, 'NAT_GATEWAY_PROCESAMIENTO_GB_SUPUESTO')} USD/GB",
            "Precio de lista publica usado solo como SUPUESTO; no verificado.",
        ),
        _partida(
            catalogo,
            "INTERAZ",
            "Transferencia inter-AZ (si el NAT y la EC2 estan en AZ distintas)",
            trafico if incluir_interaz else Decimal("0"),
            "GB",
            "TRANSFERENCIA_INTERAZ_GB",
            f"{dinero.formatear(trafico if incluir_interaz else Decimal('0'), 9)} GB x {_precio(catalogo, 'TRANSFERENCIA_INTERAZ_GB')} USD/GB",
            "Precio regional verificado; hoy ambas EC2 estan en us-east-2a, por lo que el escenario base sin inter-AZ tambien se muestra.",
            condicional=True,
        ),
        _partida(
            catalogo,
            "EGRESO_INTERNET",
            "Salida a Internet por el NAT",
            trafico,
            "GB-salida",
            "NAT_EGRESO_INTERNET_GB_SUPUESTO",
            f"{dinero.formatear(trafico, 9)} GB x {_precio(catalogo, 'NAT_EGRESO_INTERNET_GB_SUPUESTO')} USD/GB",
            "Precio de lista publicada usado solo como SUPUESTO; no se aplica franquicia de 100 GB/mes.",
        ),
    ]

    con_precio = [p for p in partidas if p["subtotal_usd"] is not None]
    sin_precio = [p["clave"] for p in partidas if p["subtotal_usd"] is None]
    total_modelado = dinero.sumar([p["subtotal_usd"] for p in con_precio])
    total_verificado = dinero.sumar(
        [p["subtotal_usd"] for p in con_precio if p["verificado"]]
    )
    total_supuesto = dinero.sumar(
        [p["subtotal_usd"] for p in con_precio if not p["verificado"]]
    )

    return {
        "dias": dias,
        "horas": horas,
        "factor_trafico": factor,
        "trafico_gb": trafico,
        "partidas": partidas,
        "sin_precio": sin_precio,
        "total_modelado_usd": total_modelado,
        "subtotal_verificado_usd": total_verificado,
        "subtotal_supuesto_usd": total_supuesto,
        "estado": (
            "MODELADO_CON_SUPUESTOS_NO_VERIFICADOS"
            if total_supuesto > 0 or sin_precio
            else "TODAS_LAS_PARTIDAS_VERIFICADAS"
        ),
    }


def calcular_tabla(
    catalogo: dict | None = None,
    creditos_verificados_usd: Decimal | str | None = None,
) -> dict:
    """Tabla completa de 8/45 días, base/alto, sensibilidades y margen."""

    catalogo = catalogo or cargar_catalogo()
    base_8 = calcular_escenario(catalogo, DIAS_CERTIFICACION, 1, True, True)
    base_45 = calcular_escenario(catalogo, DIAS_CONVENCION, 1, True, True)
    alto_8 = calcular_escenario(catalogo, DIAS_CERTIFICACION, SUPUESTOS["factor_alto"], True, True)
    alto_45 = calcular_escenario(catalogo, DIAS_CONVENCION, SUPUESTOS["factor_alto"], True, True)
    sin_eip_8 = calcular_escenario(catalogo, DIAS_CERTIFICACION, 1, False, True)
    sin_interaz_8 = calcular_escenario(catalogo, DIAS_CERTIFICACION, 1, True, False)

    infra_8_prorrateo = dinero.cuantizar(
        INFRA_EXISTENTE_REFERENCIA_45D * Decimal(DIAS_CERTIFICACION) / Decimal(DIAS_CONVENCION)
    )
    total_entorno_8 = dinero.sumar(
        [infra_8_prorrateo, base_8["total_modelado_usd"]]
    )
    total_entorno_45 = dinero.sumar(
        [INFRA_EXISTENTE_REFERENCIA_45D, base_45["total_modelado_usd"]]
    )
    margen_8 = dinero.cuantizar(total_entorno_8 * Decimal("1.10"))
    margen_45 = dinero.cuantizar(total_entorno_45 * Decimal("1.10"))

    creditos = None
    if creditos_verificados_usd is not None:
        creditos = dinero.dec(creditos_verificados_usd)

    return {
        "supuestos": dict(SUPUESTOS),
        "limite_certificacion_usd": LIMITE_CERTIFICACION_USD,
        "escenarios": {
            "base_8d": base_8,
            "base_45d": base_45,
            "alto_8d": alto_8,
            "alto_45d": alto_45,
            "base_8d_sin_eip": sin_eip_8,
            "base_8d_sin_interaz": sin_interaz_8,
        },
        "infra_existente": {
            "i000_45d_usd": INFRA_EXISTENTE_I000_45D,
            "ses_endpoint_2az_45d_usd": SES_ENDPOINT_2AZ_45D,
            "referencia_45d_usd": INFRA_EXISTENTE_REFERENCIA_45D,
            "prorrateo_8d_usd": infra_8_prorrateo,
            "nota": "Referencia documental, no factura observada; excluye I-110 y consumo variable.",
        },
        "total_entorno_8d_usd": total_entorno_8,
        "total_entorno_45d_usd": total_entorno_45,
        "margen_10_8d_usd": margen_8,
        "margen_10_45d_usd": margen_45,
        "limite_20_8d_incremental_cumplido": (
            base_8["total_modelado_usd"] <= LIMITE_CERTIFICACION_USD
        ),
        "creditos_verificados_usd": creditos,
        "creditos_no_son_autorizacion": True,
        "total_45d_cubierto_por_creditos": (
            None if creditos is None else total_entorno_45 <= creditos
        ),
        "total_8d_cubierto_por_creditos": (
            None if creditos is None else total_entorno_8 <= creditos
        ),
        "estado_global": "MODELADO_CON_SUPUESTOS_NO_VERIFICADOS; PRECIOS NAT NOT_VERIFIED; CREDITOS NO SON AUTORIZACION",
    }


def _a_texto(valor):
    if isinstance(valor, Decimal):
        return dinero.formatear(valor, 6)
    if isinstance(valor, dict):
        return {k: _a_texto(v) for k, v in valor.items()}
    if isinstance(valor, list):
        return [_a_texto(v) for v in valor]
    return valor


def tabla_texto(tabla: dict) -> str:
    """Representación JSON con importes como cadenas (6 decimales)."""

    return json.dumps(_a_texto(tabla), ensure_ascii=False, indent=2, sort_keys=True)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--creditos-verificados",
        default=None,
        help="Saldo verificado en USD; se muestra como contexto, nunca como autorizacion.",
    )
    args = parser.parse_args(argv)
    tabla = calcular_tabla(creditos_verificados_usd=args.creditos_verificados)
    print(tabla_texto(tabla))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
