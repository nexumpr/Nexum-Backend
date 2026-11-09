"""Integridad financiera del dataset restaurado (I-130, INF-130-04).

Valida, sobre datos **sinteticos** y sin AWS:

- dinero en centavos enteros (nunca float), R-030;
- movimientos e idempotencia sin duplicados;
- aportaciones divididas: ``saved = opening + sum(allocations)`` (A-070);
- centavos no distribuidos declarados y coherentes;
- reversiones que netean cero y referencias consistentes;
- aislamiento por usuario;
- sesiones revocadas y cuentas eliminadas que una restauracion antigua no debe
  reactivar;
- configuracion que un restore **no** recupera por si solo (TTL, streams, PITR,
  IAM, alarmas, etiquetas, escalamiento).

Codigos: 0 valido, 1 violaciones, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import json
import sys
from decimal import Decimal
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
DATASET_DEFECTO = RAIZ / "tests" / "fixtures" / "respaldo" / "dataset_sintetico.json"

CODIGOS = (
    "MONEY_FLOAT",
    "MONEY_BOOL",
    "MONEY_NEGATIVO",
    "MOVIMIENTO_DUPLICADO",
    "IDEMPOTENCIA_DUPLICADA",
    "REFERENCIA_ROTA",
    "AISLAMIENTO_USUARIO",
    "SUMA_ASIGNACIONES",
    "CENTAVOS_NO_DISTRIBUIDOS",
    "REVERSION_INCONSISTENTE",
    "SESION_REVOCADA_REACTIVADA",
    "CUENTA_ELIMINADA_REACTIVADA",
    "TTL_AUSENTE",
)

CONFIG_REQUERIDA = (
    "ttl_atributo",
    "streams_habilitado",
    "pitr_habilitado",
    "iam_politicas",
    "alarmas",
    "etiquetas",
    "escalamiento",
)


class ErrorUso(Exception):
    """Dataset o solicitud invalidos (codigo 2)."""


def cargar_json(ruta: Path | str) -> Any:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorUso(f"No se puede leer {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorUso(f"{ruta} no es JSON valido: {exc}") from exc


def es_entero(valor: Any) -> bool:
    return isinstance(valor, int) and not isinstance(valor, bool)


def _violacion(codigo: str, detalle: str) -> dict[str, str]:
    if codigo not in CODIGOS:  # pragma: no cover - defensivo
        raise ErrorUso(f"Codigo de violacion desconocido: {codigo}")
    return {"codigo": codigo, "detalle": detalle}


def _dinero(nombre: str, valor: Any, violaciones: list[dict[str, str]]) -> int:
    if isinstance(valor, float):
        violaciones.append(_violacion("MONEY_FLOAT", f"{nombre} es float: {valor!r}"))
        return 0
    if isinstance(valor, bool):
        violaciones.append(_violacion("MONEY_BOOL", f"{nombre} es booleano: {valor!r}"))
        return 0
    if not es_entero(valor):
        violaciones.append(_violacion("MONEY_FLOAT", f"{nombre} no es entero: {valor!r}"))
        return 0
    return valor


def validar_dataset(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    """Devuelve violaciones y el resumen de invariantes comprobadas."""
    if datos is None:
        raise ErrorUso("Falta el dataset sintetico a validar.")
    if not isinstance(datos, Mapping):
        raise ErrorUso("El dataset debe ser un objeto JSON.")

    usuarios = {u["id"]: u for u in datos.get("usuarios", [])}
    metas = {m["id"]: m for m in datos.get("metas", [])}
    movimientos = datos.get("movimientos", [])
    sesiones = datos.get("sesiones", [])
    revocadas = {r["sesion_id"] for r in datos.get("revocaciones", [])}
    eliminadas = {c["user_id"] for c in datos.get("cuentas_eliminadas", [])}

    violaciones: list[dict[str, str]] = []
    vistos: set[str] = set()
    importes: dict[str, int] = {}
    idempotencia: set[str] = set()
    neto_reversiones = Decimal("0")
    total_asignado = Decimal("0")

    for movimiento in movimientos:
        mid = str(movimiento.get("id"))
        if mid in vistos:
            violaciones.append(_violacion("MOVIMIENTO_DUPLICADO", f"id repetido: {mid}"))
        vistos.add(mid)

        clave = movimiento.get("idempotency_key")
        if clave:
            if clave in idempotencia:
                violaciones.append(
                    _violacion("IDEMPOTENCIA_DUPLICADA", f"idempotency_key repetida: {clave}")
                )
            idempotencia.add(clave)

        importe = _dinero(f"movimiento {mid}.importe_centavos", movimiento.get("importe_centavos"), violaciones)
        if importe < 0 and movimiento.get("tipo") != "reversion":
            violaciones.append(
                _violacion("MONEY_NEGATIVO", f"movimiento {mid} negativo sin ser reversion")
            )
        if movimiento.get("tipo") == "reversion":
            original = movimiento.get("original_id")
            if not original or original not in importes:
                violaciones.append(
                    _violacion("REVERSION_INCONSISTENTE", f"reversion {mid} sin original valido")
                )
            elif importe != -importes[original]:
                violaciones.append(
                    _violacion(
                        "REVERSION_INCONSISTENTE",
                        f"reversion {mid} ({importe}) no netea el original {original} ({importes[original]})",
                    )
                )
            neto_reversiones += Decimal(importe)

        importes[mid] = importe

        if not movimiento.get("expiresAt"):
            violaciones.append(_violacion("TTL_AUSENTE", f"movimiento {mid} sin expiresAt"))

        meta_id = movimiento.get("meta_id")
        if meta_id:
            meta = metas.get(meta_id)
            if meta is None:
                violaciones.append(_violacion("REFERENCIA_ROTA", f"movimiento {mid} -> meta {meta_id}"))
            elif meta.get("user_id") != movimiento.get("user_id"):
                violaciones.append(
                    _violacion(
                        "AISLAMIENTO_USUARIO",
                        f"movimiento {mid} de {movimiento.get('user_id')} referencia meta de {meta.get('user_id')}",
                    )
                )

    for meta in metas.values():
        guardado = _dinero(f"meta {meta['id']}.saved_centavos", meta.get("saved_centavos"), violaciones)
        apertura = _dinero(
            f"meta {meta['id']}.opening_saved_centavos", meta.get("opening_saved_centavos"), violaciones
        )
        asignado = Decimal("0")
        for asignacion in meta.get("allocations", []) or []:
            valor = _dinero(
                f"meta {meta['id']}.allocation.{asignacion.get('id')}",
                asignacion.get("importe_centavos"),
                violaciones,
            )
            if asignacion.get("user_id") and asignacion.get("user_id") != meta.get("user_id"):
                violaciones.append(
                    _violacion(
                        "AISLAMIENTO_USUARIO",
                        f"asignacion {asignacion.get('id')} de otro usuario en meta {meta['id']}",
                    )
                )
            asignado += Decimal(valor)
        total_asignado += asignado
        esperado = Decimal(guardado - apertura)
        no_distribuido = Decimal(str(meta.get("no_distribuido_centavos", 0)))
        if asignado + no_distribuido != esperado:
            violaciones.append(
                _violacion(
                    "SUMA_ASIGNACIONES",
                    f"meta {meta['id']}: asignado {asignado} + no distribuido {no_distribuido} != {esperado}",
                )
            )
        elif no_distribuido != 0 and not meta.get("motivo_no_distribuido"):
            violaciones.append(
                _violacion(
                    "CENTAVOS_NO_DISTRIBUIDOS",
                    f"meta {meta['id']} declara {no_distribuido} centavos sin motivo",
                )
            )

    for sesion in sesiones:
        if sesion.get("id") in revocadas and sesion.get("estado") != "revocada":
            violaciones.append(
                _violacion(
                    "SESION_REVOCADA_REACTIVADA",
                    f"sesion {sesion.get('id')} revocada aparece como {sesion.get('estado')}",
                )
            )

    for usuario_id in eliminadas:
        usuario = usuarios.get(usuario_id)
        if usuario is not None and usuario.get("estado") != "eliminado":
            violaciones.append(
                _violacion(
                    "CUENTA_ELIMINADA_REACTIVADA",
                    f"cuenta {usuario_id} eliminada aparece como {usuario.get('estado')}",
                )
            )

    return {
        "estado": "INTEGRIDAD_OK" if not violaciones else "INTEGRIDAD_ROTA",
        "violaciones": violaciones,
        "codigos": sorted({v["codigo"] for v in violaciones}),
        "resumen": {
            "usuarios": len(usuarios),
            "metas": len(metas),
            "movimientos": len(movimientos),
            "sesiones": len(sesiones),
            "revocaciones": len(revocadas),
            "cuentas_eliminadas": len(eliminadas),
            "total_asignado_centavos": str(total_asignado),
            "neto_reversiones_centavos": str(neto_reversiones),
        },
        "alcance": "DATOS_SINTETICOS",
        "advertencia": "Validacion aritmetica local; no acredita un restore real en AWS.",
    }


def verificar_config_post_restore(config: Mapping[str, Any] | None) -> dict[str, Any]:
    """Un restore de datos no recupera TTL, streams, PITR, IAM, alarmas ni tags."""
    if config is None:
        raise ErrorUso("Se requiere --config con la configuracion de la tabla restaurada.")
    faltantes = [campo for campo in CONFIG_REQUERIDA if not config.get(campo)]
    return {
        "estado": "RECONFIGURACION_PENDIENTE" if faltantes else "CONFIGURACION_COMPLETA",
        "faltantes": faltantes,
        "requeridos": list(CONFIG_REQUERIDA),
        "advertencia": (
            "Restaurar datos no restaura la infraestructura: TTL, streams, PITR, IAM, "
            "alarmas y etiquetas deben reconfigurarse y verificarse."
        ),
    }


def resumen_reactivacion(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    """Comprueba que una restauracion no reactive cuentas ni sesiones."""
    if datos is None:
        raise ErrorUso("Falta el dataset sintetico.")
    eliminadas = {c["user_id"] for c in datos.get("cuentas_eliminadas", [])}
    revocadas = {r["sesion_id"] for r in datos.get("revocaciones", [])}
    usuarios = {u["id"]: u for u in datos.get("usuarios", [])}
    sesiones = {s["id"]: s for s in datos.get("sesiones", [])}
    reactivadas = sorted(
        uid for uid in eliminadas if usuarios.get(uid, {}).get("estado") != "eliminado"
    )
    sesiones_activas = sorted(
        sid for sid in revocadas if sesiones.get(sid, {}).get("estado") != "revocada"
    )
    return {
        "estado": "SIN_REACTIVACION" if not reactivadas and not sesiones_activas else "REACTIVACION_DETECTADA",
        "cuentas_reactivadas": reactivadas,
        "sesiones_reactivadas": sesiones_activas,
        "alcance": "DATOS_SINTETICOS",
    }


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="verificar_integridad_financiera.py",
        description="Integridad financiera del dataset restaurado (I-130). Sin AWS.",
    )
    parser.add_argument("comando", help="validar | config-post-restore | reactivacion")
    parser.add_argument("--datos", default=str(DATASET_DEFECTO))
    parser.add_argument("--config", default=None)
    parser.add_argument("--salida", default=None)
    return parser


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    comando = args.comando.strip().lower()

    if comando == "validar":
        resultado = validar_dataset(cargar_json(args.datos))
    elif comando == "config-post-restore":
        resultado = verificar_config_post_restore(cargar_json(args.config) if args.config else None)
    elif comando == "reactivacion":
        resultado = resumen_reactivacion(cargar_json(args.datos))
    else:
        raise ErrorUso(
            f"Comando desconocido: {comando!r}. Solo existen: validar, config-post-restore, reactivacion."
        )

    resultado["comando"] = comando
    resultado["aws_consultado"] = False
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )

    con_violaciones = bool(resultado.get("violaciones")) or resultado.get("estado") in (
        "INTEGRIDAD_ROTA",
        "RECONFIGURACION_PENDIENTE",
        "REACTIVACION_DETECTADA",
    )
    return (1 if con_violaciones else 0), resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except ErrorUso as exc:
        print(f"DENEGADO: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
