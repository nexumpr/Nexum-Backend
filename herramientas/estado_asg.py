#!/usr/bin/env python3
"""Normaliza y valida el estado del ASG antes/después del rollout (I-090).

Recibe el JSON de `aws autoscaling describe-auto-scaling-groups` y emite un
resumen con max_size, versión del Launch Template e instancias InService.
Exige el número mínimo de nodos sanos configurado.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def resumir(datos: dict, minimo: int, nodos: int) -> tuple[dict, list[str]]:
    grupos = datos.get("AutoScalingGroups") or []
    if not grupos:
        return {}, ["ASG_AUSENTE"]
    grupo = grupos[0]
    instancias = [
        instancia for instancia in grupo.get("Instances") or []
        if instancia.get("LifecycleState") == "InService"
    ]
    problemas: list[str] = []
    if len(instancias) != nodos:
        problemas.append(f"NODOS_IN_SERVICE {len(instancias)} != {nodos}")
    if grupo.get("MinSize", 0) < minimo:
        problemas.append("MIN_SIZE_INSUFICIENTE")
    resumen = {
        "max_size": grupo.get("MaxSize"),
        "min_size": grupo.get("MinSize"),
        "launch_template_version": str((grupo.get("LaunchTemplate") or {}).get("Version", "")),
        "in_service": len(instancias),
        "instance_ids": [instancia.get("InstanceId") for instancia in instancias],
    }
    return resumen, problemas


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Estado del ASG (I-090)")
    parser.add_argument("asg_json", type=Path)
    parser.add_argument("--minimo", type=int, default=2)
    parser.add_argument("--nodos", type=int, default=2)
    parser.add_argument("--salida", type=Path, required=True)
    args = parser.parse_args(argv)
    if not args.asg_json.is_file():
        print("ESTADO_ASG_FAIL: no existe el JSON del ASG")
        return 1
    try:
        datos = json.loads(args.asg_json.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        print(f"ESTADO_ASG_FAIL: JSON invalido: {exc}")
        return 1
    resumen, problemas = resumir(datos, args.minimo, args.nodos)
    if problemas:
        print("ESTADO_ASG_FAIL")
        for problema in problemas:
            print(" -", problema)
        return 1
    args.salida.write_text(json.dumps(resumen, ensure_ascii=False), encoding="utf-8")
    print(json.dumps(resumen, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
