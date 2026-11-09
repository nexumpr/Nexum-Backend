"""Detector de cambios del CI (RFC-008, Etapa A).

Clasifica los archivos cambiados por un PR o un push y emite el perfil de jobs
aplicables. El diseno es **fail-closed**: si no se puede determinar el conjunto de
cambios, o si aparece una ruta que no se sabe clasificar, se activa `completo` y
todos los jobs se ejecutan.

Uso en CI:
    python3 herramientas/detectar_cambios_ci.py \
        --evento "$GITHUB_EVENT_NAME" --base "$BASE" --head "$HEAD" \
        --salida "$GITHUB_OUTPUT"

Uso en pruebas:
    python3 herramientas/detectar_cambios_ci.py --archivos lista.txt --json
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from pathlib import Path

CAMPOS = ("go", "api", "sdd", "infra", "completo")

# Prefijos y su perfil. El orden no importa: un archivo puede activar varios perfiles.
PREFIJOS: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("sdd/03-endpoints/", ("api", "sdd")),
    ("sdd/", ("sdd",)),
    ("cmd/", ("go",)),
    ("internal/", ("go",)),
    ("pkg/", ("go",)),
    ("tests/", ("go",)),
    ("infra/", ("infra",)),
    ("herramientas/", ("completo",)),
    (".github/", ("completo",)),
)

# Archivos exactos y su perfil.
EXACTOS: dict[str, tuple[str, ...]] = {
    "go.mod": ("go", "completo"),
    "go.sum": ("go", "completo"),
    ".trivyignore.yaml": ("completo",),
    ".semgrepignore": ("completo",),
    ".gitleaks.toml": ("completo",),
    ".gitleaks-baseline.json": ("completo",),
    "opencode.json": ("completo",),
    "AGENTS.md": ("completo",),
    "constitution.md": ("completo",),
}

# Documentacion: no activa jobs por si sola, pero tampoco se considera desconocida.
SUFIJOS_DOCUMENTACION = (".md", ".txt", ".rst")

# Un cambio en la superficie de seguridad obliga a ejecutar TODO: un PR que toca
# los workflows, el escaner de secretos o las dependencias no es "trivial" aunque
# el diff sea de una linea.
PERFIL_COMPLETO = {campo: True for campo in CAMPOS}


def _normalizar(ruta: str) -> str:
    return ruta.replace("\\", "/").strip().lstrip("./")


def clasificar(archivos: list[str]) -> dict[str, bool]:
    """Devuelve el perfil de jobs aplicables para el conjunto de archivos dado."""
    perfil = {campo: False for campo in CAMPOS}
    for ruta in archivos:
        limpia = _normalizar(ruta)
        if not limpia:
            continue

        conocido = False
        for prefijo, marcas in PREFIJOS:
            if limpia.startswith(prefijo):
                conocido = True
                for marca in marcas:
                    perfil[marca] = True

        if limpia in EXACTOS:
            conocido = True
            for marca in EXACTOS[limpia]:
                perfil[marca] = True

        if not conocido and not limpia.lower().endswith(SUFIJOS_DOCUMENTACION):
            # Ruta no clasificada: no se asume que sea inocua.
            perfil["completo"] = True

    if perfil["completo"]:
        return dict(PERFIL_COMPLETO)
    return perfil


def archivos_desde_git(evento: str, base: str, head: str, timeout: int = 60) -> list[str] | None:
    """Lista los archivos cambiados. Devuelve None si no se puede determinar."""
    if evento == "workflow_dispatch":
        return None

    intentos: list[list[str]] = []
    if evento == "pull_request":
        if base and head:
            intentos.append(["git", "diff", "--name-only", f"{base}...{head}"])
            intentos.append(["git", "diff", "--name-only", base, head])
    if head:
        intentos.append(["git", "show", "--pretty=", "--name-only", head])
    intentos.append(["git", "diff", "--name-only", "HEAD~1", "HEAD"])

    for comando in intentos:
        try:
            resultado = subprocess.run(
                comando, capture_output=True, text=True, timeout=timeout, check=True
            )
        except (OSError, subprocess.SubprocessError):
            continue
        return [linea for linea in resultado.stdout.splitlines() if linea.strip()]
    return None


def escribir_salidas(perfil: dict[str, bool], ruta_salida: Path) -> None:
    """Escribe las salidas en el formato de $GITHUB_OUTPUT."""
    lineas = [f"{campo}={str(perfil[campo]).lower()}" for campo in CAMPOS]
    with ruta_salida.open("a", encoding="utf-8") as manejador:
        manejador.write("\n".join(lineas) + "\n")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Detector de cambios del CI (RFC-008)")
    parser.add_argument("--evento", default="")
    parser.add_argument("--base", default="")
    parser.add_argument("--head", default="")
    parser.add_argument("--archivos", default="", help="Archivo con una ruta por linea (pruebas)")
    parser.add_argument("--salida", default="", help="Ruta de $GITHUB_OUTPUT")
    parser.add_argument("--json", action="store_true", help="Imprime el resultado en JSON")
    args = parser.parse_args(argv)

    motivo = "git"
    if args.archivos:
        try:
            listado = Path(args.archivos).read_text(encoding="utf-8").splitlines()
            motivo = "archivo"
        except OSError as exc:
            listado = None
            motivo = f"error-leyendo-archivo: {exc}"
    else:
        listado = archivos_desde_git(args.evento, args.base, args.head)
        if listado is None:
            motivo = "sin-diff-determinable"

    if listado is None:
        perfil = dict(PERFIL_COMPLETO)
        resultado = "COMPLETO_POR_DESCONOCIMIENTO"
    else:
        perfil = clasificar(listado)
        resultado = "PERFIL_CALCULADO"

    if args.salida:
        escribir_salidas(perfil, Path(args.salida))

    resumen = {
        "resultado": resultado,
        "motivo": motivo,
        "archivos": len(listado) if listado is not None else None,
        "perfil": perfil,
        "aws_consultado": False,
    }
    if args.json:
        print(json.dumps(resumen, ensure_ascii=False, indent=2, sort_keys=True))
    else:
        print(json.dumps(resumen, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
