#!/usr/bin/env python3
"""Escaneo estático DevSecOps de IaC (I-100, INF-100-05).

Complementa a Trivy (escáner principal) con comprobaciones explícitas exigidas por
el SDD. No ejecuta Terraform ni consulta AWS.

Reglas:
- SG_PUBLICO: ingreso público (0.0.0.0/0 o ::/0) en un bloque **ingress** de
  `aws_security_group`, en `aws_security_group_rule` con `type = "ingress"` o en
  `aws_vpc_security_group_ingress_rule`. Nunca se confunde con egress.
- S3_SIN_CIFRADO: cada bucket debe tener un recurso de cifrado que lo referencie
  **dentro de su propio bloque** y con `sse_algorithm` válido.
- S3_CIFRADO_NO_VERIFICABLE: cifrado con expresión dinámica no asociable.
- IAM_WILDCARD: comodín `*` en Action/Resource escalar o en listas.
- FORCE_DESTROY, ACL_PUBLICA.
- TFSTATE_RASTREADO / GIT_CHECK_FAILED: si Git no puede verificar, falla cerrado.

Límite declarado: análisis textual con emparejamiento de bloques por llaves. No
resuelve variables, módulos externos ni expresiones dinámicas; cuando no puede
asociar una configuración reporta `*_NO_VERIFICABLE` en lugar de asumir seguridad.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

EXTENSIONES = (".tf", ".tftpl", ".json")
PATRON_RECURSO = re.compile(r'resource\s+"([^"]+)"\s+"([^"]+)"\s*\{')
PATRON_BUCKET_REF = re.compile(r'bucket\s*=\s*aws_s3_bucket\.([A-Za-z0-9_-]+)')
PATRON_SSE = re.compile(r'sse_algorithm\s*=\s*"([^"]+)"')
PATRON_CIDR = re.compile(
    r"(cidr_blocks|ipv6_cidr_blocks|cidr_ipv4|cidr_ipv6)\s*=\s*(\[[^\]]*\]|\"[^\"]*\")",
    re.S,
)
PATRON_IAM_LISTA = re.compile(r"[\"']?(Action|Resource|actions|resources)[\"']?\s*[:=]\s*(\[[^\]]*\])", re.S)
PATRON_IAM_ESCALAR = re.compile(r"[\"']?(Action|Resource|actions|resources)[\"']?\s*[:=]\s*[\"']\*[\"']")
PATRON_FORCE = re.compile(r"force_destroy\s*=\s*true")
PATRON_ACL = re.compile(r"acl\s*=\s*\"(public-read|public-read-write|authenticated-read)\"")


def _archivos(raiz: Path) -> list[Path]:
    return sorted(p for p in raiz.rglob("*") if p.is_file() and p.suffix in EXTENSIONES)


def _sin_comentarios(texto: str) -> str:
    texto = re.sub(r"/\*.*?\*/", "", texto, flags=re.S)
    texto = re.sub(r"(?m)^\s*#.*$", "", texto)
    texto = re.sub(r"(?m)^\s*//.*$", "", texto)
    return texto


def _recursos(texto: str) -> list[tuple[str, str, str, int]]:
    """Devuelve (tipo, nombre, cuerpo, posición) con emparejamiento de llaves."""
    recursos: list[tuple[str, str, str, int]] = []
    for coincidencia in PATRON_RECURSO.finditer(texto):
        inicio = coincidencia.end() - 1
        profundidad = 0
        fin = inicio
        en_cadena = False
        escape = False
        for posicion in range(inicio, len(texto)):
            caracter = texto[posicion]
            if en_cadena:
                if escape:
                    escape = False
                elif caracter == "\\":
                    escape = True
                elif caracter == '"':
                    en_cadena = False
                continue
            if caracter == '"':
                en_cadena = True
            elif caracter == "{":
                profundidad += 1
            elif caracter == "}":
                profundidad -= 1
                if profundidad == 0:
                    fin = posicion
                    break
        recursos.append((coincidencia.group(1), coincidencia.group(2), texto[inicio + 1:fin], coincidencia.start()))
    return recursos


def _cidr_publico(cuerpo: str) -> bool:
    for coincidencia in PATRON_CIDR.finditer(cuerpo):
        valor = coincidencia.group(2)
        if "0.0.0.0/0" in valor or "::/0" in valor:
            return True
    return False


def _es_ingreso(tipo: str, cuerpo: str) -> bool:
    if tipo == "aws_vpc_security_group_ingress_rule":
        return True
    if tipo == "aws_vpc_security_group_egress_rule":
        return False
    if tipo == "aws_security_group_rule":
        return bool(re.search(r'type\s*=\s*"ingress"', cuerpo))
    if tipo == "aws_security_group":
        return "ingress" in cuerpo
    return False


def _bloques_ingreso(cuerpo: str) -> list[str]:
    """Extrae sub-bloques `ingress { ... }` de un aws_security_group."""
    bloques: list[str] = []
    for coincidencia in re.finditer(r"ingress\s*\{", cuerpo):
        inicio = coincidencia.end() - 1
        profundidad = 0
        for posicion in range(inicio, len(cuerpo)):
            if cuerpo[posicion] == "{":
                profundidad += 1
            elif cuerpo[posicion] == "}":
                profundidad -= 1
                if profundidad == 0:
                    bloques.append(cuerpo[inicio + 1:posicion])
                    break
    return bloques


def _state_rastreado(raiz: Path) -> tuple[list[str], str | None]:
    try:
        salida = subprocess.run(
            ["git", "ls-files", "*.tfstate", "*.tfstate.*"],
            cwd=raiz, capture_output=True, text=True, timeout=30, check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        return [], f"{type(exc).__name__}: {exc}"
    if salida.returncode != 0:
        return [], (salida.stderr.strip() or f"git exit {salida.returncode}")
    return [linea for linea in salida.stdout.splitlines() if linea.strip()], None


def analizar(raiz: Path, comprobar_git: bool = True) -> tuple[list[dict], str | None]:
    hallazgos: list[dict] = []
    archivos = _archivos(raiz)
    buckets: list[tuple[str, Path, int]] = []
    cifrados: set[str] = set()
    cifrado_dinamico = False

    for archivo in archivos:
        texto = _sin_comentarios(archivo.read_text(encoding="utf-8", errors="replace"))
        for tipo, nombre, cuerpo, posicion in _recursos(texto):
            if tipo == "aws_s3_bucket":
                buckets.append((nombre, archivo, texto[:posicion].count("\n") + 1))
            elif tipo == "aws_s3_bucket_server_side_encryption_configuration":
                sse = PATRON_SSE.search(cuerpo)
                if not sse or sse.group(1) not in ("AES256", "aws:kms"):
                    continue
                referencias = {m.group(1) for m in PATRON_BUCKET_REF.finditer(cuerpo)}
                if referencias:
                    cifrados |= referencias
                else:
                    cifrado_dinamico = True
            elif _es_ingreso(tipo, cuerpo):
                cuerpos = _bloques_ingreso(cuerpo) if tipo == "aws_security_group" else [cuerpo]
                if any(_cidr_publico(bloque) for bloque in cuerpos):
                    hallazgos.append({
                        "regla": "SG_PUBLICO", "severidad": "HIGH",
                        "archivo": archivo.relative_to(raiz).as_posix(),
                        "linea": texto[:posicion].count("\n") + 1,
                    })
        for coincidencia in PATRON_IAM_LISTA.finditer(texto):
            if re.search(r"[\"']\*[\"']", coincidencia.group(2)):
                hallazgos.append({
                    "regla": "IAM_WILDCARD", "severidad": "HIGH",
                    "archivo": archivo.relative_to(raiz).as_posix(),
                    "linea": texto[:coincidencia.start()].count("\n") + 1,
                })
        for coincidencia in PATRON_IAM_ESCALAR.finditer(texto):
            hallazgos.append({
                "regla": "IAM_WILDCARD", "severidad": "HIGH",
                "archivo": archivo.relative_to(raiz).as_posix(),
                "linea": texto[:coincidencia.start()].count("\n") + 1,
            })
        for coincidencia in PATRON_FORCE.finditer(texto):
            hallazgos.append({
                "regla": "FORCE_DESTROY", "severidad": "HIGH",
                "archivo": archivo.relative_to(raiz).as_posix(),
                "linea": texto[:coincidencia.start()].count("\n") + 1,
            })
        for coincidencia in PATRON_ACL.finditer(texto):
            hallazgos.append({
                "regla": "ACL_PUBLICA", "severidad": "HIGH",
                "archivo": archivo.relative_to(raiz).as_posix(),
                "linea": texto[:coincidencia.start()].count("\n") + 1,
            })

    for nombre, archivo, linea in buckets:
        if nombre in cifrados:
            continue
        regla = "S3_CIFRADO_NO_VERIFICABLE" if cifrado_dinamico else "S3_SIN_CIFRADO"
        hallazgos.append({
            "regla": regla, "severidad": "HIGH",
            "archivo": archivo.relative_to(raiz).as_posix(), "linea": linea,
        })

    error_git = None
    if comprobar_git:
        rastreados, error_git = _state_rastreado(raiz)
        for ruta in rastreados:
            hallazgos.append({"regla": "TFSTATE_RASTREADO", "severidad": "CRITICAL", "archivo": ruta, "linea": 1})
        if error_git is not None:
            hallazgos.append({"regla": "GIT_CHECK_FAILED", "severidad": "CRITICAL", "archivo": ".", "linea": 1})
    return hallazgos, error_git


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Escaneo IaC DevSecOps (I-100)")
    parser.add_argument("--raiz", type=Path, default=Path("infra/terraform"))
    parser.add_argument("--sin-git", action="store_true")
    parser.add_argument("--json", type=Path, default=None)
    args = parser.parse_args(argv)
    raiz = args.raiz.resolve()
    if not raiz.is_dir():
        print(f"IAC_DEVSECOPS_FAIL: no existe {raiz}")
        return 1
    hallazgos, error_git = analizar(raiz, comprobar_git=not args.sin_git)
    if args.json is not None:
        args.json.write_text(
            json.dumps({"hallazgos": hallazgos, "git_error": error_git}, ensure_ascii=False, indent=2),
            encoding="utf-8",
        )
    if hallazgos:
        print("IAC_DEVSECOPS_FAIL")
        for hallazgo in hallazgos:
            print(f" - {hallazgo['severidad']} {hallazgo['regla']} {hallazgo['archivo']}:{hallazgo['linea']}")
        return 1
    print(f"IAC_DEVSECOPS_PASS: {len(_archivos(raiz))} archivos sin hallazgos")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
