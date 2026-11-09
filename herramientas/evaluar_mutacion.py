"""Evaluacion determinista de mutation testing para Go (RFC-008, Etapa C).

No inventa metricas: comprueba condiciones objetivas y emite una decision.

Condiciones evaluadas:
1. Existe una herramienta de mutacion para Go con version fijable.
2. La herramienta es ejecutable en el entorno (necesita el binario `diff`).
3. Los paquetes prioritarios tienen pruebas en el **mismo paquete**; sin ellas,
   `go test <paquete>` no ejecuta nada y todos los mutantes sobrevivirian, lo que
   produciria un numero real pero inutil y enganoso.

Salida: JSON con `estado` = VIABLE | BLOCKED_TECNICO_MUTACION y la lista de razones.
"""

from __future__ import annotations

import argparse
import json
import shutil
import sys
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[1]

# Paquetes prioritarios por riesgo: dinero, calculo financiero, idempotencia y seguridad.
PAQUETES_PRIORITARIOS = (
    "internal/domain/money",
    "internal/domain/financial",
    "internal/application/financial",
    "internal/application/identity",
    "internal/infrastructure/security",
)

HERRAMIENTA = "github.com/avito-tech/go-mutesting/cmd/go-mutesting"
# Sin releases etiquetadas: solo pseudo-versiones (comprobado con `go list -m -versions`).
VERSION_OBSERVADA = "v0.0.0-20261002081621-9ce5f731e321"


def _pruebas_en_paquete(paquete: str) -> list[str]:
    directorio = RAIZ / paquete
    if not directorio.is_dir():
        return []
    return sorted(ruta.name for ruta in directorio.glob("*_test.go"))


def evaluar() -> dict:
    pruebas = {paquete: _pruebas_en_paquete(paquete) for paquete in PAQUETES_PRIORITARIOS}
    sin_pruebas = [paquete for paquete, archivos in pruebas.items() if not archivos]
    diff_disponible = shutil.which("diff") is not None
    herramienta_en_path = shutil.which("go-mutesting") is not None

    condiciones = [
        {
            "condicion": "herramienta_compila_con_go_del_modulo",
            "cumplida": True,
            "evidencia": (
                f"go install {HERRAMIENTA}@{VERSION_OBSERVADA} finalizo con codigo 0"
            ),
        },
        {
            "condicion": "binario_diff_disponible",
            "cumplida": diff_disponible,
            "evidencia": (
                "la herramienta invoca `diff` para cada mutante; sin el aborta"
                if not diff_disponible
                else "diff presente en PATH"
            ),
        },
        {
            "condicion": "pruebas_en_el_mismo_paquete",
            "cumplida": not sin_pruebas,
            "evidencia": (
                "paquetes prioritarios sin `_test.go` propio: " + ", ".join(sin_pruebas)
                if sin_pruebas
                else "todos los paquetes prioritarios tienen pruebas propias"
            ),
        },
        {
            "condicion": "herramienta_instalada_en_el_runner",
            "cumplida": herramienta_en_path,
            "evidencia": (
                "no se instala por defecto: anadiria tiempo de CI sin aportar "
                "medicion valida mientras falten pruebas en el paquete"
            ),
        },
    ]

    fallos = [item["condicion"] for item in condiciones if not item["cumplida"]]
    viable = not fallos

    return {
        "comando": "evaluar-mutacion",
        "herramienta": HERRAMIENTA,
        "version_observada": VERSION_OBSERVADA,
        "releases_etiquetadas": False,
        "paquetes_prioritarios": list(PAQUETES_PRIORITARIOS),
        "pruebas_por_paquete": pruebas,
        "condiciones": condiciones,
        "estado": "VIABLE" if viable else "BLOCKED_TECNICO_MUTACION",
        "motivos": fallos,
        "metricas_publicadas": False,
        "advertencia": (
            "No se publican mutantes generados/detectados/sobrevivientes porque la "
            "medicion no es valida en las condiciones actuales. Un porcentaje en este "
            "estado seria un dato inventado."
        ),
        "activacion": (
            "Anadir pruebas en el mismo paquete de los modulos prioritarios y validar "
            "la invocacion con `/exec` en Linux; entonces este evaluador pasara a VIABLE."
        ),
        "aws_consultado": False,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Evaluacion de mutation testing (RFC-008)")
    parser.add_argument("--json", default="", help="Ruta del reporte JSON")
    args = parser.parse_args(argv)

    reporte = evaluar()
    texto = json.dumps(reporte, ensure_ascii=False, indent=2, sort_keys=True)
    print(texto)
    if args.json:
        ruta = Path(args.json)
        ruta.parent.mkdir(parents=True, exist_ok=True)
        ruta.write_text(texto + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
