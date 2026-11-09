"""Verificador local de respaldo y restauracion de Nexum-Backend (spec I-130).

Reglas:

- **Nunca** consulta AWS ni ejecuta Terraform: produce planes reproducibles.
- La restauracion solo puede ir a una **tabla nueva aislada**; sobreescribir la
  tabla activa es imposible por construccion y se rechaza.
- Destruir datos o borrar copias fuera de retencion no existe como capacidad.
- El dinero se calcula con ``Decimal``; las cifras no medidas se declaran como
  tales, nunca como cero.
- ``RESTORE_AWS_NO_VERIFICADO`` y ``OBJETIVO_PROPUESTO_NO_MEDIDO`` son estados
  explicitos: un plan local no acredita un restore real.

Codigos: 0 correcto, 1 bloqueado, 2 uso invalido o prohibido.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from datetime import datetime, timezone
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
INVENTARIO_DEFECTO = RAIZ / "infra" / "terraform" / "respaldo" / "inventario_backup.json"
CATALOGO_DEFECTO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"

CUATRO_DECIMALES = Decimal("0.0001")
HORAS_MES = Decimal("730")
HORAS_45_DIAS = Decimal("1080")

BASES_VALIDAS = ("hora", "mes", "existencia", "variable")
ESTADOS_PROTEGIDOS = ("DECLARADO_NO_CREADO", "OPCIONAL_NO_CREADO", "BLOQUEADO_RFC004")

RESTORE_AWS = "RESTORE_AWS_NO_VERIFICADO"
RTO_RPO_NO_MEDIDO = "OBJETIVO_PROPUESTO_NO_MEDIDO"

# En I-130 LOCAL ninguna evidencia controlada por el agente puede acreditar AWS.
# El contrato de verificacion autenticada queda definido pero DESHABILITADO.
VERIFICACION_AWS_HABILITADA = False
GATES_AUTENTICADOS: tuple[str, ...] = ()


def evaluar_evidencia_aws(evidencia: Any) -> dict[str, Any]:
    """Valida forma y estructura; nunca acredita veracidad en I-130 LOCAL."""
    if evidencia is None:
        return {"presente": False, "bien_formada": False, "acreditada": False, "motivo": "sin evidencia"}
    if not isinstance(evidencia, Mapping):
        return {
            "presente": True,
            "bien_formada": False,
            "acreditada": False,
            "motivo": "evidencia malformada: se esperaba un objeto",
        }
    huella = str(evidencia.get("huella_sha256", ""))
    campos_ok = (
        evidencia.get("verificado_por_aws") is True
        and isinstance(evidencia.get("gate"), str)
        and bool(evidencia.get("gate"))
        and len(huella) == 64
        and all(c in "0123456789abcdef" for c in huella.lower())
    )
    acreditada = bool(
        campos_ok
        and VERIFICACION_AWS_HABILITADA
        and evidencia.get("gate") in GATES_AUTENTICADOS
    )
    return {
        "presente": True,
        "bien_formada": campos_ok,
        "acreditada": acreditada,
        "motivo": (
            "verificacion AWS deshabilitada en I-130 LOCAL: un JSON local no acredita la nube"
            if campos_ok
            else "evidencia incompleta o malformada (bandera, gate o huella)"
        ),
    }

COMANDOS_PROHIBIDOS = frozenset(
    {
        "destroy-data",
        "destruir-datos",
        "destroy-state",
        "destruir-estado",
        "destroy-all",
        "destruir-todo",
        "restaurar-sobre-produccion",
        "restore-in-place",
        "drop-table",
    }
)


class ErrorUso(Exception):
    """Uso invalido o comando prohibido (codigo 2)."""


class ErrorInventario(Exception):
    """Inventario de respaldo malformado (codigo 2)."""


def cuantizar(valor: Decimal) -> Decimal:
    return valor.quantize(CUATRO_DECIMALES, rounding=ROUND_HALF_UP)


def cargar_json(ruta: Path | str) -> Any:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorInventario(f"No se puede leer {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorInventario(f"{ruta} no es JSON valido: {exc}") from exc


def cargar_inventario(ruta: Path | str = INVENTARIO_DEFECTO) -> dict[str, Any]:
    datos = cargar_json(ruta)
    if not isinstance(datos, dict) or not isinstance(datos.get("activos"), list):
        raise ErrorInventario("El inventario de respaldo debe exponer 'activos'.")
    if not datos["activos"]:
        raise ErrorInventario(
            "Inventario vacio: sin activos no se puede afirmar que hay respaldo."
        )
    return datos


def cargar_catalogo(ruta: Path | str = CATALOGO_DEFECTO) -> dict[str, Any]:
    datos = cargar_json(ruta)
    if not isinstance(datos, dict) or not isinstance(datos.get("items"), dict):
        raise ErrorInventario("El catalogo de precios debe exponer 'items'.")
    return datos


def precio(catalogo: Mapping[str, Any], sku: str) -> Decimal:
    items = catalogo["items"]
    if sku not in items:
        raise ErrorInventario(f"SKU desconocido: {sku}")
    valor = items[sku].get("precio_usd")
    if valor is None:
        raise ErrorInventario(f"SKU sin precio verificado: {sku}; no se infiere.")
    return Decimal(str(valor))


def _cantidad(valor: Any, campo: str) -> Decimal:
    if valor is None:
        raise ErrorInventario(f"Cantidad nula en '{campo}'.")
    numero = Decimal(str(valor))
    if numero < 0:
        raise ErrorInventario(f"Cantidad negativa en '{campo}'.")
    return numero


def coste_horario(activo: Mapping[str, Any], catalogo: Mapping[str, Any]) -> Decimal:
    total = Decimal("0")
    for partida in activo.get("partidas", []) or []:
        base = partida.get("base")
        if base not in BASES_VALIDAS:
            raise ErrorInventario(f"Base invalida en '{activo.get('nombre')}': {base!r}")
        if base in ("existencia", "variable") or partida.get("cantidad") is None:
            continue
        importe = precio(catalogo, partida["sku"]) * _cantidad(
            partida["cantidad"], f"{activo['nombre']}.{partida['sku']}"
        )
        total += importe if base == "hora" else importe / HORAS_MES
    return total


def coste_respaldo(inventario: Mapping[str, Any], catalogo: Mapping[str, Any]) -> dict[str, Any]:
    residente = Decimal("0")
    presupuestado = Decimal("0")
    no_computable: dict[str, str] = {}
    detalle: dict[str, str] = {}
    for activo in inventario["activos"]:
        estado = activo.get("estado")
        if estado == "BLOQUEADO_RFC004":
            no_computable[activo["nombre"]] = estado
            continue
        if estado not in ESTADOS_PROTEGIDOS:
            no_computable[activo["nombre"]] = f"estado no reconocido: {estado}"
            continue
        valor = coste_horario(activo, catalogo)
        detalle[activo["nombre"]] = str(cuantizar(valor))
        if estado == "OPCIONAL_NO_CREADO":
            presupuestado += valor
        else:
            residente += valor
    total = residente + presupuestado
    return {
        "por_activo_hora_usd": {k: detalle[k] for k in sorted(detalle)},
        "residente_hora_usd": str(cuantizar(residente)),
        "opcional_hora_usd": str(cuantizar(presupuestado)),
        "total_hora_usd": str(cuantizar(total)),
        "total_45d_usd": str(cuantizar(total * HORAS_45_DIAS)),
        "no_computable": {k: no_computable[k] for k in sorted(no_computable)},
        "moneda": "USD",
        "horas_mes": str(HORAS_MES),
        "costo_observado": "NOT_VERIFIED",
        "advertencia": (
            "Coste fijo de existencia. El backup on-demand solo se paga si se crea; "
            "el coste real y los creditos siguen NOT_VERIFIED."
        ),
    }


def coste_restauracion(gb: Any, catalogo: Mapping[str, Any]) -> str:
    cantidad = _cantidad(gb, "gb_restauracion")
    if cantidad <= 0:
        raise ErrorUso("'gb_restauracion' debe ser mayor que cero.")
    return str(cuantizar(precio(catalogo, "DDB_RESTORE_GB") * cantidad))


def validar_objetivos(inventario: Mapping[str, Any]) -> dict[str, Any]:
    objetivos = inventario.get("objetivos", {})
    rto = _cantidad(objetivos.get("rto_horas"), "rto_horas")
    rpo = _cantidad(objetivos.get("rpo_horas"), "rpo_horas")
    if rto <= 0 or rpo <= 0:
        raise ErrorInventario("RTO y RPO deben ser positivos.")
    return {
        "rto_horas": str(rto),
        "rpo_horas": str(rpo),
        "medido_declarado": bool(objetivos.get("medido")),
        "medido": False,
        "estado": RTO_RPO_NO_MEDIDO,
        "advertencia": (
            "Un objetivo no medido no es un objetivo cumplido: ningun valor local cambia el "
            "estado; se exige restore real con duracion registrada y Gate independiente."
        ),
    }


def verificar_inventario(inventario: Mapping[str, Any]) -> dict[str, Any]:
    faltantes: list[str] = []
    for activo in inventario["activos"]:
        for campo in ("nombre", "tipo", "mecanismo", "retencion_dias", "estado", "protegido"):
            if campo not in activo:
                faltantes.append(f"{activo.get('nombre', '?')}: falta {campo}")
        if activo.get("protegido") and activo.get("estado") not in ESTADOS_PROTEGIDOS:
            faltantes.append(f"{activo.get('nombre', '?')}: estado no reconocido")
    politica = inventario.get("politica", {})
    for campo in ("datos_activos", "restore", "state", "secretos", "borrado_cuenta"):
        if not politica.get(campo):
            faltantes.append(f"politica: falta {campo}")
    return {
        "activos": len(inventario["activos"]),
        "faltantes": faltantes,
        "ok": not faltantes,
        "estado": "INVENTARIO_VALIDADO" if not faltantes else "INVENTARIO_INCOMPLETO",
    }


def plan_restauracion(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    solicitud: Mapping[str, Any] | None,
) -> dict[str, Any]:
    if solicitud is None:
        raise ErrorUso("Plan de restauracion requiere --solicitud con tabla y punto de restauracion.")
    origen = str(solicitud.get("tabla_origen", "")).strip()
    destino = str(solicitud.get("tabla_destino", "")).strip()
    punto = str(solicitud.get("punto_restauracion", "")).strip()
    if not origen or not destino or not punto:
        raise ErrorUso("La solicitud debe declarar tabla_origen, tabla_destino y punto_restauracion.")

    patron = inventario["restore"]["patron_tabla_destino"]
    if not re.match(patron, destino):
        raise ErrorUso(
            f"La tabla de restauracion '{destino}' no cumple el patron aislado {patron}."
        )
    if destino == origen:
        raise ErrorUso("La restauracion sobre la tabla activa esta prohibida (INF-130-04).")
    if destino in {activo["nombre"] for activo in inventario["activos"]}:
        raise ErrorUso(f"'{destino}' es un activo del inventario: no es una tabla nueva aislada.")
    if solicitud.get("sobreescribir"):
        raise ErrorUso("'sobreescribir' esta prohibido: la restauracion solo crea tabla nueva.")

    return {
        "comando": "plan-restauracion",
        "estado": "PLAN_LOCAL_VALIDADO",
        "alcance_aws": RESTORE_AWS,
        "autoriza_ejecucion_real": False,
        "tabla_origen": origen,
        "tabla_destino": destino,
        "punto_restauracion": punto,
        "gb_restauracion": str(_cantidad(solicitud.get("gb_restauracion", "0"), "gb_restauracion")),
        "coste_restauracion_usd": coste_restauracion(solicitud.get("gb_restauracion"), catalogo),
        "etiqueta_obligatoria": inventario["restore"]["etiqueta_obligatoria"],
        "pasos": [
            "verificar que el PITR cubre el punto solicitado (AWS, solo lectura)",
            "crear tabla NUEVA con el nombre autorizado y etiqueta de verificacion",
            "comparar recuentos y checksums contra la tabla activa",
            "validar invariantes de saldos y aportaciones sobre datos sinteticos",
            "registrar duracion real para medir RTO y RPO",
            "eliminar la tabla de verificacion solo con Gate y autorizacion humana",
        ],
        "advertencia": "Plan local. El restore real exige AWS, coste por GB y Gate de despliegue.",
    }


def _centavos(valor: Any, campo: str) -> Decimal:
    """Los importes se expresan en centavos **enteros**; float, bool y texto no valen."""
    if isinstance(valor, bool):
        raise ErrorUso(f"{campo}: un booleano no es una cantidad de centavos.")
    if isinstance(valor, float):
        raise ErrorUso(f"{campo}: {valor!r} es float; se exigen centavos enteros (R-030).")
    if isinstance(valor, str):
        if not re.fullmatch(r"-?[0-9]+", valor.strip()):
            raise ErrorUso(f"{campo}: {valor!r} no es un entero de centavos.")
        return Decimal(valor.strip())
    if isinstance(valor, int):
        return Decimal(valor)
    raise ErrorUso(f"{campo}: tipo no admitido {type(valor).__name__}.")


def verificar_restore_sintetico(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("La verificacion sintetica requiere --datos con movimientos y metas.")
    saved = _centavos(datos.get("saved_centavos", "0"), "saved_centavos")
    apertura = _centavos(datos.get("opening_saved_centavos", "0"), "opening_saved_centavos")
    asignaciones = datos.get("allocations_centavos", [])
    if not isinstance(asignaciones, list):
        raise ErrorUso("'allocations_centavos' debe ser una lista.")
    valores = [_centavos(valor, "allocations_centavos") for valor in asignaciones]
    total = sum(valores, Decimal("0"))
    if any(valor < 0 for valor in valores):
        raise ErrorUso("Una asignacion negativa rompe la invariante de saldos.")
    esperado = apertura + total
    return {
        "comando": "verificar-restore",
        "estado": "INVARIANTES_OK" if saved == esperado else "INVARIANTES_ROTAS",
        "saved_centavos": str(saved),
        "opening_saved_centavos": str(apertura),
        "suma_asignaciones_centavos": str(total),
        "esperado_centavos": str(esperado),
        "alcance_aws": RESTORE_AWS,
        "advertencia": "Verificacion aritmetica sobre datos sinteticos; no acredita un restore real.",
    }


def evaluar_borrado_cuenta(politica: Mapping[str, Any] | None) -> dict[str, Any]:
    if politica is None:
        raise ErrorUso("Se requiere --politica con la politica de retencion y borrado.")
    if politica.get("borrado_inmediato_backups") is True:
        raise ErrorUso(
            "No se puede afirmar borrado inmediato de copias inmutables: es falso en AWS."
        )
    if politica.get("afirma_borrado_inmutable") is True:
        raise ErrorUso("No se puede afirmar borrado de copias inmutables sin evidencia.")
    if not politica.get("backups") or not politica.get("datos_activos"):
        raise ErrorUso("La politica debe describir datos activos y copias.")
    return {
        "comando": "borrado-cuenta",
        "estado": "BORRADO_DIFERIDO_DOCUMENTADO",
        "datos_activos": politica["datos_activos"],
        "backups": politica["backups"],
        "afirma_borrado_inmediato_de_copias": False,
        "advertencia": "La eliminacion de copias sigue la retencion; no se declara borrado instantaneo.",
    }


def estado(inventario: Mapping[str, Any], catalogo: Mapping[str, Any]) -> dict[str, Any]:
    return {
        "comando": "estado",
        "objetivos": validar_objetivos(inventario),
        "inventario": verificar_inventario(inventario),
        "coste": coste_respaldo(inventario, catalogo),
        "politica": inventario.get("politica", {}),
        "restauracion_aws": RESTORE_AWS,
        "aws_consultado": False,
        "terraform_ejecutado": False,
        "advertencia": "Sin AWS: no se declara backup creado, restore verificado ni RTO/RPO cumplido.",
    }


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="verificar_backup_restore.py",
        description="Respaldo y restauracion locales de I-130. No consulta AWS.",
    )
    parser.add_argument(
        "comando",
        help="estado | plan-restauracion | verificar-restore | borrado-cuenta | verificar-inventario",
    )
    parser.add_argument("--inventario", default=str(INVENTARIO_DEFECTO))
    parser.add_argument("--catalogo", default=str(CATALOGO_DEFECTO))
    parser.add_argument("--solicitud", default=None)
    parser.add_argument("--datos", default=None)
    parser.add_argument("--politica", default=None)
    parser.add_argument("--salida", default=None)
    parser.add_argument("--ahora", default=None)
    return parser


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    comando = args.comando.strip().lower()

    if comando in COMANDOS_PROHIBIDOS:
        raise ErrorUso(
            "PROHIBIDO: destruir datos o restauracion sobre produccion no existe en I-130; "
            "el restore solo crea una tabla nueva aislada."
        )

    inventario = cargar_inventario(args.inventario)
    catalogo = cargar_catalogo(args.catalogo)

    if comando == "estado":
        resultado = estado(inventario, catalogo)
    elif comando == "verificar-inventario":
        verificacion = verificar_inventario(inventario)
        resultado = {"comando": comando, **verificacion, "objetivos": validar_objetivos(inventario)}
    elif comando == "plan-restauracion":
        resultado = plan_restauracion(inventario, catalogo, cargar_json(args.solicitud) if args.solicitud else None)
    elif comando == "verificar-restore":
        resultado = verificar_restore_sintetico(cargar_json(args.datos) if args.datos else None)
    elif comando == "borrado-cuenta":
        resultado = evaluar_borrado_cuenta(cargar_json(args.politica) if args.politica else None)
    else:
        raise ErrorUso(
            f"Comando desconocido: {comando!r}. Solo existen: estado, plan-restauracion, "
            "verificar-restore, borrado-cuenta, verificar-inventario."
        )

    resultado["generado_utc"] = args.ahora or datetime.now(timezone.utc).isoformat(timespec="seconds")
    resultado.setdefault("aws_consultado", False)
    resultado.setdefault("terraform_ejecutado", False)
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )

    codigo = 0
    if resultado.get("estado") in ("INVENTARIO_INCOMPLETO", "INVARIANTES_ROTAS"):
        codigo = 1
    return codigo, resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except ErrorUso as exc:
        print(f"DENEGADO: {exc}", file=sys.stderr)
        return 2
    except ErrorInventario as exc:
        print(f"INVENTARIO_INVALIDO: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
