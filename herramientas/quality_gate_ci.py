"""Quality Gate del CI (RFC-008, Etapa D).

Evalua el resultado REAL de cada job obligatorio a partir del contexto `needs` de
GitHub Actions y del perfil de cambios, y falla cerrado:

- `failure` o `cancelled` en cualquier job obligatorio -> FAIL.
- `skipped` en un job que el perfil exigia -> FAIL (omision injustificada).
- `skipped` justificado por el perfil -> aceptado y registrado.
- Resultado ausente o desconocido -> FAIL.
- Artefacto exigido por un job ejecutado y no publicado -> FAIL.

No sustituye autorizaciones humanas: solo certifica el estado de los controles.

Uso:
    python3 herramientas/quality_gate_ci.py \
        --needs needs.json --perfil perfil.json \
        --artefactos artefactos --sha "$GITHUB_SHA" \
        --json quality-gate.json --resumen "$GITHUB_STEP_SUMMARY"
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

# Jobs obligatorios y los perfiles que los exigen. Sin perfiles = siempre obligatorio.
DEPENDENCIAS: dict[str, tuple[str, ...]] = {
    "secretos": (),
    "calidad": ("go",),
    "seguridad": ("go",),
    "cadena-suministro": ("go",),
    "contratos": ("api", "sdd", "go"),
    "sast-semgrep": ("go", "sdd"),
    "integracion": ("go",),
    "iac": ("infra", "sdd"),
}

# Artefactos que cada job debe publicar cuando se ejecuta.
ARTEFACTOS: dict[str, tuple[tuple[str, str], ...]] = {
    "calidad": (
        ("cobertura-{sha}", "coverage.out"),
        ("binario-{sha}", "binario.sha256"),
        ("mutacion-{sha}", "mutacion.json"),
    ),
    "seguridad": (("dependencias-{sha}", "dependencias.txt"),),
    "cadena-suministro": (("sbom-spdx-{sha}", "sbom.spdx.json"),),
    "sast-semgrep": (("semgrep-{sha}", "semgrep.json"),),
    "integracion": (("integracion-{sha}", "integracion.txt"),),
    "mutacion": (("mutacion-{sha}", "mutacion.json"),),
    "iac": (("iac-devsecops-{sha}", "iac-devsecops.json"),),
}

EXIGIDOS = "success"


def _resultado(needs: dict, job: str) -> str | None:
    datos = needs.get(job)
    if not isinstance(datos, dict):
        return None
    resultado = datos.get("result")
    return resultado if isinstance(resultado, str) else None


def _exigido(job: str, perfil: dict[str, bool]) -> bool:
    marcas = DEPENDENCIAS[job]
    if not marcas:
        return True
    return any(bool(perfil.get(marca)) for marca in marcas)


def evaluar(needs: dict, perfil: dict, artefactos: Path, sha: str) -> dict:
    fallos: list[str] = []
    aceptados: list[str] = []
    omitidos: list[dict[str, str]] = []
    artefactos_vistos: list[str] = []

    detectar = _resultado(needs, "detectar")
    if detectar != EXIGIDOS:
        fallos.append(
            f"detectar: resultado '{detectar}', se exige 'success' "
            "(sin deteccion fiable no hay gate valido)"
        )

    for job in DEPENDENCIAS:
        resultado = _resultado(needs, job)
        requerido = _exigido(job, perfil)

        if resultado is None:
            fallos.append(f"{job}: sin resultado en el contexto needs (falla cerrado)")
            continue

        if resultado == EXIGIDOS:
            aceptados.append(job)
        elif resultado == "skipped":
            if requerido:
                fallos.append(
                    f"{job}: omitido injustificadamente (el perfil exigia ejecutarlo)"
                )
            else:
                omitidos.append({"job": job, "motivo": "perfil sin cambios aplicables"})
        elif resultado in ("failure", "cancelled"):
            fallos.append(f"{job}: resultado '{resultado}'")
        else:
            fallos.append(f"{job}: resultado desconocido '{resultado}' (falla cerrado)")
            continue

        if resultado == EXIGIDOS:
            for nombre, archivo in ARTEFACTOS.get(job, ()):
                directorio = artefactos / nombre.format(sha=sha)
                ruta = directorio / archivo
                if ruta.is_file():
                    artefactos_vistos.append(f"{directorio.name}/{archivo}")
                else:
                    fallos.append(
                        f"{job}: falta el artefacto exigido '{directorio.name}/{archivo}'"
                    )

    return {
        "comando": "quality-gate",
        "sha": sha,
        "perfil": {clave: bool(valor) for clave, valor in perfil.items()},
        "jobs_ok": sorted(aceptados),
        "jobs_omitidos_justificados": omitidos,
        "artefactos_verificados": sorted(artefactos_vistos),
        "fallos": fallos,
        "estado": "PASS" if not fallos else "FAIL",
        "aws_consultado": False,
        "advertencia": (
            "Certifica el estado de los controles de CI; no acredita despliegue, "
            "AWS, HTTP real ni aprobacion humana."
        ),
    }


def _resumen_markdown(reporte: dict) -> str:
    lineas = [
        "## PR Quality Gate",
        "",
        f"- Estado: **{reporte['estado']}**",
        f"- Commit: `{reporte['sha']}`",
        f"- Jobs correctos: {len(reporte['jobs_ok'])}",
        f"- Omitidos justificados: {len(reporte['jobs_omitidos_justificados'])}",
        f"- Artefactos verificados: {len(reporte['artefactos_verificados'])}",
        "",
    ]
    if reporte["fallos"]:
        lineas.append("### Fallos")
        lineas.extend(f"- {fallo}" for fallo in reporte["fallos"])
        lineas.append("")
    if reporte["jobs_omitidos_justificados"]:
        lineas.append("### Omitidos con justificacion")
        lineas.extend(
            f"- `{item['job']}`: {item['motivo']}"
            for item in reporte["jobs_omitidos_justificados"]
        )
        lineas.append("")
    return "\n".join(lineas)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Quality Gate del CI (RFC-008)")
    parser.add_argument("--needs", required=True, help="JSON con el objeto needs")
    parser.add_argument("--perfil", required=True, help="JSON con el perfil de cambios")
    parser.add_argument("--artefactos", required=True, help="Directorio de artefactos descargados")
    parser.add_argument("--sha", required=True)
    parser.add_argument("--json", default="", help="Ruta del reporte JSON")
    parser.add_argument("--resumen", default="", help="Ruta del resumen markdown")
    args = parser.parse_args(argv)

    try:
        needs = json.loads(Path(args.needs).read_text(encoding="utf-8"))
        perfil = json.loads(Path(args.perfil).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        print(f"::error::No se pudo leer el contexto del gate: {exc}", flush=True)
        return 1
    if not isinstance(needs, dict) or not isinstance(perfil, dict):
        print("::error::Contexto del gate malformado; falla cerrado.", flush=True)
        return 1

    reporte = evaluar(needs, perfil, Path(args.artefactos), args.sha)
    texto = json.dumps(reporte, ensure_ascii=False, indent=2, sort_keys=True)
    print(texto)

    if args.json:
        Path(args.json).write_text(texto + "\n", encoding="utf-8")
    if args.resumen:
        with Path(args.resumen).open("a", encoding="utf-8") as manejador:
            manejador.write(_resumen_markdown(reporte) + "\n")

    return 0 if reporte["estado"] == "PASS" else 1


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
