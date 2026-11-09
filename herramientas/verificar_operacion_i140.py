"""Verificador operativo de Nexum-Backend (I-140).

Valida en local, sin AWS:

- runbooks obligatorios con sus campos;
- matriz de prioridades S1/S2/S3 coherente;
- plan de drills con estados NOT_RUN y sin acciones destructivas;
- inventario derivado del código (los nombres no prueban existencia);
- política de acceso sin root, sin SSH público y con identidades temporales.

Codigos: 0 correcto, 1 incompleto/bloqueado, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
RUNBOOK = RAIZ / "sdd" / "_control" / "H08" / "RUNBOOK_OPERATIVO.md"
FIXTURES = RAIZ / "tests" / "fixtures" / "operacion"

RUNBOOKS_REQUERIDOS = (
    "deployment",
    "health-fail",
    "dynamo-throttle",
    "redis-unavailable",
    "certificados",
    "rotacion-secretos",
    "auth-inesperado",
    "presupuesto",
    "incidente",
    "eliminacion-iac",
)

CAMPOS_RUNBOOK = (
    "responsable",
    "precondiciones",
    "diagnostico",
    "mitigacion",
    "rollback",
    "evidencia",
    "criterio-exito",
    "estado",
)

NIVELES = ("S1", "S2", "S3")

DRILLS_REQUERIDOS = (
    "aislar-instancia-a",
    "reemplazo-target",
    "fallo-redis",
    "cuota-dynamo",
    "rotacion-jwt",
)

ESTADOS_DRILL = (
    "NOT_RUN",
    "SIMULADO",
    "EJECUTADO_AWS",
)

ACCIONES_PROHIBIDAS = ("delete", "destroy", "deshabilitar", "disable")


class ErrorUso(Exception):
    """Entrada invalida (codigo 2)."""


def cargar_json(ruta: Path | str) -> Any:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorUso(f"No se puede leer {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorUso(f"{ruta} no es JSON valido: {exc}") from exc


def _secciones(texto: str) -> dict[str, str]:
    secciones: dict[str, list[str]] = {}
    actual = None
    for linea in texto.splitlines():
        coincidencia = re.match(r"^##\s+([A-Za-z0-9_./-]+)\s*$", linea.strip())
        if coincidencia:
            actual = coincidencia.group(1)
            secciones[actual] = []
            continue
        if actual is not None:
            secciones[actual].append(linea)
    return {clave: "\n".join(valor) for clave, valor in secciones.items()}


def validar_runbooks(texto: str | None = None, ruta: Path | None = None) -> dict[str, Any]:
    """Valida el runbook indicado. Un fichero ausente es error, nunca un fallback silencioso."""
    if texto is None:
        objetivo = Path(ruta) if ruta is not None else RUNBOOK
        if not objetivo.is_file():
            raise ErrorUso(f"Falta el runbook {objetivo}")
        texto = objetivo.read_text(encoding="utf-8")
    secciones = _secciones(texto)
    faltantes: dict[str, list[str]] = {}
    for nombre in RUNBOOKS_REQUERIDOS:
        if nombre not in secciones:
            faltantes[nombre] = ["seccion ausente"]
            continue
        cuerpo = secciones[nombre]
        sin_campo = [campo for campo in CAMPOS_RUNBOOK if f"- **{campo}**" not in cuerpo]
        if sin_campo:
            faltantes[nombre] = sin_campo
    doc_no_ejecutado = all(
        "DOCUMENTADO_NO_EJECUTADO" in secciones.get(nombre, "")
        for nombre in RUNBOOKS_REQUERIDOS
        if nombre in secciones
    )
    return {
        "comando": "runbooks",
        "runbooks_requeridos": len(RUNBOOKS_REQUERIDOS),
        "runbooks_presentes": sum(1 for n in RUNBOOKS_REQUERIDOS if n in secciones),
        "faltantes": faltantes,
        "estado": "RUNBOOKS_COMPLETOS" if not faltantes else "RUNBOOKS_INCOMPLETOS",
        "todos_documentados_sin_ejecutar": doc_no_ejecutado,
        "ejecucion_aws": "NOT_RUN",
        "advertencia": "Un runbook documentado no acredita que se haya ejecutado.",
    }


def validar_prioridades(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con la matriz de prioridades.")
    prioridades = datos.get("prioridades")
    if not isinstance(prioridades, Mapping):
        raise ErrorUso("'prioridades' debe ser un objeto con S1/S2/S3.")
    faltantes = [nivel for nivel in NIVELES if nivel not in prioridades]
    if faltantes:
        return {
            "comando": "prioridades",
            "estado": "PRIORIDADES_INCOMPLETAS",
            "faltantes": faltantes,
            "ratificacion": "PENDIENTE",
        }

    notificaciones: dict[str, int] = {}
    problemas: list[str] = []
    for nivel in NIVELES:
        entrada = prioridades[nivel]
        for campo in ("descripcion", "tiempo_notificacion_min", "escalamiento", "rollback"):
            if not entrada.get(campo):
                problemas.append(f"{nivel}: falta {campo}")
        valor = entrada.get("tiempo_notificacion_min")
        if isinstance(valor, int) and not isinstance(valor, bool):
            notificaciones[nivel] = valor
        else:
            problemas.append(f"{nivel}: tiempo_notificacion_min invalido")
    if len(notificaciones) == 3 and not (notificaciones["S1"] <= notificaciones["S2"] <= notificaciones["S3"]):
        problemas.append("los tiempos no son crecientes S1 <= S2 <= S3")
    return {
        "comando": "prioridades",
        "estado": "PRIORIDADES_COHERENTES" if not problemas else "PRIORIDADES_INCOHERENTES",
        "problemas": problemas,
        "tiempos_notificacion": notificaciones,
        "ratificacion": "PENDIENTE_DE_RATIFICACION_HUMANA",
        "advertencia": "Los tiempos son propuesta operativa; no sustituyen un SLA ratificado.",
    }


def validar_drills(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con el plan de drills.")
    drills = datos.get("drills")
    if not isinstance(drills, list):
        raise ErrorUso("'drills' debe ser una lista.")

    identificadores = {str(d.get("id")) for d in drills if isinstance(d, Mapping)}
    faltantes = [nombre for nombre in DRILLS_REQUERIDOS if nombre not in identificadores]
    problemas: list[str] = []
    mutantes: list[str] = []

    for drill in drills:
        if not isinstance(drill, Mapping):
            problemas.append("drill malformado")
            continue
        identificador = str(drill.get("id"))
        for campo in ("componente", "degradacion_esperada", "criterio_exito", "rollback", "estado"):
            if not drill.get(campo):
                problemas.append(f"{identificador}: falta {campo}")
        if drill.get("estado") not in ESTADOS_DRILL:
            problemas.append(f"{identificador}: estado desconocido {drill.get('estado')!r}")
        if drill.get("estado") == "EJECUTADO_AWS":
            problemas.append(f"{identificador}: EJECUTADO_AWS no es valido en I-140 LOCAL")
        accion = str(drill.get("accion", "")).lower()
        if any(prohibida in accion for prohibida in ACCIONES_PROHIBIDAS):
            mutantes.append(identificador)
        if drill.get("requiere_aws") and drill.get("datos_sinteticos") is not True:
            problemas.append(f"{identificador}: un drill con AWS exige datos sinteticos")

    return {
        "comando": "drills",
        "requeridos": len(DRILLS_REQUERIDOS),
        "presentes": len(identificadores),
        "faltantes": faltantes,
        "problemas": problemas,
        "drills_mutantes": mutantes,
        "estado": (
            "PLAN_VALIDO"
            if not faltantes and not problemas and not mutantes
            else "PLAN_RECHAZADO"
        ),
        "ejecucion_aws": "NOT_RUN",
        "advertencia": "El plan se valida en local; ningun drill se ejecuto contra AWS.",
    }


def inventario_desde_codigo(raiz: Path | None = None) -> dict[str, Any]:
    base = raiz or RAIZ / "infra" / "terraform"
    por_modulo: dict[str, int] = {}
    for archivo in sorted(base.rglob("*.tf")):
        texto = archivo.read_text(encoding="utf-8", errors="replace")
        recursos = len(re.findall(r'^resource\s+"aws_', texto, flags=re.MULTILINE))
        if not recursos:
            continue
        modulo = archivo.parent.name
        por_modulo[modulo] = por_modulo.get(modulo, 0) + recursos
    observabilidad = RAIZ / "sdd" / "_control" / "H08" / "MATRIZ_METRICAS_ALERTAS.md"
    return {
        "comando": "inventario",
        "estado": "INVENTARIO_DERIVADO" if por_modulo else "INVENTARIO_VACIO",
        "modulos": {k: por_modulo[k] for k in sorted(por_modulo)},
        "recursos_declarados": sum(por_modulo.values()),
        "existe_en_aws": "NOT_VERIFIED",
        "core_24x7": "segun inventario de I-120 (no se deduce del nombre)",
        "alarmas_documentadas": observabilidad.is_file(),
        "advertencia": "El inventario se deriva del codigo; un nombre de recurso no prueba existencia en AWS.",
    }


def validar_acceso(datos: Mapping[str, Any] | None) -> dict[str, Any]:
    if datos is None:
        raise ErrorUso("Se requiere --datos con la politica de acceso.")
    problemas: list[str] = []
    if datos.get("shell_root") is True:
        problemas.append("shell de root prohibido")
    if datos.get("ssh_publico") is True:
        problemas.append("SSH publico prohibido")
    if datos.get("identidades_temporales") is not True:
        problemas.append("se exigen identidades temporales (SSO/OIDC)")
    if datos.get("claves_estaticas") is True:
        problemas.append("no se admiten claves de acceso estaticas")
    if datos.get("cambio_scp") is True:
        problemas.append("cambiar la SCP requiere RFC y Gate")
    if datos.get("ssm_solo_en_ventana") is not True:
        problemas.append("SSM debe limitarse a la ventana autorizada")
    return {
        "comando": "acceso",
        "estado": "ACCESO_CONFORME" if not problemas else "ACCESO_RECHAZADO",
        "problemas": problemas,
        "advertencia": "Politica declarativa; no se valido ningun acceso real.",
    }


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="verificar_operacion_i140.py",
        description="Verificacion operativa de I-140. Sin AWS.",
    )
    parser.add_argument("comando", help="runbooks | prioridades | drills | inventario | acceso")
    parser.add_argument("--datos", default=None)
    parser.add_argument("--runbook", default=str(RUNBOOK))
    parser.add_argument("--salida", default=None)
    return parser


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    comando = args.comando.strip().lower()
    datos = cargar_json(args.datos) if args.datos else None

    if comando == "runbooks":
        resultado = validar_runbooks(ruta=Path(args.runbook))
    elif comando == "prioridades":
        resultado = validar_prioridades(datos)
    elif comando == "drills":
        resultado = validar_drills(datos)
    elif comando == "inventario":
        resultado = inventario_desde_codigo()
    elif comando == "acceso":
        resultado = validar_acceso(datos)
    else:
        raise ErrorUso(
            f"Comando desconocido: {comando!r}. Solo existen: runbooks, prioridades, drills, "
            "inventario, acceso."
        )

    resultado["aws_consultado"] = False
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
    bloqueantes = (
        "RUNBOOKS_INCOMPLETOS",
        "PRIORIDADES_INCOMPLETAS",
        "PRIORIDADES_INCOHERENTES",
        "PLAN_RECHAZADO",
        "ACCESO_RECHAZADO",
    )
    return (1 if resultado["estado"] in bloqueantes else 0), resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except (ErrorUso, OSError) as exc:
        print(f"DENEGADO: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
