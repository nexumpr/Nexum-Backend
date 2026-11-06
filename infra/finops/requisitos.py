"""Validación de requisitos de topología exigidos por la spec I-000.

Comprueba dos invariantes que no admiten simulación (constitution R-036/R-037):
- dos instancias de servidor reales, simultáneas, en nodos/zonas distintas,
  detrás de un balanceador real;
- conectividad privada sin NAT (gateway endpoints para S3 y DynamoDB).
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2]
RUTA_TOPOLOGIA = Path(__file__).with_name("evidencia") / "topologia_c_2026-11-06.json"


def validar_dos_instancias(topologia: dict) -> list[str]:
    """Devuelve violaciones del requisito de dos instancias simultáneas reales."""

    violaciones: list[str] = []
    instancias = topologia.get("instancias")
    if not isinstance(instancias, list):
        return ["INSTANCIAS_AUSENTES"]
    if len(instancias) != 2:
        violaciones.append(f"INSTANCIAS={len(instancias)}; se exigen exactamente 2")
    zonas = set()
    for indice, instancia in enumerate(instancias):
        etiqueta = instancia.get("id", f"#{indice}")
        if instancia.get("tipo") != "proceso_servidor":
            violaciones.append(f"{etiqueta}: no es proceso servidor real ({instancia.get('tipo')})")
        if instancia.get("simulado") is not False:
            violaciones.append(f"{etiqueta}: simulado no declarado como False")
        if instancia.get("misma_maquina") is not False:
            violaciones.append(f"{etiqueta}: misma_maquina no declarado como False")
        if instancia.get("activa_simultanea") is not True:
            violaciones.append(f"{etiqueta}: no declarada activa simultánea")
        if instancia.get("detras_de_balanceador") is not True:
            violaciones.append(f"{etiqueta}: no está detrás del balanceador")
        zona = instancia.get("az")
        if not zona:
            violaciones.append(f"{etiqueta}: sin zona de disponibilidad")
        else:
            zonas.add(zona)
    if len(instancias) == 2 and len(zonas) < 2:
        violaciones.append("LAS_DOS_INSTANCIAS_COMPARTEN_AZ")
    balanceador = topologia.get("balanceador") or {}
    if balanceador.get("tipo") not in ("alb", "cloudfront+alb"):
        violaciones.append("BALANCEADOR_NO_REAL")
    if balanceador.get("real") is not True:
        violaciones.append("BALANCEADOR_NO_DECLARADO_REAL")
    return violaciones


def validar_conectividad_privada(topologia: dict) -> list[str]:
    """Comprueba el diseño sin NAT y con gateway endpoints para S3 y DynamoDB."""

    violaciones: list[str] = []
    red = topologia.get("red") or {}
    if red.get("nat_gateways", 0) != 0:
        violaciones.append("NAT_PRESENTE: el diseño debe evitar NAT")
    endpoints = red.get("gateway_endpoints") or []
    requeridos = {"s3", "dynamodb"}
    faltantes = requeridos - {str(e).lower() for e in endpoints}
    if faltantes:
        violaciones.append(f"GATEWAY_ENDPOINTS_FALTANTES: {sorted(faltantes)}")
    if red.get("interface_endpoints", 0) not in (0, None):
        violaciones.append("INTERFACE_ENDPOINTS_PRESENTES: revisar costo por hora")
    if red.get("ipv4_publicas", 0) != 0:
        violaciones.append("IPV4_PUBLICAS: el diseño C no debe asignar IPv4 pública")
    if red.get("egress_restringido") is not True:
        violaciones.append("EGRESS_NO_RESTRINGIDO")
    return violaciones


def validar_tls(topologia: dict) -> list[str]:
    """HTTPS de navegador a CloudFront y advertencia de tramo interno."""

    violaciones: list[str] = []
    tls = topologia.get("tls") or {}
    if tls.get("navegador_a_cloudfront") != "https_tls12+":
        violaciones.append("TLS_NAVEGADOR_INSUFICIENTE")
    if tls.get("cloudfront_a_origen") != "https" and tls.get("origen_extremo_a_extremo") is not False:
        violaciones.append("TLS_ORIGEN_NO_ACEPTADO_FORMALMENTE")
    return violaciones


def validar(topologia: dict) -> dict:
    """Agrega las validaciones de topología en un resultado único."""

    dos = validar_dos_instancias(topologia)
    privada = validar_conectividad_privada(topologia)
    tls = validar_tls(topologia)
    diseno_valido = not dos and not privada
    return {
        "diseno_valido": diseno_valido,
        "dos_instancias": dos,
        "conectividad_privada": privada,
        "tls": tls,
        "verificacion_runtime": "NOT_RUN",
        "nota": "Estas comprobaciones validan el DISEÑO; no acreditan instancias ni tráfico reales",
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Valida requisitos de topología I-000")
    parser.add_argument("--topologia", type=Path, default=RUTA_TOPOLOGIA)
    args = parser.parse_args(argv)
    topologia = json.loads(args.topologia.read_text(encoding="utf-8"))
    resultado = validar(topologia)
    print(json.dumps(resultado, indent=2, ensure_ascii=False, sort_keys=True))
    return 0 if resultado["diseno_valido"] else 1


if __name__ == "__main__":
    sys.exit(main())
