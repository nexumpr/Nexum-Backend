"""Operador determinista de encendido/apagado de Nexum-Backend (spec I-120).

Reglas que este programa hace cumplir:

- **Nunca** habla con AWS ni ejecuta Terraform: no importa SDK de nube ni lanza
  procesos externos. Todo lo que produce es un *plan* local reproducible
  (``AWS_NOT_RUN``).
- El *core* (2 EC2 Go, ASG, ALB, CloudFront, Redis, DynamoDB, red esencial,
  state, secretos, monitoreo) es 24x7 e intocable: ningun comando auxiliar puede
  detenerlo, escalarlo por debajo de 2, eliminarlo ni destruirlo.
- ``criticality=auxiliary`` solo se opera si figura en la allowlist revisada.
- Un recurso sin etiqueta ``nexum:criticality`` valida, desconocido o incoherente
  se **deniega** siempre (deny por defecto).
- ``destroy-data``, ``destroy-state`` y ``destroy-all`` **no existen** como
  capacidades; se rechazan con codigo 2 y sin efecto.
- Toda operacion capaz de mutar infraestructura esta **deshabilitada** en
  I-120 LOCAL: ``apply`` devuelve un plan y codigo 2. Ningun flag lo habilita.
- El dinero se calcula con ``Decimal`` (R-030): nunca float.

Codigos de salida: 0 correcto, 1 denegado/bloqueado, 2 uso invalido o prohibido.
"""

from __future__ import annotations

import argparse
import hmac
import json
import sys
from datetime import datetime, timezone
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path
from typing import Any, Callable, Iterable, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
INVENTARIO_DEFECTO = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"
CATALOGO_DEFECTO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"

CUATRO_DECIMALES = Decimal("0.0001")
HORAS_MES = Decimal("730")  # 24 x 365 / 12, convencion de facturacion AWS
HORAS_45_DIAS = Decimal("1080")

CRITICIDADES_VALIDAS = ("core", "auxiliary")
BASES_VALIDAS = ("hora", "mes", "existencia", "variable")

ESTADOS_RESIDUAL = ("DECLARADO_NO_CREADO", "PRESUPUESTADO_NO_CREADO")
ESTADOS_NO_COMPUTABLES = ("BLOQUEADO_RFC004",)

ESTADOS_RUNTIME = ("OFF", "STARTING", "ON_2_HEALTHY", "DRAINING", "ERROR", "COST_REVIEW")

TRANSICIONES = {
    "OFF": ("STARTING",),
    "STARTING": ("ON_2_HEALTHY", "DRAINING", "ERROR"),
    "ON_2_HEALTHY": ("DRAINING", "COST_REVIEW", "ERROR"),
    "DRAINING": ("OFF", "ERROR"),
    "COST_REVIEW": ("ON_2_HEALTHY", "DRAINING", "ERROR"),
    "ERROR": ("DRAINING", "STARTING"),
}

# Estados que describen el runtime auxiliar; nunca el core compartido 24x7.
ESTADOS_AUXILIARES = ("OFF", "DRAINING", "ERROR")
ESTADO_CORE_NUNCA = ("OFF",)

COMANDOS = {
    "estado": "estado",
    "status": "estado",
    "plan": "plan",
    "encender": "encender",
    "start": "encender",
    "suspender-aux": "suspender-aux",
    "suspend-aux": "suspender-aux",
    "reanudar-aux": "reanudar-aux",
    "resume-aux": "reanudar-aux",
    "reconciliar": "reconciliar",
    "reconcile": "reconciliar",
    "rollback": "rollback",
    "destruir-aux": "destruir-aux",
    "destroy-aux": "destruir-aux",
    "aplicar": "aplicar",
    "apply": "aplicar",
}

# Nombres que un operador podria teclear y que este Gate no autoriza jamas.
COMANDOS_PROHIBIDOS = frozenset(
    {
        "destroy-all",
        "destroy-all-resources",
        "destruir-todo",
        "destruir_todo",
        "destruir-datos",
        "destruir_datos",
        "destroy-data",
        "destruir-estado",
        "destruir_estado",
        "destroy-state",
        "destroy-infra",
        "destroy-runtime-core",
        "terraform-apply",
        "terraform-destroy",
        "destroy",
        "refresh",
    }
)

DESTRUIR_DATOS_MSG = (
    "PROHIBIDO: destroy-data no existe en I-120. DynamoDB nunca se destruye por "
    "rutina de ahorro (spec I-120 seccion 9; constitution R-035)."
)
DESTRUIR_ESTADO_MSG = (
    "PROHIBIDO: destroy-state no existe en I-120. El state Terraform tiene "
    "prevent_destroy y perderlo impide reconciliar (I-070)."
)
DESTRUIR_TODO_MSG = (
    "PROHIBIDO: destroy-all no existe. Los destroys estan separados por diseno "
    "(spec I-120 INF-120-05: destroy-runtime / destroy-data / destroy-state)."
)
EJECUCION_MSG = (
    "EJECUCION_REAL_DESHABILITADA_EN_I120_LOCAL: apply solo produce plan. Mutar "
    "infraestructura exige Gate de despliegue independiente y entorno protegido "
    "(I-090); ningun flag --force/--yes habilita efectos."
)


class ErrorUso(Exception):
    """Uso invalido, comando prohibido o ejecucion no autorizada (codigo 2)."""


class ErrorInventario(Exception):
    """Inventario malformado (codigo 2). No se tolera a la baja."""


class ErrorOperacion(Exception):
    """Fallo de una operacion simulada, apto para reintento."""


# --------------------------------------------------------------------------- #
# Carga y validacion
# --------------------------------------------------------------------------- #


def cargar_json(ruta: Path | str) -> Any:
    return json.loads(Path(ruta).read_text(encoding="utf-8"))


def cargar_inventario(ruta: Path | str = INVENTARIO_DEFECTO) -> dict[str, Any]:
    datos = cargar_json(ruta)
    if not isinstance(datos, dict) or not isinstance(datos.get("items"), list):
        raise ErrorInventario("El inventario debe ser un objeto con lista 'items'.")
    if not datos["items"]:
        raise ErrorInventario(
            "Inventario vacio: una lista vacia haria creer que no hay core que proteger."
        )
    return datos


def cargar_catalogo(ruta: Path | str = CATALOGO_DEFECTO) -> dict[str, Any]:
    datos = cargar_json(ruta)
    if not isinstance(datos, dict) or not isinstance(datos.get("items"), dict):
        raise ErrorInventario("El catalogo de precios debe exponer 'items'.")
    return datos


def _decimal(valor: Any, campo: str) -> Decimal:
    if valor is None:
        raise ErrorInventario(f"Cantidad nula en '{campo}'.")
    try:
        return Decimal(str(valor))
    except Exception as exc:  # pragma: no cover - defensivo
        raise ErrorInventario(f"Cantidad no decimal en '{campo}': {exc}") from exc


def validar_item(item: Mapping[str, Any]) -> None:
    obligatorios = (
        "nombre",
        "componente",
        "tipo",
        "criticidad",
        "etiqueta_criticidad",
        "estado_recurso",
        "suspendible",
    )
    for campo in obligatorios:
        if campo not in item:
            raise ErrorInventario(f"Item sin campo obligatorio '{campo}': {item!r}")
    if item["criticidad"] not in CRITICIDADES_VALIDAS:
        raise ErrorInventario(
            f"criticality invalida en '{item['nombre']}': {item['criticidad']!r}. "
            "Solo se admite core o auxiliary; cualquier otro valor se deniega."
        )
    if not isinstance(item["suspendible"], bool):
        raise ErrorInventario(f"'suspendible' debe ser booleano en '{item['nombre']}'.")
    for partida in item.get("partidas", []) or []:
        if partida.get("base") not in BASES_VALIDAS:
            raise ErrorInventario(
                f"Base de partida invalida en '{item['nombre']}': {partida.get('base')!r}"
            )
        if "sku" not in partida:
            raise ErrorInventario(f"Partida sin sku en '{item['nombre']}'.")


def validar_inventario(inventario: Mapping[str, Any]) -> None:
    vistos: set[str] = set()
    for item in inventario["items"]:
        validar_item(item)
        nombre = item["nombre"]
        if nombre in vistos:
            raise ErrorInventario(f"Recurso duplicado en el inventario: {nombre}")
        vistos.add(nombre)


# --------------------------------------------------------------------------- #
# Clasificacion (deny por defecto)
# --------------------------------------------------------------------------- #


def clasificar(
    inventario: Mapping[str, Any], allowlist: Iterable[str] | None = None
) -> dict[str, Any]:
    """Clasifica y autoriza. Una allowlist vacia u omitida NO autoriza nada.

    - ``core``: recursos intocables.
    - ``auxiliares``: auxiliares coherentes del inventario (solo informativo).
    - ``suspendibles``: interseccion auxiliar + allowlist revisada (autorizados).
    - ``denegados``: motivo por el que no se puede operar el recurso.
    """
    permitidos = set(allowlist or ())
    core: list[str] = []
    auxiliares: list[str] = []
    suspendibles: list[str] = []
    denegados: dict[str, str] = {}

    for item in inventario["items"]:
        nombre = item["nombre"]
        if item["etiqueta_criticidad"] != item["criticidad"]:
            denegados[nombre] = "etiqueta_criticidad no coincide con criticidad"
            continue
        if item["criticidad"] == "core":
            if item["suspendible"]:
                denegados[nombre] = "recurso core marcado como suspendible"
                continue
            core.append(nombre)
            continue
        auxiliares.append(nombre)
        if not item["suspendible"]:
            denegados[nombre] = "auxiliar no suspendible: clasificacion incoherente"
            continue
        if nombre not in permitidos:
            denegados[nombre] = "auxiliar fuera de la allowlist revisada"
            continue
        suspendibles.append(nombre)

    # Una allowlist con nombres inventados tampoco autoriza: se denuncia.
    inventario_nombres = {item["nombre"] for item in inventario["items"]}
    for nombre in sorted(permitidos - inventario_nombres):
        denegados[nombre] = "allowlist con recurso inexistente en el inventario"

    return {
        "core": sorted(core),
        "auxiliares": sorted(auxiliares),
        "suspendibles": sorted(suspendibles),
        "denegados": {k: denegados[k] for k in sorted(denegados)},
        "allowlist": sorted(permitidos),
        "autorizacion_operaciones": "ALLOWLIST_EXPLICITA",
    }

def validar_allowlist(
    inventario: Mapping[str, Any], allowlist: Iterable[str] | None
) -> list[str]:
    """La allowlist debe ser explicita, no vacia y sin core ni nombres inventados."""
    nombres = sorted(set(allowlist or ()))
    if not nombres:
        raise ErrorUso(
            "Allowlist vacia u omitida: no autoriza ningun recurso. La pertenencia "
            "explicita a la allowlist revisada es requisito, y el inventario por si "
            "solo no concede autorizacion."
        )
    por_nombre = {item["nombre"]: item for item in inventario["items"]}
    core = {item["nombre"] for item in inventario["items"] if item["criticidad"] == "core"}
    intrusos = [n for n in nombres if n in core]
    if intrusos:
        raise ErrorUso(
            "La allowlist no puede incluir recursos core: " + ", ".join(intrusos)
        )
    desconocidos = [n for n in nombres if n not in por_nombre]
    if desconocidos:
        raise ErrorUso(
            "La allowlist incluye recursos que no existen en el inventario: "
            + ", ".join(desconocidos)
        )
    etiqueta_mala = [
        n for n in nombres if por_nombre[n]["etiqueta_criticidad"] != "auxiliary"
    ]
    if etiqueta_mala:
        raise ErrorUso(
            "La allowlist solo admite recursos con etiqueta nexum:criticality=auxiliary: "
            + ", ".join(etiqueta_mala)
        )
    return nombres


IMPACTOS_CORE = ("NINGUNO", "VENTANA_DECLARADA", "BLOQUEANTE")


def dependencias_core(inventario: Mapping[str, Any], nombres: Sequence[str]) -> dict[str, str]:
    """Impacto indirecto declarado: BLOQUEANTE impide operar el auxiliar.

    VENTANA_DECLARADA (D6 de I-000) es el unico impacto admitido: el recurso es
    necesario solo dentro de la ventana aprobada de mantenimiento.
    """
    por_nombre = {item["nombre"]: item for item in inventario["items"]}
    impacto: dict[str, str] = {}
    for nombre in nombres:
        item = por_nombre.get(nombre)
        if item is None:
            raise ErrorInventario(f"Recurso inexistente en el inventario: {nombre}")
        valor = item.get("impacto_core", "NINGUNO")
        if valor not in IMPACTOS_CORE:
            raise ErrorInventario(f"impacto_core invalido en '{nombre}': {valor!r}")
        if valor == "BLOQUEANTE":
            impacto[nombre] = valor
    return impacto


# --------------------------------------------------------------------------- #
# Coste exacto (Decimal, R-030)
# --------------------------------------------------------------------------- #


def precio_sku(catalogo: Mapping[str, Any], sku: str) -> Decimal:
    items = catalogo["items"]
    if sku not in items:
        raise ErrorInventario(f"SKU desconocido en el catalogo de precios: {sku}")
    bruto = items[sku].get("precio_usd")
    if bruto is None:
        raise ErrorInventario(
            f"SKU sin precio verificado: {sku}. No se admite inferir un precio."
        )
    return Decimal(str(bruto))


def costo_horario_item(item: Mapping[str, Any], catalogo: Mapping[str, Any]) -> Decimal:
    """Coste por hora de un item. 'existencia' y 'variable' no aportan coste fijo."""
    total = Decimal("0")
    for partida in item.get("partidas", []) or []:
        base = partida.get("base")
        if base in ("existencia", "variable") or partida.get("cantidad") is None:
            continue
        precio = precio_sku(catalogo, partida["sku"])
        cantidad = _decimal(partida["cantidad"], f"{item['nombre']}.{partida['sku']}")
        importe = precio * cantidad
        total += importe if base == "hora" else importe / HORAS_MES
    return total


def residual(
    inventario: Mapping[str, Any], catalogo: Mapping[str, Any]
) -> dict[str, Any]:
    """Coste que persiste por existencia (no depende del trafico)."""
    residente = Decimal("0")
    presupuestado = Decimal("0")
    no_computable: dict[str, str] = {}
    variable: list[str] = []
    detalle: dict[str, str] = {}

    for item in inventario["items"]:
        if item["criticidad"] != "core":
            continue
        estado = item["estado_recurso"]
        if estado in ESTADOS_NO_COMPUTABLES:
            no_computable[item["nombre"]] = estado
            continue
        if estado not in ESTADOS_RESIDUAL:
            no_computable[item["nombre"]] = f"estado no reconocido: {estado}"
            continue
        if any((p.get("base") == "variable") for p in item.get("partidas", []) or []):
            variable.append(item["nombre"])
        coste = costo_horario_item(item, catalogo)
        detalle[item["nombre"]] = str(cuantizar(coste))
        if estado == "DECLARADO_NO_CREADO":
            residente += coste
        else:
            presupuestado += coste

    total_hora = residente + presupuestado
    return {
        "por_recurso_hora_usd": {k: detalle[k] for k in sorted(detalle)},
        "residente_hora_usd": str(cuantizar(residente)),
        "presupuestado_hora_usd": str(cuantizar(presupuestado)),
        "total_hora_usd": str(cuantizar(total_hora)),
        "residente_45d_usd": str(cuantizar(residente * HORAS_45_DIAS)),
        "presupuestado_45d_usd": str(cuantizar(presupuestado * HORAS_45_DIAS)),
        "total_45d_usd": str(cuantizar(total_hora * HORAS_45_DIAS)),
        "neto_con_creditos_usd": None,
        "creditos_aplicables_usd": None,
        "margen_seguridad_10_usd": str(cuantizar(total_hora * HORAS_45_DIAS * Decimal("1.10"))),
        "no_computable": {k: no_computable[k] for k in sorted(no_computable)},
        "componentes_variables": sorted(variable),
        "moneda": "USD",
        "horas_mes": str(HORAS_MES),
        "advertencia": (
            "Solo coste fijo por existencia. Las partidas variables (RRU/WRU, "
            "ingesta, egreso CloudFront, solicitudes S3, interferencia Redis) y "
            "las reservas de I-000 NO se incluyen ni se declaran cero. El costo "
            "neto es desconocido sin creditos verificados."
        ),
    }


def cuantizar(valor: Decimal) -> Decimal:
    return valor.quantize(CUATRO_DECIMALES, rounding=ROUND_HALF_UP)


def ahorro_auxiliares(
    inventario: Mapping[str, Any], catalogo: Mapping[str, Any], nombres: Sequence[str]
) -> dict[str, str]:
    total = Decimal("0")
    detalle: dict[str, str] = {}
    por_nombre = {item["nombre"]: item for item in inventario["items"]}
    for nombre in nombres:
        item = por_nombre.get(nombre)
        if item is None:
            raise ErrorInventario(f"Recurso inexistente en el inventario: {nombre}")
        coste = costo_horario_item(item, catalogo)
        detalle[nombre] = str(cuantizar(coste))
        total += coste
    return {
        "por_recurso_hora_usd": {k: detalle[k] for k in sorted(detalle)},
        "total_hora_usd": str(cuantizar(total)),
        "total_45d_usd": str(cuantizar(total * HORAS_45_DIAS)),
    }


# --------------------------------------------------------------------------- #
# Maquina de estados (INF-120-01)
# --------------------------------------------------------------------------- #


def transicion(
    desde: str,
    hacia: str,
    actor: str | None,
    runbook: str | None,
    ahora: str | None = None,
    **contexto: Any,
) -> dict[str, Any]:
    """Transicion validada con el registro completo exigido por INF-120-01."""
    if desde not in ESTADOS_RUNTIME:
        raise ErrorUso(f"Estado de origen desconocido: {desde!r}")
    if hacia not in ESTADOS_RUNTIME:
        raise ErrorUso(f"Estado de destino desconocido: {hacia!r}")

    registro = {
        "estado_anterior": desde,
        "estado_nuevo": hacia,
        "actor": actor or "DESCONOCIDO",
        "timestamp_utc": ahora,
        "entorno": contexto.get("entorno"),
        "recurso": contexto.get("recurso", "runtime"),
        "motivo": contexto.get("motivo"),
        "operacion_id": contexto.get("operacion_id"),
        "autorizacion": contexto.get("autorizacion"),
        "resultado": "IDEMPOTENTE" if desde == hacia else "PLANIFICADO",
        "evidencia": contexto.get("evidencia"),
        "rollback": contexto.get("rollback", "RUNBOOK_START_STOP.md#rollback"),
        "permitida": True,
        "idempotente": desde == hacia,
        "desde": desde,
        "hacia": hacia,
        "runbook": runbook,
        "actor_operador": actor,
    }

    if desde == hacia:
        return registro

    if hacia not in TRANSICIONES[desde]:
        raise ErrorUso(
            f"Transicion no permitida: {desde} -> {hacia}. "
            f"Desde {desde} solo se admite {', '.join(TRANSICIONES[desde])}."
        )
    if not actor:
        raise ErrorUso("Toda transicion exige --actor identificable.")
    if not runbook:
        raise ErrorUso("Toda transicion exige --runbook (procedimiento aplicado).")

    registro["timestamp_utc"] = ahora or datetime.now(timezone.utc).isoformat(timespec="seconds")
    return registro


def es_estado_auxiliar(estado: str) -> bool:
    """OFF nunca es un estado valido del core compartido 24x7 (spec seccion 9)."""
    return estado in ESTADOS_AUXILIARES


# --------------------------------------------------------------------------- #
# Alcance real del plan (proteccion contra destruccion indirecta)
# --------------------------------------------------------------------------- #

ACCIONES_PROHIBIDAS = ("delete", "destroy", "forget")
CAMPOS_CAPACIDAD = ("desired", "desired_capacity", "desiredCapacity", "max_size")

# Un JSON local no acredita el alcance de un plan Terraform real aplicado contra AWS.
TERRAFORM_REAL_PLAN_NOT_VERIFIED = True


def verificar_alcance_plan(
    cambios: Sequence[Mapping[str, Any]] | None,
    core: Sequence[str],
    allowlist: Sequence[str],
    requiere_cambios: bool = True,
) -> dict[str, Any]:
    """Valida un plan local contra el inventario autorizado.

    Un plan ausente, vacio (cuando la operacion implica cambios) o malformado
    **no** se acepta: falla cerrado. El resultado distingue siempre
    ``PLAN_LOCAL_VALIDADO`` de ``PLAN_AWS_NO_VERIFICADO``.
    """
    base = {
        "representacion": "cambios_locales",
        "plan_terraform_real": "TERRAFORM_REAL_PLAN_NOT_VERIFIED",
        "alcance_aws": "PLAN_AWS_NO_VERIFICADO",
        "requiere_cambios": requiere_cambios,
    }

    if cambios is None:
        return {
            **base,
            "verificado": False,
            "ok": False,
            "estado": "PLAN_AUSENTE",
            "denegados": ["no se aporto plan: sin plan no se autoriza ninguna modificacion"],
            "motivo": "Plan ausente; el alcance no se asume.",
        }

    core_set = set(core)
    permitidos = set(allowlist)
    denegados: list[str] = []
    nodos_tras_cambio: int | None = None

    for cambio in cambios:
        if not isinstance(cambio, Mapping):
            raise ErrorUso("Cada cambio del plan debe ser un objeto: plan malformado.")
        direccion = str(cambio.get("direccion", "")).strip()
        accion = str(cambio.get("accion", "")).strip().lower()
        base_direccion = direccion.split("[")[0].strip()
        if not direccion:
            raise ErrorUso("Cambio sin 'direccion': plan malformado, se falla cerrado.")
        if not accion:
            raise ErrorUso(f"Cambio sin 'accion' para {direccion}: plan malformado.")
        if base_direccion in core_set:
            denegados.append(f"{direccion}: el core nunca se modifica ({accion})")
            continue
        if base_direccion not in permitidos:
            denegados.append(f"{direccion}: recurso no declarado o fuera de allowlist")
            continue
        if accion in ACCIONES_PROHIBIDAS:
            denegados.append(f"{direccion}: accion '{accion}' no permitida sobre auxiliares")
        for campo in CAMPOS_CAPACIDAD:
            if campo in cambio:
                valor = cambio[campo]
                if valor is not None:
                    nodos_tras_cambio = int(valor)
                    if nodos_tras_cambio < 2:
                        denegados.append(
                            f"{direccion}: {campo}={nodos_tras_cambio} rompe la doble instancia (GOV-02)"
                        )

    if not cambios and requiere_cambios:
        denegados.append("plan vacio: no justifica ningun cambio solicitado")

    return {
        **base,
        "verificado": True,
        "cambios": len(cambios),
        "denegados": denegados,
        "ok": not denegados,
        "nodos_tras_cambio": nodos_tras_cambio,
        "estado": "PLAN_LOCAL_VALIDADO" if not denegados else "PLAN_RECHAZADO",
    }


def exigir_alcance_plan(
    cambios: Sequence[Mapping[str, Any]] | None,
    core: Sequence[str],
    allowlist: Sequence[str],
    requiere_cambios: bool = True,
) -> dict[str, Any]:
    resultado = verificar_alcance_plan(cambios, core, allowlist, requiere_cambios)
    if not resultado.get("verificado"):
        raise ErrorUso(
            "Plan no verificable: se exige un plan de cambios aportado y evaluado; "
            "sin el no se autoriza ninguna modificacion."
        )
    if resultado.get("denegados"):
        raise ErrorUso(
            "Plan rechazado por alcance: " + "; ".join(resultado["denegados"])
        )
    return resultado


# --------------------------------------------------------------------------- #
# Aprobaciones, creditos y reintentos
# --------------------------------------------------------------------------- #


def exigir_aprobacion(token: str | None, esperado: str) -> None:
    if not token:
        raise ErrorUso(
            f"Falta aprobacion humana explicita. Se esperaba la frase exacta: {esperado}"
        )
    if not hmac.compare_digest(token.strip(), esperado):
        raise ErrorUso(
            "Aprobacion invalida: la frase no coincide exactamente con "
            f"{esperado} (distingue mayusculas y entorno)."
        )


def cargar_autorizacion(ruta: str | Path | None, frase: str, entorno: str, operacion: str) -> dict[str, Any]:
    """Registro de autorizacion revisado. Un archivo local NO habilita ejecucion."""
    if not ruta:
        raise ErrorUso(
            "Falta --autorizacion: se exige el registro revisado del Gate del entorno, "
            "no solo la frase (una frase dentro de un prompt no es un Gate)."
        )
    datos = cargar_json(ruta)
    if not isinstance(datos, Mapping):
        raise ErrorUso("El registro de autorizacion debe ser un objeto JSON.")
    if datos.get("frase") != frase:
        raise ErrorUso("El registro de autorizacion no contiene la frase esperada.")
    if datos.get("entorno") != entorno:
        raise ErrorUso("El registro de autorizacion pertenece a otro entorno.")
    if datos.get("operacion") not in (operacion, "*"):
        raise ErrorUso("El registro de autorizacion no cubre esta operacion.")
    if not datos.get("aprobador"):
        raise ErrorUso("El registro de autorizacion carece de aprobador humano.")
    return {
        "procedencia": "registro_revisado_local",
        "aprobador": datos.get("aprobador"),
        "fecha": datos.get("fecha"),
        "referencia": datos.get("referencia"),
        "habilitacion_ejecucion": "DENEGADA_EN_I120_LOCAL",
        "nota": (
            "Un archivo local no autoriza efectos reales. La ejecucion exige entorno "
            "protegido y Gate de despliegue (I-090); apply permanece deshabilitado."
        ),
    }


def evaluar_creditos(
    creditos: str | Decimal | None, umbrales: Mapping[str, Any]
) -> dict[str, Any]:
    """Umbrales INF-120/§10 + I-000. Sin dato no se asume nada (falla cerrado)."""
    if creditos is None:
        return {
            "estado": "BLOQUEADO",
            "motivo": "Creditos no verificados (AWS_NOT_RUN): no se asume saldo ni se declara cero.",
            "umbrales": dict(umbrales),
        }
    valor = Decimal(str(creditos))
    aviso = Decimal(str(umbrales.get("aviso", "45")))
    bloqueo = Decimal(str(umbrales.get("bloqueo_auxiliares", "35")))
    revision = Decimal(str(umbrales.get("revision_urgente", "30")))
    critico = Decimal(str(umbrales.get("incidente_critico", "20")))
    if valor < critico:
        estado, motivo = "INCIDENTE_CRITICO", "Saldo por debajo del umbral critico: escalar al responsable."
    elif valor <= revision:
        estado, motivo = "REVISION_URGENTE", "Saldo en revision urgente: bloquear cambios auxiliares no esenciales."
    elif valor <= bloqueo:
        estado, motivo = "BLOQUEADO", "Saldo por debajo del umbral de bloqueo de auxiliares."
    elif valor <= aviso:
        estado, motivo = "AVISO", "Saldo por debajo del umbral de aviso; mantener ventanas al minimo."
    else:
        estado, motivo = "OK", "Saldo por encima del umbral de aviso."
    return {
        "estado": estado,
        "motivo": motivo,
        "creditos_usd": str(cuantizar(valor)),
        "umbrales": dict(umbrales),
        "advertencia": "Los umbrales generan alertas, no cortes automaticos, y no autorizan plan Paid.",
    }


def ejecutar_con_reintentos(
    accion: Callable[[], Any], intentos: int = 3
) -> tuple[Any, list[str]]:
    """Reintenta fallos transitorios y devuelve el historial. No oculta el fallo final."""
    errores: list[str] = []
    for _ in range(max(1, int(intentos))):
        try:
            return accion(), errores
        except ErrorOperacion as exc:
            errores.append(str(exc))
    raise ErrorOperacion(
        f"Operacion fallida tras {intentos} intentos: " + "; ".join(errores)
    )


def simular_aplicacion(
    objetivo: Sequence[str], fallos: Iterable[str] = ()
) -> dict[str, Any]:
    """Aplicacion simulada con fallos parciales declarados por quien prueba."""
    fallidos = set(fallos)
    aplicados, errores = [], []
    for nombre in objetivo:
        if nombre in fallidos:
            errores.append(nombre)
        else:
            aplicados.append(nombre)
    resultado = "PARCIAL" if aplicados and errores else ("FALLIDO" if errores else "APLICADO")
    return {
        "resultado": resultado,
        "aplicados": aplicados,
        "fallidos": errores,
        "rollback": "RUNBOOK_START_STOP.md#rollback",
        "datos_destruidos": False,
        "estado_destruido": False,
        "ejecutado_en_aws": False,
    }


# --------------------------------------------------------------------------- #
# Planes por comando
# --------------------------------------------------------------------------- #


def _dos_nodos(targets: Any, minimo: int = 2) -> dict[str, Any]:
    sano = False
    cantidad = None
    if targets is not None:
        try:
            cantidad = int(targets)
        except (TypeError, ValueError):
            raise ErrorUso(f"targets_sanos invalido: {targets!r}")
        sano = cantidad >= int(minimo)
    return {
        "targets_sanos": cantidad,
        "minimo_requerido": int(minimo),
        "cumple_doble_instancia": sano,
        "publicable": sano,
        "motivo": (
            "2 o mas targets sanos"
            if sano
            else "No se puede publicar el servicio: 1 nodo no es doble instancia (GOV-02)."
        ),
        "alcance": "disponibilidad_global_del_backend",
    }


def plan_estado(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    allowlist: Sequence[str],
    observado: Mapping[str, Any] | None,
) -> dict[str, Any]:
    clasif = clasificar(inventario, allowlist)
    estado = (observado or {}).get("estado", "DESCONOCIDO")
    return {
        "comando": "estado",
        "estado_runtime": estado,
        "estado_origen": "observado" if observado else "DESCONOCIDO (sin evidencia AWS)",
        "off_sobre_core": estado in ESTADO_CORE_NUNCA,
        "clasificacion": clasif,
        "residual": residual(inventario, catalogo),
        "dos_nodos": _dos_nodos(
            (observado or {}).get("targets_sanos"),
            (observado or {}).get("minimo_instancias", 2),
        ),
        "advertencia": "Estado leido de un archivo observado; este Gate no consulta AWS.",
    }


def plan_encender(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    allowlist: Sequence[str],
    observado: Mapping[str, Any] | None,
    aprobacion: str | None,
    entorno: str,
    actor: str | None,
    runbook: str | None,
    ahora: str | None,
    motivo: str | None = None,
    operacion_id: str | None = None,
    autorizacion: str | None = None,
) -> dict[str, Any]:
    exigir_aprobacion(aprobacion, f"APRUEBO_ENCENDER_{entorno.upper()}")
    obs = observado or {}
    desde = obs.get("estado", "OFF")
    dos = _dos_nodos(obs.get("targets_sanos"), 2)
    hacia = "ON_2_HEALTHY" if dos["cumple_doble_instancia"] else "STARTING"
    registro = transicion(
        desde,
        hacia,
        actor,
        runbook,
        ahora,
        entorno=entorno,
        recurso="runtime",
        motivo=motivo or "reanudacion planificada",
        operacion_id=operacion_id,
        autorizacion=autorizacion,
        evidencia=obs.get("evidencia"),
    )
    return {
        "comando": "encender",
        "transicion": registro,
        "dos_nodos": dos,
        "acciones": [
            "verificar plan Free, creditos y cuota antes de publicar",
            "desiredCapacity=2 (min=desired=max)",
            "comprobar 2 targets sanos en /api/v1/health/live; readiness segun su Gate",
            "no exponer la URL publica hasta ON_2_HEALTHY",
        ],
        "residual": residual(inventario, catalogo),
        "ejecutado_en_aws": False,
        "advertencia": "Plan local. Terraform apply/AWS exige Gate independiente (AWS_NOT_RUN).",
    }


def plan_suspender_aux(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    allowlist: Sequence[str],
    aprobacion: str | None,
    entorno: str,
    seleccion: Sequence[str] | None,
    actor: str | None,
    runbook: str | None,
    observado: Mapping[str, Any] | None,
    cambios_plan: Sequence[Mapping[str, Any]] | None = None,
) -> dict[str, Any]:
    exigir_aprobacion(aprobacion, f"APRUEBO_SUSPENDER_AUX_{entorno.upper()}")
    validar_allowlist(inventario, allowlist)
    clasif = clasificar(inventario, allowlist)
    objetivo = sorted(set(seleccion or clasif["suspendibles"]))
    if not objetivo:
        raise ErrorUso(
            "No hay recursos auxiliares autorizados: revisar allowlist antes de suspender."
        )
    intrusos = [n for n in objetivo if n not in clasif["suspendibles"]]
    if intrusos:
        raise ErrorUso(
            "Se intento suspender recursos no autorizados: "
            + ", ".join(sorted(intrusos))
            + ". El core, los desconocidos y los no etiquetados se deniegan siempre."
        )
    suspendidos_observados = set((observado or {}).get("recursos_suspendidos", []) or [])
    core_tocado = sorted(suspendidos_observados & set(clasif["core"]))
    if core_tocado:
        raise ErrorUso(
            "El core figura suspendido en el estado observado: "
            + ", ".join(core_tocado)
            + ". Es un incidente, no un ahorro."
        )
    impacto = dependencias_core(inventario, objetivo)
    if impacto:
        raise ErrorUso(
            "Suspension rechazada por impacto indirecto BLOQUEANTE sobre el core: "
            + json.dumps(impacto, ensure_ascii=False, sort_keys=True)
        )
    ya_suspendidos = sorted(set(objetivo) & suspendidos_observados)
    requiere_cambios = len(ya_suspendidos) != len(objetivo)
    alcance = exigir_alcance_plan(
        cambios_plan, clasif["core"], clasif["suspendibles"], requiere_cambios
    )
    return {
        "comando": "suspender-aux",
        "objetivo": objetivo,
        "idempotente": len(ya_suspendidos) == len(objetivo),
        "ya_suspendidos": ya_suspendidos,
        "ahorro": ahorro_auxiliares(inventario, catalogo, objetivo),
        "residual_tras_suspender": residual(inventario, catalogo),
        "no_tocado": clasif["core"],
        "denegados": clasif["denegados"],
        "alcance_plan": alcance,
        "registro": transicion(
            (observado or {}).get("estado", "ON_2_HEALTHY"),
            (observado or {}).get("estado", "ON_2_HEALTHY"),
            actor,
            runbook,
            entorno=entorno,
            recurso=",".join(objetivo),
            motivo="ventana de mantenimiento",
            autorizacion="APRUEBO_SUSPENDER_AUX",
        ),
        "actor": actor,
        "runbook": runbook,
        "ejecutado_en_aws": False,
        "dry_run": True,
    }


def plan_reanudar_aux(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    allowlist: Sequence[str],
    aprobacion: str | None,
    entorno: str,
    creditos: str | None,
    actor: str | None,
    runbook: str | None,
    pronostico: str | None = None,
    cambios_plan: Sequence[Mapping[str, Any]] | None = None,
) -> dict[str, Any]:
    exigir_aprobacion(aprobacion, f"APRUEBO_REANUDAR_AUX_{entorno.upper()}")
    validar_allowlist(inventario, allowlist)
    clasif = clasificar(inventario, allowlist)
    umbrales = inventario.get("umbrales_credito_usd", {})
    evaluacion = evaluar_creditos(creditos, umbrales)
    pronostico_eval = evaluar_pronostico(pronostico, creditos)
    bloqueado = evaluacion["estado"] in ("BLOQUEADO", "INCIDENTE_CRITICO", "REVISION_URGENTE")
    if pronostico_eval["bloquea"]:
        bloqueado = True
    alcance = exigir_alcance_plan(cambios_plan, clasif["core"], clasif["suspendibles"])
    return {
        "comando": "reanudar-aux",
        "objetivo": clasif["suspendibles"],
        "creditos": evaluacion,
        "pronostico": pronostico_eval,
        "alcance_plan": alcance,
        "estado": "BLOQUEADO" if bloqueado else "PLANIFICADO",
        "requisitos": [
            "autorizacion humana del ambiente",
            "inventario y allowlist revisados",
            "creditos y estado de cuenta verificados",
            "costos futuros calculados",
            "dependencias sin impacto en el core",
            "state Terraform preservado",
            "rollback disponible",
        ],
        "acciones": [
            "terraform plan contra el state remoto preservado (I-070)",
            "verificar prerrequisitos, cuota y creditos",
            "aplicar de forma idempotente tras aprobacion por ambiente",
            "pruebas funcionales sin regenerar secretos ni sustituir datos",
        ],
        "idempotente": True,
        "actor": actor,
        "runbook": runbook,
        "ejecutado_en_aws": False,
    }


def evaluar_pronostico(pronostico: str | None, creditos: str | None) -> dict[str, Any]:
    """Forecast > credito restante => bloqueo de nuevos despliegues auxiliares."""
    if pronostico is None or creditos is None:
        return {
            "estado": "BLOQUEADO",
            "bloquea": True,
            "motivo": "Sin forecast y creditos verificados no se autoriza reanudar auxiliares.",
        }
    previsto = Decimal(str(pronostico))
    disponible = Decimal(str(creditos))
    excede = previsto > disponible
    return {
        "estado": "EXCEDE_CREDITO" if excede else "CUBIERTO",
        "bloquea": excede,
        "pronostico_usd": str(cuantizar(previsto)),
        "credito_restante_usd": str(cuantizar(disponible)),
        "motivo": (
            "El forecast excede el credito restante: bloquear nuevos despliegues auxiliares."
            if excede
            else "El forecast cabe en el credito restante declarado."
        ),
        "advertencia": "Un forecast no es un hard cap ni una autorizacion de gasto.",
    }


def plan_reconciliar(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    allowlist: Sequence[str],
    observado: Mapping[str, Any] | None,
) -> dict[str, Any]:
    clasif = clasificar(inventario, allowlist)
    if observado is None:
        return {
            "comando": "reconciliar",
            "estado": "BLOQUEADO",
            "motivo": "Sin inventario observado no se declara convergencia ni se asume estado.",
            "residual": residual(inventario, catalogo),
        }
    presentes = set(observado.get("recursos_presentes", []) or [])
    ausentes_core = sorted(set(clasif["core"]) - presentes)
    sobrantes = sorted(presentes - set(clasif["core"]) - set(clasif["suspendibles"]))
    drift = bool(ausentes_core or sobrantes)
    return {
        "comando": "reconciliar",
        "estado": "DRIFT" if drift else "CONVERGENTE",
        "core_ausente": ausentes_core,
        "no_declarados_presentes": sobrantes,
        "dos_nodos": _dos_nodos(observado.get("targets_sanos"), 2),
        "acciones_sugeridas": [
            "revisar el plan de Terraform, no aplicar a ciegas",
            "no recrear recursos core ausentes sin diagnostico previo",
        ]
        if drift
        else [],
        "ejecutado_en_aws": False,
    }


def plan_rollback(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    allowlist: Sequence[str],
    snapshot: Mapping[str, Any] | None,
    actor: str | None,
    runbook: str | None,
) -> dict[str, Any]:
    if snapshot is None:
        raise ErrorUso("Rollback exige --snapshot con el estado deseado anterior.")
    if not actor or not runbook:
        raise ErrorUso("Rollback exige --actor y --runbook.")
    for prohibido in ("destroy_datos", "destroy_estado", "destroy_all"):
        if snapshot.get(prohibido):
            raise ErrorUso(
                f"El snapshot solicita '{prohibido}': el rollback nunca destruye datos ni estado."
            )
    clasif = clasificar(inventario, allowlist)
    deseados = set(snapshot.get("recursos_deseados", []) or [])
    no_autorizados = sorted(deseados - set(clasif["core"]) - set(clasif["suspendibles"]))
    if no_autorizados:
        raise ErrorUso(
            "El snapshot incluye recursos no autorizados: " + ", ".join(no_autorizados)
        )
    return {
        "comando": "rollback",
        "estado_anterior": snapshot.get("estado", "DESCONOCIDO"),
        "recursos_deseados": sorted(deseados) or sorted(clasif["core"]),
        "core_protegido": clasif["core"],
        "actor": actor,
        "runbook": runbook,
        "datos_destruidos": False,
        "estado_destruido": False,
        "ejecutado_en_aws": False,
    }


def plan_destruir_aux(
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    allowlist: Sequence[str],
    aprobacion: str | None,
    entorno: str,
    seleccion: Sequence[str] | None,
    actor: str | None,
    runbook: str | None,
    cambios_plan: Sequence[Mapping[str, Any]] | None = None,
) -> dict[str, Any]:
    exigir_aprobacion(aprobacion, f"APRUEBO_DESTRUIR_AUX_{entorno.upper()}")
    validar_allowlist(inventario, allowlist)
    clasif = clasificar(inventario, allowlist)
    objetivo = sorted(set(seleccion or clasif["suspendibles"]))
    intrusos = [n for n in objetivo if n not in clasif["suspendibles"]]
    if intrusos:
        raise ErrorUso(
            "destroy-runtime solo alcanza auxiliares en allowlist; se intento: "
            + ", ".join(sorted(intrusos))
        )
    impacto = dependencias_core(inventario, objetivo)
    if impacto:
        raise ErrorUso(
            "Destroy rechazado por impacto indirecto BLOQUEANTE sobre el core: "
            + json.dumps(impacto, ensure_ascii=False, sort_keys=True)
        )
    alcance = exigir_alcance_plan(cambios_plan, clasif["core"], clasif["suspendibles"])
    return {
        "comando": "destruir-aux",
        "objetivo": objetivo,
        "ahorro": ahorro_auxiliares(inventario, catalogo, objetivo),
        "residual_tras_destruir": residual(inventario, catalogo),
        "core_intacto": clasif["core"],
        "alcance_plan": alcance,
        "datos_destruidos": False,
        "estado_destruido": False,
        "actor": actor,
        "runbook": runbook,
        "ejecutado_en_aws": False,
    }


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="nexum_operacion.py",
        description="Operador local y determinista de encendido/apagado (I-120). Nunca ejecuta AWS.",
    )
    parser.add_argument(
        "comando",
        help=(
            "estado/status | plan | encender/start | suspender-aux/suspend-aux | "
            "reanudar-aux/resume-aux | reconciliar/reconcile | rollback | "
            "destruir-aux/destroy-aux | aplicar/apply (solo plan)"
        ),
    )
    parser.add_argument("--inventario", default=str(INVENTARIO_DEFECTO))
    parser.add_argument("--catalogo", default=str(CATALOGO_DEFECTO))
    parser.add_argument("--entorno", default="h08i")
    parser.add_argument("--allowlist", default=None, help="Lista separada por comas; por defecto la del inventario auxiliar")
    parser.add_argument("--observado", default=None, help="JSON con el estado observado (nunca se consulta AWS)")
    parser.add_argument("--snapshot", default=None, help="JSON con el estado deseado anterior para rollback")
    parser.add_argument("--plan-cambios", default=None, dest="plan_cambios", help="JSON con los cambios propuestos a verificar")
    parser.add_argument("--seleccion", default=None, help="Subconjunto de auxiliares separado por comas")
    parser.add_argument("--aprobacion", default=None)
    parser.add_argument("--autorizacion", default=None, help="Registro revisado del Gate del entorno")
    parser.add_argument("--creditos", default=None, help="Creditos declarados en USD (evidencia externa)")
    parser.add_argument("--pronostico", default=None, help="Gasto proyectado en USD (evidencia externa)")
    parser.add_argument("--motivo", default=None)
    parser.add_argument("--operacion-id", default=None, dest="operacion_id")
    parser.add_argument("--actor", default=None)
    parser.add_argument("--runbook", default=None)
    parser.add_argument("--ahora", default=None, help="Marca ISO-8601 para pruebas deterministas")
    parser.add_argument("--json", action="store_true", dest="salida_json")
    parser.add_argument("--salida", default=None, help="Ruta opcional donde escribir el plan JSON")
    # Flags de fuerza: se aceptan solo para rechazarlos de forma explicita.
    parser.add_argument("--force", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--yes", action="store_true", help=argparse.SUPPRESS)
    return parser


def _lista(valor: str | None) -> list[str]:
    if not valor:
        return []
    return [parte.strip() for parte in valor.split(",") if parte.strip()]


def _leer_opcional(ruta: str | None) -> Any:
    return cargar_json(ruta) if ruta else None


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    parser = construir_parser()
    args = parser.parse_args(list(argv) if argv is not None else None)

    crudo = args.comando.strip().lower()
    if crudo in COMANDOS_PROHIBIDOS:
        if crudo in ("destruir-datos", "destruir_datos", "destroy-data"):
            raise ErrorUso(DESTRUIR_DATOS_MSG)
        if crudo in ("destruir-estado", "destruir_estado", "destroy-state"):
            raise ErrorUso(DESTRUIR_ESTADO_MSG)
        raise ErrorUso(DESTRUIR_TODO_MSG)

    if getattr(args, "force", False) or getattr(args, "yes", False):
        raise ErrorUso(
            "Los flags --force/--yes no habilitan efectos: la autorizacion exige Gate "
            "humano con procedencia verificable, no un booleano."
        )

    comando = COMANDOS.get(crudo)
    if comando is None:
        raise ErrorUso(
            f"Comando desconocido: {crudo!r}. Solo existen: estado, plan, encender, "
            "suspender-aux, reanudar-aux, reconciliar, rollback, destruir-aux, aplicar."
        )

    inventario = cargar_inventario(args.inventario)
    validar_inventario(inventario)
    catalogo = cargar_catalogo(args.catalogo)

    # Sin fallback: omitir --allowlist NO autoriza todos los auxiliares del
    # inventario. Solo las operaciones de lectura admiten una allowlist vacia.
    allowlist = _lista(args.allowlist)

    observado = _leer_opcional(args.observado)
    snapshot = _leer_opcional(args.snapshot)
    cambios_plan = _leer_opcional(args.plan_cambios)
    seleccion = _lista(args.seleccion) or None

    if comando == "aplicar":
        validar_allowlist(inventario, allowlist)
        clasif = clasificar(inventario, allowlist)
        alcance = exigir_alcance_plan(cambios_plan, clasif["core"], clasif["suspendibles"])
        resultado = {
            "comando": "aplicar",
            "estado": "EJECUCION_DESHABILITADA",
            "dry_run": True,
            "alcance_plan": alcance,
            "datos_destruidos": False,
            "estado_destruido": False,
            "ejecutado_en_aws": False,
            "motivo": EJECUCION_MSG,
        }
        resultado["entorno"] = args.entorno
        resultado["generado_utc"] = args.ahora or datetime.now(timezone.utc).isoformat(timespec="seconds")
        resultado["aws_consultado"] = False
        resultado["terraform_ejecutado"] = False
        if args.salida:
            Path(args.salida).write_text(
                json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
                encoding="utf-8",
            )
        return 2, resultado

    if comando == "estado":
        resultado = plan_estado(inventario, catalogo, allowlist, observado)
    elif comando in ("plan", "encender"):
        resultado = plan_encender(
            inventario, catalogo, allowlist, observado, args.aprobacion, args.entorno,
            args.actor, args.runbook, args.ahora, args.motivo, args.operacion_id,
            args.autorizacion,
        )
    elif comando == "suspender-aux":
        autorizacion = cargar_autorizacion(
            args.autorizacion, f"APRUEBO_SUSPENDER_AUX_{args.entorno.upper()}",
            args.entorno, "suspender-aux",
        ) if args.autorizacion else None
        resultado = plan_suspender_aux(
            inventario, catalogo, allowlist, args.aprobacion, args.entorno,
            seleccion, args.actor, args.runbook, observado, cambios_plan,
        )
        resultado["autorizacion"] = autorizacion
    elif comando == "reanudar-aux":
        resultado = plan_reanudar_aux(
            inventario, catalogo, allowlist, args.aprobacion, args.entorno,
            args.creditos, args.actor, args.runbook, args.pronostico, cambios_plan,
        )
    elif comando == "reconciliar":
        resultado = plan_reconciliar(inventario, catalogo, allowlist, observado)
    elif comando == "rollback":
        resultado = plan_rollback(
            inventario, catalogo, allowlist, snapshot, args.actor, args.runbook
        )
    elif comando == "destruir-aux":
        resultado = plan_destruir_aux(
            inventario, catalogo, allowlist, args.aprobacion, args.entorno,
            seleccion, args.actor, args.runbook, cambios_plan,
        )
    else:  # pragma: no cover - cubierto por el mapa COMANDOS
        raise ErrorUso(f"Comando no implementado: {comando!r}")

    resultado["entorno"] = args.entorno
    resultado["generado_utc"] = args.ahora or datetime.now(timezone.utc).isoformat(timespec="seconds")
    resultado["operacion_id"] = args.operacion_id
    resultado["aws_consultado"] = False
    resultado["terraform_ejecutado"] = False

    codigo = 0
    if resultado.get("estado") == "BLOQUEADO":
        codigo = 1
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
    return codigo, resultado


def imprimir(resultado: Mapping[str, Any]) -> None:
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except ErrorUso as exc:
        print(f"DENEGADO: {exc}", file=sys.stderr)
        return 2
    except ErrorInventario as exc:
        print(f"INVENTARIO_INVALIDO: {exc}", file=sys.stderr)
        return 2
    imprimir(resultado)
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
