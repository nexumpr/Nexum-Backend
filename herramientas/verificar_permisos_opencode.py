"""Verificador estatico de la politica de permisos de OpenCode (sin ejecutar el motor).

Motivo: la politica de `opencode.json` se evalua por ultima coincidencia dentro de
`permissions` y por denegacion dura en `policies`. Un cambio de orden o una regla
anadida al final puede alterar el resultado sin que nada lo detecte.

Este verificador resuelve casos representativos contra el archivo REAL versionado
y falla cerrado. NO sustituye al motor de OpenCode: no puede demostrar que el
binario instalado aplique la misma semantica (eso exige prueba en la sesion).

Modos:
  --contrato-gobierno    suelo de seguridad permanente (debe pasar siempre).
  --objetivo-permanente  politica objetivo de RFC-017 (solo tras aplicarla).
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

DEFECTO = Path(__file__).resolve().parents[1] / "opencode.json"

# Suelo de seguridad: nunca puede concederse, con independencia de la fase.
GOBIERNO = {
    "deny": [
        ("shell", "git push origin main"),
        ("shell", "git push --force origin feat/x"),
        ("shell", "git push -f origin feat/x"),
        ("shell", "git reset --hard HEAD~1"),
        ("shell", "git clean -fdx"),
        ("shell", "terraform destroy -auto-approve"),
        ("shell", "terraform import aws_s3_bucket.x y"),
        ("shell", "terraform state rm aws_s3_bucket.x"),
        ("shell", "terraform init -migrate-state"),
        ("shell", "terraform init -reconfigure"),
        ("shell", "aws dynamodb scan --table-name NEXUM_CORE_H08I"),
        ("shell", "aws dynamodb delete-table --table-name NEXUM_CORE_H08I"),
        ("shell", "aws ec2 terminate-instances --instance-ids i-1"),
        ("shell", "aws s3api delete-object --bucket b --key k"),
        ("shell", "aws s3 rb s3://nexum-h08i-tfstate-713797602090 --force"),
        ("shell", "aws iam delete-role --role-name r"),
        ("shell", "docker run alpine"),
        ("shell", "wsl ls"),
        ("shell", "Remove-Item -Recurse -Force dist"),
        ("shell", "rm -rf dist"),
        ("read", ".env"),
        ("read", "infra/terraform/bootstrap/terraform.tfstate"),
        ("read", "secrets/prod.tfvars"),
        ("read", "clave.pem"),
        ("read", "id_rsa.key"),
        ("edit", "constitution.md"),
        ("edit", ".env"),
        ("edit", "infra/terraform/envs/h08i/terraform.tfvars"),
    ],
    "allow_solo_lectura": [
        ("shell", "git status --porcelain"),
        ("shell", "go build ./..."),
        ("shell", "go vet ./..."),
        ("shell", "go test ./... -count=1"),
        ("shell", "py -3.13 herramientas/validar_sdd_integral.py --check-git"),
    ],
}

# Politica objetivo de RFC-017 tras su aprobacion.
OBJETIVO = {
    "ask": [
        ("shell", "git add ."),
        ("shell", "git commit -m 'chore(x): y'"),
        ("shell", "git push origin feat/h08e-e005"),
        ("shell", "gh pr create --fill"),
        ("shell", "gh pr merge 65 --squash"),
        ("shell", "terraform plan -out plan.tfplan"),
        ("shell", "terraform apply plan.tfplan"),
        ("shell", "terraform show plan.tfplan"),
        ("shell", "aws s3api put-object --bucket b --key k --body f"),
        ("shell", "aws sts assume-role --role-arn arn:aws:iam::713797602090:role/x"),
        ("shell", "aws autoscaling update-auto-scaling-group --max-size 3"),
        ("shell", "aws autoscaling start-instance-refresh --auto-scaling-group-name a"),
        ("shell", "aws autoscaling set-instance-protection --instance-ids i-1"),
        ("shell", "aws ec2 create-launch-template --launch-template-name lt"),
        ("shell", "aws ec2 create-launch-template-version --launch-template-id lt-1"),
        ("shell", "aws iam create-role --role-name r --assume-role-policy-document d"),
        ("shell", "aws iam put-role-policy --role-name r --policy-name p --policy-document d"),
        ("shell", "aws iam get-role-policy --role-name r --policy-name p"),
        ("shell", "aws s3api list-objects-v2 --bucket b --prefix p"),
        ("shell", "aws dynamodb get-item --table-name NEXUM_CORE_H08I --key k"),
        ("shell", "aws dynamodb query --table-name NEXUM_CORE_H08I"),
        ("edit", "sdd/03-endpoints/E005-auth-password-forgot/spec.md"),
        ("edit", "sdd/03-endpoints/E005-auth-password-forgot/plan.md"),
        ("edit", "sdd/03-endpoints/openapi/nexum-api-v1.yaml"),
    ],
    "allow": [
        ("shell", "aws sts get-caller-identity"),
        ("shell", "aws ec2 describe-instances --filters x"),
        ("shell", "aws ec2 describe-launch-template-versions --launch-template-id lt-1"),
        ("shell", "aws autoscaling describe-auto-scaling-groups --auto-scaling-group-names a"),
        ("shell", "aws elbv2 describe-target-health --target-group-arn t"),
        ("shell", "aws ec2 describe-launch-templates --filters x"),
    ],
}


class Politica:
    def __init__(self, datos: dict):
        if not isinstance(datos, dict):
            raise ValueError("CONFIG_INVALIDA")
        self.permissions = datos.get("permissions") or []
        self.policies = datos.get("policies") or []
        if not isinstance(self.permissions, list) or not isinstance(self.policies, list):
            raise ValueError("CONFIG_INVALIDA_ARRAYS")
        for e in self.permissions + self.policies:
            if not isinstance(e, dict) or not {"action", "resource", "effect"} <= set(e):
                raise ValueError(f"ENTRADA_INVALIDA {e!r}")
            if e["effect"] not in ("allow", "ask", "deny"):
                raise ValueError(f"EFECTO_INVALIDO {e['effect']}")

    @staticmethod
    def coincide(patron: str, valor: str) -> bool:
        if patron == "*":
            return True
        regex = "^" + ".*".join(re.escape(p) for p in patron.split("*")) + "$"
        return re.match(regex, valor) is not None

    def resolver(self, action: str, resource: str, es_shell: bool = True) -> tuple[str, str]:
        efecto, regla = "deny", "sin-coincidencia (deny por defecto)"
        for e in self.permissions:
            if self.coincide(e["action"], action) and self.coincide(e["resource"], resource):
                efecto, regla = e["effect"], f"permission:{e['action']}:{e['resource']}"
        # Capa de politicas: solo puede endurecer.
        objetivo = f"{action}:{resource}"
        for e in self.policies:
            if e["effect"] != "deny":
                continue
            if self.coincide(e["resource"], objetivo):
                return "deny", f"policy:{e['resource']}"
        return efecto, regla

    def verificar(self, casos: dict, clave: str) -> list[str]:
        fallos = []
        for efecto, casos_accion in casos.items():
            if efecto == "allow_solo_lectura":
                efecto = "allow"
            for accion, recurso in casos_accion:
                obtenido, regla = self.resolver(accion, recurso)
                if obtenido != efecto:
                    fallos.append(f"{efecto.upper()} esperado, obtenido {obtenido.upper()}: {accion} {recurso} ({regla})")
        return [f"[{clave}] {f}" for f in fallos]


def verificar(ruta: Path, objetivo_permanente: bool) -> dict:
    datos = json.loads(ruta.read_text(encoding="utf-8"))
    pol = Politica(datos)
    fallos = []
    fallos += pol.verificar(GOBIERNO, "gobierno")
    casos = len(GOBIERNO["deny"]) + len(GOBIERNO["allow_solo_lectura"])
    if objetivo_permanente:
        fallos += pol.verificar(OBJETIVO, "objetivo")
        casos += sum(len(v) for v in OBJETIVO.values())
    return {
        "archivo": str(ruta),
        "permissions": len(pol.permissions),
        "policies": len(pol.policies),
        "casos": casos,
        "fallos": fallos,
        "resultado": "PERMISOS_PASS" if not fallos else "PERMISOS_FAIL",
    }


def cobertura(base: Path, objetivo: Path) -> dict:
    """Reglas de BASE ausentes o degradadas en OBJETIVO (reconciliacion de tres vias).

    Compara por (action, resource). Un cambio de efecto NO se considera perdida si el
    efecto nuevo es igual o mas restrictivo; se informa igualmente para que quede
    documentado en la RFC.
    """
    b = json.loads(base.read_text(encoding="utf-8"))
    o = json.loads(objetivo.read_text(encoding="utf-8"))
    clave = lambda e: (e["action"], e["resource"])
    objetivo_perm = {clave(e): e["effect"] for e in o.get("permissions") or []}
    objetivo_pol = {clave(e): e["effect"] for e in o.get("policies") or []}
    orden = {"deny": 0, "ask": 1, "allow": 2}
    ausentes, degradadas, endurecidas = [], [], []
    for e in b.get("permissions") or []:
        k = clave(e)
        if k not in objetivo_perm:
            ausentes.append(f"permission {e['action']} {e['resource']} ({e['effect']})")
        elif objetivo_perm[k] != e["effect"]:
            destino = "endurecidas" if orden[objetivo_perm[k]] < orden[e["effect"]] else "degradadas"
            (degradadas if destino == "degradadas" else endurecidas).append(
                f"permission {e['action']} {e['resource']}: {e['effect']} -> {objetivo_perm[k]}"
            )
    for e in b.get("policies") or []:
        k = clave(e)
        if k not in objetivo_pol:
            ausentes.append(f"policy {e['action']} {e['resource']} ({e['effect']})")
        elif objetivo_pol[k] != e["effect"]:
            destino = "endurecidas" if orden[objetivo_pol[k]] < orden[e["effect"]] else "degradadas"
            (degradadas if destino == "degradadas" else endurecidas).append(
                f"policy {e['action']} {e['resource']}: {e['effect']} -> {objetivo_pol[k]}"
            )
    return {"ausentes": ausentes, "degradadas": degradadas, "endurecidas": endurecidas}


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description="Verifica la politica de permisos de OpenCode (estatico).")
    p.add_argument("--archivo", default=None)
    p.add_argument("--objetivo-permanente", action="store_true")
    p.add_argument("--cobertura", default=None, help="Configuracion BASE a reconciliar contra --archivo.")
    p.add_argument("--json", default=None)
    a = p.parse_args(argv)
    ruta = Path(a.archivo) if a.archivo else DEFECTO
    try:
        if a.cobertura:
            c = cobertura(Path(a.cobertura), ruta)
            if a.json:
                Path(a.json).write_text(json.dumps(c, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
            print(
                f"COBERTURA: ausentes={len(c['ausentes'])} degradadas={len(c['degradadas'])} "
                f"endurecidas={len(c['endurecidas'])}"
            )
            for x in c["ausentes"]:
                print(" - AUSENTE", x)
            for x in c["degradadas"]:
                print(" - DEGRADADA", x)
            for x in c["endurecidas"]:
                print(" - ENDURECIDA", x)
            return 0
        r = verificar(ruta, a.objetivo_permanente)
    except (OSError, ValueError, json.JSONDecodeError) as ex:
        print(f"PERMISOS_ERROR {ex}")
        return 2
    if a.json:
        Path(a.json).write_text(json.dumps(r, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        f"{r['resultado']}: {r['permissions']} permissions, {r['policies']} policies, "
        f"{r['casos']} casos; archivo={r['archivo']}"
    )
    for f in r["fallos"][:40]:
        print(" -", f)
    return 0 if not r["fallos"] else 1


if __name__ == "__main__":
    sys.exit(main())
