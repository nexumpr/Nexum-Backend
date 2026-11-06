"""Validación de requisitos de topología exigidos por la spec I-000.

Corrección D: se distinguen dos estados que antes se confundían.

- ``DESIGN_VALIDATED``: el documento de diseño declara dos procesos servidor
  simultáneos en zonas distintas detrás de un balanceador real, red privada sin
  NAT y el esquema TLS acordado. **No acredita nada desplegado.**
- ``AWS_DEPLOYMENT_VERIFIED``: requiere evidencia de runtime obtenida de AWS
  (``describe-instances``) y revisión humana. En esta fase no existe, por lo que
  el resultado es siempre ``NOT_RUN``.

Un campo ``"real": true`` escrito a mano en el JSON es una intención de diseño;
no se acepta como prueba de infraestructura desplegada. Nunca se crean
instancias para comprobarlo.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2]
RUTA_TOPOLOGIA = Path(__file__).with_name("evidencia") / "topologia_c_2026-11-06.json"
DIR_EVIDENCIA = Path(__file__).with_name("evidencia")

ESTADO_DISENO_OK = "DESIGN_VALIDATED"
ESTADO_DISENO_INVALIDO = "DESIGN_INVALID"
ESTADO_DESPLEGUE_OK = "AWS_DEPLOYMENT_VERIFIED"
ESTADO_DESPLEGUE_NO_VERIFICADO = "NOT_RUN"

# Único origen admitido para acreditar runtime; exige dos instancias descritas.
ORIGEN_RUNTIME_ADMITIDO = "aws_ec2_describe_instances"
ESTADO_RUNTIME_ADMITIDO = "VERIFIED_AWS"


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
            violaciones.append(
                f"{etiqueta}: no es proceso servidor real ({instancia.get('tipo')})"
            )
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


def _autocertificacion(topologia: dict) -> list[str]:
    """Impide que el propio JSON de diseño se declare desplegado."""

    problemas: list[str] = []
    estado = topologia.get("estado_verificacion") or {}
    runtime = estado.get("runtime")
    if runtime not in (None, ESTADO_DESPLEGUE_NO_VERIFICADO):
        problemas.append(
            "TOPOLOGIA_NO_PUEDE_AUTOCERTIFICAR_DESPLIEGUE: "
            f"el JSON declara runtime={runtime!r}; sólo aws_ec2_describe_instances puede acreditarlo"
        )
    if estado.get("recursos_creados") not in (False, None):
        problemas.append(
            "TOPOLOGIA_DECLARA_RECURSOS_CREADOS: un documento de diseño no puede afirmarlo"
        )
    return problemas


def verificar_despliegue(dir_evidencia: Path = DIR_EVIDENCIA) -> dict:
    """Busca evidencia de runtime AWS; sin ella el estado es NOT_RUN."""

    if not Path(dir_evidencia).is_dir():
        return {
            "estado": ESTADO_DESPLEGUE_NO_VERIFICADO,
            "motivo": "sin directorio de evidencia",
            "evidencia": None,
        }
    candidatos = sorted(Path(dir_evidencia).glob("despliegue_runtime_*.json"))
    for ruta in candidatos:
        try:
            datos = json.loads(ruta.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            continue
        if (
            datos.get("origen") == ORIGEN_RUNTIME_ADMITIDO
            and datos.get("estado") == ESTADO_RUNTIME_ADMITIDO
            and datos.get("verificado_por_humano") is True
            and isinstance(datos.get("instancias"), list)
            and len(datos["instancias"]) == 2
        ):
            return {
                "estado": ESTADO_DESPLEGUE_OK,
                "motivo": "evidencia de runtime AWS verificada",
                "evidencia": ruta.name,
            }
    return {
        "estado": ESTADO_DESPLEGUE_NO_VERIFICADO,
        "motivo": (
            "no existe evidencia de runtime AWS "
            f"(se exige {ORIGEN_RUNTIME_ADMITIDO} con 2 instancias y revisión humana)"
        ),
        "evidencia": None,
    }


def validar(topologia: dict, dir_evidencia: Path = DIR_EVIDENCIA) -> dict:
    """Agrega las validaciones de diseño y separa el estado de despliegue."""

    dos = validar_dos_instancias(topologia)
    privada = validar_conectividad_privada(topologia)
    tls = validar_tls(topologia)
    autocert = _autocertificacion(topologia)
    diseno_valido = not dos and not privada and not autocert
    despliegue = verificar_despliegue(dir_evidencia)
    return {
        "estado_diseno": ESTADO_DISENO_OK if diseno_valido else ESTADO_DISENO_INVALIDO,
        "diseno_valido": diseno_valido,
        "dos_instancias": dos,
        "conectividad_privada": privada,
        "tls": tls,
        "autocertificacion": autocert,
        "estado_despliegue": despliegue["estado"],
        "motivo_despliegue": despliegue["motivo"],
        "evidencia_despliegue": despliegue["evidencia"],
        "verificacion_runtime": ESTADO_DESPLEGUE_NO_VERIFICADO
        if despliegue["estado"] != ESTADO_DESPLEGUE_OK
        else ESTADO_DESPLEGUE_OK,
        "nota": (
            "Las comprobaciones de este módulo validan el DOCUMENTO DE DISENO; "
            "un campo real=true no acredita instancias, balanceo ni trafico"
        ),
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Valida requisitos de topología I-000")
    parser.add_argument("--topologia", type=Path, default=RUTA_TOPOLOGIA)
    parser.add_argument("--evidencia", type=Path, default=DIR_EVIDENCIA)
    args = parser.parse_args(argv)
    topologia = json.loads(args.topologia.read_text(encoding="utf-8"))
    resultado = validar(topologia, args.evidencia)
    print(json.dumps(resultado, indent=2, ensure_ascii=False, sort_keys=True))
    return 0 if resultado["diseno_valido"] else 1


if __name__ == "__main__":
    sys.exit(main())
