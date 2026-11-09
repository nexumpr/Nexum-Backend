"""Hardening Report consolidado del CI (RFC-008, Etapa D).

Genera un reporte unico en Markdown y JSON a partir de los resultados reales de
los jobs y de los artefactos publicados. Se ejecuta SIEMPRE (`if: always()`), de
modo que produce evidencia incluso cuando un control falla; **no** convierte un
fallo en exito y **no** certifica controles: la autoridad es el Quality Gate.

Cada seccion se aisla: si un dato no esta disponible, se registra como
`NO_DISPONIBLE` con la causa, sin inventar valores.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[1]
CONTROL = RAIZ / "sdd" / "_control"

JOBS_ESPERADOS = (
    "detectar",
    "calidad",
    "seguridad",
    "secretos",
    "iac",
    "contratos",
    "cadena-suministro",
    "sast-semgrep",
    "integracion",
    "quality-gate",
)


def _sha256(ruta: Path) -> str:
    return hashlib.sha256(ruta.read_bytes()).hexdigest()


def _leer_json(ruta: Path) -> dict | None:
    try:
        return json.loads(ruta.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return None


def _artefacto(artefactos: Path, nombre: str, archivo: str) -> Path:
    return artefactos / nombre / archivo


def _seccion_cobertura(artefactos: Path, sha: str) -> dict:
    ruta = _artefacto(artefactos, f"cobertura-{sha}", "coverage.out")
    if not ruta.is_file():
        return {"estado": "NO_DISPONIBLE", "detalle": "coverage.out no publicado"}
    return {
        "estado": "DISPONIBLE",
        "bytes": ruta.stat().st_size,
        "sha256": _sha256(ruta),
        "umbral": "80 % del dominio (herramientas/cobertura_dominio.py)",
        "detalle": "el umbral se evalua en el job `calidad`; aqui solo se registra el artefacto",
    }


def _seccion_semgrep(artefactos: Path, sha: str) -> dict:
    ruta = _artefacto(artefactos, f"semgrep-{sha}", "semgrep.json")
    datos = _leer_json(ruta) if ruta.is_file() else None
    if datos is None:
        return {"estado": "NO_DISPONIBLE", "detalle": "semgrep.json no publicado"}
    resultados = datos.get("results") or []
    return {
        "estado": "DISPONIBLE",
        "hallazgos": len(resultados),
        "reglas": sorted({str(item.get("check_id", "")).split(".")[-1] for item in resultados}),
        "severidades": sorted({str((item.get("extra") or {}).get("severity", "")) for item in resultados}),
        "sha256": _sha256(ruta),
    }


def _seccion_iac(artefactos: Path, sha: str) -> dict:
    ruta = _artefacto(artefactos, f"iac-devsecops-{sha}", "iac-devsecops.json")
    datos = _leer_json(ruta)
    if datos is None:
        return {"estado": "NO_DISPONIBLE", "detalle": "iac-devsecops.json no publicado"}
    return {
        "estado": "DISPONIBLE",
        "hallazgos": datos.get("hallazgos") or datos.get("findings") or [],
        "sha256": _sha256(ruta),
    }


def _seccion_sbom(artefactos: Path, sha: str) -> dict:
    ruta = _artefacto(artefactos, f"sbom-spdx-{sha}", "sbom.spdx.json")
    if not ruta.is_file():
        return {"estado": "NO_DISPONIBLE", "detalle": "sbom.spdx.json no publicado"}
    return {"estado": "DISPONIBLE", "bytes": ruta.stat().st_size, "sha256": _sha256(ruta)}


def _seccion_mutacion(artefactos: Path, sha: str) -> dict:
    ruta = _artefacto(artefactos, f"mutacion-{sha}", "mutacion.json")
    datos = _leer_json(ruta)
    if datos is None:
        return {"estado": "NO_DISPONIBLE", "detalle": "mutacion.json no publicado"}
    return {
        "estado": datos.get("estado", "DESCONOCIDO"),
        "motivos": datos.get("motivos") or [],
        "metricas_publicadas": datos.get("metricas_publicadas", False),
        "detalle": datos.get("advertencia", ""),
    }


def _seccion_integracion(artefactos: Path, sha: str) -> dict:
    ruta = _artefacto(artefactos, f"integracion-{sha}", "integracion.txt")
    if not ruta.is_file():
        return {"estado": "NO_DISPONIBLE", "detalle": "integracion.txt no publicado"}
    texto = ruta.read_text(encoding="utf-8", errors="replace")
    return {
        "estado": "DISPONIBLE",
        "lineas": len(texto.splitlines()),
        "contiene_ok": "PASS" in texto or "ok " in texto,
        "sha256": _sha256(ruta),
    }


def _seccion_rfc() -> list[dict]:
    rfc006 = CONTROL / "H08" / "RFC-006-EXCEPCION_WAF_I030.md"
    rfc004 = CONTROL / "H08" / "RFC-004-SECRETOS_BOOTSTRAP.md"
    vigentes = [
        {
            "rfc": "RFC-004",
            "materia": "secretos de bootstrap",
            "estado": "PROPUESTO" if rfc004.is_file() else "NO_DISPONIBLE",
            "vigencia": "sin aprobacion: no habilita despliegue",
        }
    ]
    if rfc006.is_file():
        texto = rfc006.read_text(encoding="utf-8", errors="replace")
        vigencia = "2026-12-02" if "2026-12-02" in texto else "NO_DETECTADA"
        vigentes.append(
            {
                "rfc": "RFC-006",
                "materia": "excepcion WAF temporal (AWS-0011)",
                "estado": "APROBADA con caducidad" if "APROBADA" in texto else "DESCONOCIDO",
                "vigencia": vigencia,
            }
        )
    vigentes.append(
        {
            "rfc": "RFC-007",
            "materia": "notificacion de alarmas (SNS/KMS)",
            "estado": "PENDIENTE (sin documento)",
            "vigencia": "no aplica",
        }
    )
    return vigentes


def _seccion_sdd() -> dict:
    manifiesto = CONTROL / "H07" / "BASELINE_SHA256.json"
    datos = _leer_json(manifiesto)
    if datos is None:
        return {"estado": "NO_DISPONIBLE"}
    return {
        "estado": "MANIFIESTO_DISPONIBLE",
        "entradas": len(datos.get("files") or {}),
        "sha256_manifiesto": _sha256(manifiesto),
        "autoridad": "workflow `sdd-validacion.yml` (job `validar`); no se duplica aqui",
    }


def construir(needs: dict, perfil: dict, artefactos: Path, sha: str, ref: str, pr: str) -> dict:
    jobs = []
    for job in JOBS_ESPERADOS:
        datos = needs.get(job) if isinstance(needs.get(job), dict) else {}
        jobs.append(
            {
                "job": job,
                "resultado": datos.get("result", "NO_DISPONIBLE"),
                "exigido": (
                    "si"
                    if job in ("detectar", "secretos", "quality-gate")
                    else ("segun perfil" if any(perfil.values()) else "no aplica")
                ),
            }
        )

    omitidos = [
        {"job": item["job"], "motivo": item["resultado"]}
        for item in jobs
        if item["resultado"] == "skipped"
    ]

    return {
        "commit": sha,
        "ref": ref,
        "pull_request": pr or "NO_DISPONIBLE",
        "perfil_de_cambios": {clave: bool(valor) for clave, valor in perfil.items()},
        "jobs": jobs,
        "omitidos": omitidos,
        "cobertura": _seccion_cobertura(artefactos, sha),
        "sast": _seccion_semgrep(artefactos, sha),
        "sca": {
            "estado": "EJECUTADO_EN_JOB",
            "detalle": "go mod verify, govulncheck, gosec y staticcheck se ejecutan en los jobs `seguridad` y `calidad`; el resultado queda en el log del job y se refleja arriba",
        },
        "secretos": {
            "estado": "EJECUTADO_EN_JOB",
            "detalle": "gitleaks con baseline y reglas acotadas en el job `secretos`",
        },
        "mutacion": _seccion_mutacion(artefactos, sha),
        "integracion": _seccion_integracion(artefactos, sha),
        "contratos": {
            "estado": "EJECUTADO_EN_JOB",
            "detalle": "validar_h06.py (75 operaciones) y comparar_contrato_api.py contra origin/main en el job `contratos`",
        },
        "iac": _seccion_iac(artefactos, sha),
        "sbom": _seccion_sbom(artefactos, sha),
        "fuzzing_y_api_security": {
            "estado": "EJECUTADO_EN_JOB",
            "detalle": "fuzzing semilla en `calidad`; pruebas adversariales de autenticacion, idempotencia, CORS y cabeceras en `calidad` y `integracion`",
        },
        "dast": {
            "estado": "NOT_RUN",
            "motivo": (
                "no existe entorno autorizado: el servidor local no expone dependencias "
                "reales y el DAST sobre AWS requiere Gate de despliegue"
            ),
        },
        "rfc_vigentes": _seccion_rfc(),
        "sdd": _seccion_sdd(),
        "riesgos_residuales": [
            "branch protection no disponible en el plan actual (BLOQUEADO_PLAN_GITHUB): el gate es logico, no una regla de GitHub",
            "readiness 503 por diseno hasta registrar dependencias reales",
            "RFC-004 PROPUESTO: el despliegue sigue bloqueado",
            "RFC-006 vence el 2026-12-02 o al aprobar el Gate AWS (lo que ocurra antes)",
            "RFC-007: entrega de alertas CloudWatch bloqueada",
            "mutation testing BLOCKED_TECNICO_MUTACION: sin metricas publicadas",
            "DAST NOT_RUN",
        ],
        "aws_consultado": False,
        "advertencia": (
            "Reporte de hardening de CI: no acredita despliegue, AWS, HTTP real, "
            "dos nodos ni aprobacion humana."
        ),
    }


def _markdown(reporte: dict) -> str:
    lineas = [
        "# Hardening Report — Nexum-Backend",
        "",
        f"- Commit: `{reporte['commit']}`",
        f"- Ref: `{reporte['ref']}`",
        f"- Pull request: {reporte['pull_request']}",
        f"- Perfil de cambios: `{json.dumps(reporte['perfil_de_cambios'], sort_keys=True)}`",
        "",
        "## Resultados por job",
        "",
        "| Job | Resultado | Exigido |",
        "|---|---|---|",
    ]
    lineas += [f"| `{item['job']}` | {item['resultado']} | {item['exigido']} |" for item in reporte["jobs"]]
    lineas += [
        "",
        "## Controles",
        "",
        f"- Cobertura: **{reporte['cobertura']['estado']}** — {reporte['cobertura'].get('detalle', '')}",
        f"- SAST Semgrep: **{reporte['sast']['estado']}** — hallazgos={reporte['sast'].get('hallazgos', 'n/d')}",
        f"- SCA: {reporte['sca']['detalle']}",
        f"- Secretos: {reporte['secretos']['detalle']}",
        f"- Mutation testing: **{reporte['mutacion']['estado']}** — {reporte['mutacion'].get('detalle', reporte['mutacion'].get('detalle', ''))}",
        f"- Integracion: **{reporte['integracion']['estado']}** — lineas={reporte['integracion'].get('lineas', 'n/d')}",
        f"- Contratos: {reporte['contratos']['detalle']}",
        f"- IaC: **{reporte['iac']['estado']}**",
        f"- SBOM: **{reporte['sbom']['estado']}** — sha256={reporte['sbom'].get('sha256', 'n/d')}",
        f"- Fuzzing y API security: {reporte['fuzzing_y_api_security']['detalle']}",
        f"- DAST: **{reporte['dast']['estado']}** — {reporte['dast']['motivo']}",
        "",
        "## RFC vigentes",
        "",
        "| RFC | Materia | Estado | Vigencia |",
        "|---|---|---|---|",
    ]
    lineas += [
        f"| {item['rfc']} | {item['materia']} | {item['estado']} | {item['vigencia']} |"
        for item in reporte["rfc_vigentes"]
    ]
    lineas += [
        "",
        "## SDD",
        "",
        f"- {reporte['sdd'].get('estado')} — entradas={reporte['sdd'].get('entradas', 'n/d')} — sha256={reporte['sdd'].get('sha256_manifiesto', 'n/d')}",
        f"- {reporte['sdd'].get('autoridad', '')}",
        "",
        "## Riesgos residuales",
        "",
    ]
    lineas += [f"- {riesgo}" for riesgo in reporte["riesgos_residuales"]]
    lineas += ["", "---", "", f"_{reporte['advertencia']}_", ""]
    return "\n".join(lineas)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Hardening Report del CI (RFC-008)")
    parser.add_argument("--needs", required=True)
    parser.add_argument("--perfil", required=True)
    parser.add_argument("--artefactos", required=True)
    parser.add_argument("--sha", required=True)
    parser.add_argument("--ref", default="")
    parser.add_argument("--pr", default="")
    parser.add_argument("--salida", required=True, help="Directorio de salida")
    parser.add_argument("--resumen", default="", help="Ruta de $GITHUB_STEP_SUMMARY")
    args = parser.parse_args(argv)

    salida = Path(args.salida)
    salida.mkdir(parents=True, exist_ok=True)

    try:
        needs = json.loads(Path(args.needs).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        needs = {}
    try:
        perfil = json.loads(Path(args.perfil).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        perfil = {}

    reporte = construir(needs, perfil, Path(args.artefactos), args.sha, args.ref, args.pr)
    (salida / "hardening-report.json").write_text(
        json.dumps(reporte, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    markdown = _markdown(reporte)
    (salida / "hardening-report.md").write_text(markdown + "\n", encoding="utf-8")

    print(markdown)
    if args.resumen:
        with Path(args.resumen).open("a", encoding="utf-8") as manejador:
            manejador.write(markdown + "\n")
    return 0


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
