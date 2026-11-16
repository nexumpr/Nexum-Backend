"""Construye el `opencode.json` permanente (RFC-017) de forma determinista.

Uso (no instala nada: solo escribe el archivo de salida indicado):

    py -3.13 herramientas/construir_opencode_permanente.py \
        --base dist/tmp/opencode.HEAD.json \
        --salida dist/tmp/opencode.permanente.json

Reglas de diseno:
- Se parte de una base explicita (normalmente el blob versionado) y se aplican
  reemplazos EXACTOS. Cada ancla debe aparecer una sola vez: si el archivo cambia,
  el script falla en vez de producir una configuracion distinta en silencio.
- Las reglas nuevas se anaden al FINAL de `permissions` porque la ultima
  coincidencia prevalece frente a `aws *`, `aws * create-*`, `aws * put-*`, etc.
- `policies` mantiene su funcion de denegacion dura; no se convierte en allowlist.
- No contiene credenciales ni rutas de usuario.

La salida NO se instala automaticamente: aplicarla exige aprobacion humana
(`APRUEBO_RFC_017_PERMISOS_PERMANENTES`) y verificacion en el motor real.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

# (id, descripcion, texto exacto esperado, texto de reemplazo)
CAMBIOS: list[tuple[str, str, str, str]] = [
    ("T1", "spec.md protegido: DENY -> ASK",
     '      "resource": "sdd/**/spec.md",\n      "effect": "deny"',
     '      "resource": "sdd/**/spec.md",\n      "effect": "ask"'),
    ("T2", "plan.md protegido: DENY -> ASK",
     '      "resource": "sdd/**/plan.md",\n      "effect": "deny"',
     '      "resource": "sdd/**/plan.md",\n      "effect": "ask"'),
    ("T3", "openapi/** (YAML) protegido: DENY -> ASK",
     '      "resource": "sdd/03-endpoints/**/openapi/**",\n      "effect": "deny"',
     '      "resource": "sdd/03-endpoints/**/openapi/**",\n      "effect": "ask"'),
    ("T4", "openapi.* (JSON) protegido: DENY -> ASK",
     '      "resource": "sdd/03-endpoints/**/openapi.*",\n      "effect": "deny"',
     '      "resource": "sdd/03-endpoints/**/openapi.*",\n      "effect": "ask"'),
    # T5 retirado: `terraform apply*` sigue en DENY. Solo se permite ASK sobre un plan guardado.
    ("T6", "sts get-caller-identity: ASK -> ALLOW (lectura)",
     '      "resource": "aws sts get-caller-identity*",\n      "effect": "ask"',
     '      "resource": "aws sts get-caller-identity*",\n      "effect": "allow"'),
    ("T7", "ec2 describe-instances: ASK -> ALLOW (lectura)",
     '      "resource": "aws ec2 describe-instances*",\n      "effect": "ask"',
     '      "resource": "aws ec2 describe-instances*",\n      "effect": "allow"'),
    ("T8", "elbv2 describe-load-balancers: ASK -> ALLOW (lectura)",
     '      "resource": "aws elbv2 describe-load-balancers*",\n      "effect": "ask"',
     '      "resource": "aws elbv2 describe-load-balancers*",\n      "effect": "allow"'),
    ("T9", "elbv2 describe-target-groups: ASK -> ALLOW (lectura)",
     '      "resource": "aws elbv2 describe-target-groups*",\n      "effect": "ask"',
     '      "resource": "aws elbv2 describe-target-groups*",\n      "effect": "allow"'),
    ("T10", "autoscaling describe-auto-scaling-groups: ASK -> ALLOW (lectura)",
     '      "resource": "aws autoscaling describe-auto-scaling-groups*",\n      "effect": "ask"',
     '      "resource": "aws autoscaling describe-auto-scaling-groups*",\n      "effect": "allow"'),
    ("T11", "ec2 describe-instance-types: ASK -> ALLOW (lectura)",
     '      "resource": "aws ec2 describe-instance-types*",\n      "effect": "ask"',
     '      "resource": "aws ec2 describe-instance-types*",\n      "effect": "allow"'),
    ("T12", "retirar el DENY duro de policies que impedia el ASK de apply",
     '    {\n      "action": "tool.use",\n      "resource": "shell:terraform apply*",\n      "effect": "deny"\n    },\n',
     ""),
]

# Reglas permanentes nuevas (efecto, accion, recurso).
BLOQUE_PERMISSIONS = [
    # Apply SOLO sobre un plan guardado: nunca `terraform apply` generico.
    ("ask", "shell", "terraform apply *.tfplan"),
    ("ask", "shell", "terraform apply -input=false *.tfplan"),
    ("allow", "shell", "aws ec2 describe-launch-template-versions *"),
    ("allow", "shell", "aws ec2 describe-launch-templates *"),
    ("allow", "shell", "aws autoscaling describe-instance-refreshes *"),
    ("allow", "shell", "aws autoscaling describe-scaling-activities *"),
    ("allow", "shell", "aws elbv2 describe-target-health *"),
    ("allow", "shell", "aws ec2 describe-instance-type-offerings *"),
    ("ask", "shell", "aws s3api put-object *"),
    ("ask", "shell", "aws sts assume-role *"),
    ("ask", "shell", "aws autoscaling update-auto-scaling-group *"),
    ("ask", "shell", "aws autoscaling start-instance-refresh *"),
    ("ask", "shell", "aws autoscaling cancel-instance-refresh *"),
    ("ask", "shell", "aws autoscaling set-instance-protection *"),
    ("ask", "shell", "aws ec2 create-launch-template *"),
    ("ask", "shell", "aws ec2 create-launch-template-version *"),
    ("ask", "shell", "aws ec2 modify-launch-template *"),
    ("ask", "shell", "aws iam create-role *"),
    ("ask", "shell", "aws iam put-role-policy *"),
    ("ask", "shell", "aws iam get-role-policy *"),
    ("ask", "shell", "aws iam list-role-policies *"),
    ("ask", "shell", "aws iam create-instance-profile *"),
    ("ask", "shell", "aws iam add-role-to-instance-profile *"),
    ("ask", "shell", "aws iam get-instance-profile *"),
    ("ask", "shell", "aws dynamodb get-item *"),
    ("ask", "shell", "aws dynamodb query *"),
    ("deny", "shell", "aws ec2 terminate-instances *"),
    ("deny", "shell", "aws dynamodb delete-table *"),
    ("deny", "shell", "aws iam delete-*"),
    # Lecturas de diagnostico SES/Logs/SSM (sin mutacion), anadidas en el preflight.
    ("allow", "shell", "aws ses get-account*"),
    ("allow", "shell", "aws ses list-identities*"),
    ("allow", "shell", "aws sesv2 get-account*"),
    ("allow", "shell", "aws logs describe-log-groups*"),
    # SSM retirado: `get-parameter` admite --with-decryption. Se exige excepcion por operacion.
]

BLOQUE_POLICIES = [
    ("deny", "tool.use", "shell:aws * delete-*"),
    ("deny", "tool.use", "shell:aws ec2 terminate-instances *"),
    ("deny", "tool.use", "shell:aws dynamodb delete-table *"),
    ("deny", "tool.use", "shell:aws iam delete-*"),
    ("deny", "tool.use", "shell:git push -f *"),
]

# Conservadas de la configuracion local del usuario (rollouts E003/E004): no conceden
# permiso, pero retirarlas alteraria un comportamiento ya observado sin prueba.
BLOQUE_POLICIES_ALLOW = [
    ("allow", "tool.use", "shell:aws autoscaling describe-auto-scaling-groups *"),
    ("allow", "tool.use", "shell:aws autoscaling describe-instance-refreshes *"),
    ("allow", "tool.use", "shell:aws autoscaling describe-scaling-activities *"),
    ("allow", "tool.use", "shell:aws autoscaling update-auto-scaling-group *"),
    ("allow", "tool.use", "shell:aws autoscaling start-instance-refresh *"),
    ("allow", "tool.use", "shell:aws autoscaling cancel-instance-refresh *"),
    ("allow", "tool.use", "shell:aws autoscaling set-instance-protection *"),
    ("allow", "tool.use", "shell:aws elbv2 describe-target-health *"),
    ("allow", "tool.use", "shell:aws ec2 describe-instances *"),
    ("allow", "tool.use", "shell:aws ec2 describe-launch-template-versions *"),
]


class ErrorConstruccion(Exception):
    pass


# Variantes con espacio que duplican una regla mas general ya existente.
REDUNDANTES = {"aws ec2 describe-instances *", "aws autoscaling describe-auto-scaling-groups *"}


def entrada(accion: str, recurso: str, efecto: str) -> str:
    return (
        "    {\n"
        f'      "action": "{accion}",\n'
        f'      "resource": "{recurso}",\n'
        f'      "effect": "{efecto}"\n'
        "    }"
    )


def construir(texto: str) -> str:
    texto = texto.replace("\r\n", "\n")
    for cid, descripcion, viejo, nuevo in CAMBIOS:
        n = texto.count(viejo)
        if n != 1:
            raise ErrorConstruccion(f"ANCLA_NO_UNICA ({n}) {cid} {descripcion}")
        texto = texto.replace(viejo, nuevo, 1)

    datos = __import__("json").loads(texto)
    permisos = [e for e in datos.get("permissions", []) if e.get("resource") not in REDUNDANTES]
    politicas = datos.get("policies", [])

    def fusionar(lista, nuevas, mantener_ultima=True):
        indice = {(e["action"], e["resource"]): i for i, e in enumerate(lista)}
        for efecto, accion, recurso in nuevas:
            clave = (accion, recurso)
            entrada = {"action": accion, "resource": recurso, "effect": efecto}
            if clave in indice:
                lista[indice[clave]] = entrada
            else:
                indice[clave] = len(lista)
                lista.append(entrada)
        return lista

    permisos = fusionar(permisos, BLOQUE_PERMISSIONS)
    politicas = fusionar(politicas, BLOQUE_POLICIES + BLOQUE_POLICIES_ALLOW, mantener_ultima=True)
    datos["permissions"] = permisos
    datos["policies"] = politicas
    return __import__("json").dumps(datos, ensure_ascii=False, indent=2) + "\n"


def validar(texto: str) -> dict:
    datos = json.loads(texto)
    esperadas = {(e, a, r) for e, a, r in BLOQUE_PERMISSIONS}
    reales = {(x["effect"], x["action"], x["resource"]) for x in datos["permissions"]}
    faltan = esperadas - reales
    if faltan:
        raise ErrorConstruccion(f"REGLAS_NO_APLICADAS {sorted(faltan)}")
    esperadas_pol = {(e, a, r) for e, a, r in BLOQUE_POLICIES + BLOQUE_POLICIES_ALLOW}
    reales_pol = {(x["effect"], x["action"], x["resource"]) for x in datos["policies"]}
    faltan_pol = esperadas_pol - reales_pol
    if faltan_pol:
        raise ErrorConstruccion(f"POLITICAS_NO_APLICADAS {sorted(faltan_pol)}")
    return datos


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description="Construye la configuracion permanente de RFC-017.")
    p.add_argument("--base", required=True, help="JSON de partida (blob versionado).")
    p.add_argument("--salida", required=True, help="Ruta del candidato a escribir.")
    a = p.parse_args(argv)
    try:
        texto = construir(Path(a.base).read_text(encoding="utf-8"))
        datos = validar(texto)
    except (OSError, ValueError, ErrorConstruccion) as ex:
        print(f"CONSTRUCCION_ERROR {ex}")
        return 2
    Path(a.salida).parent.mkdir(parents=True, exist_ok=True)
    Path(a.salida).write_text(texto, encoding="utf-8", newline="\n")
    print(
        f"PERMANENTE_OK salida={a.salida} permissions={len(datos['permissions'])} "
        f"policies={len(datos['policies'])} cambios={len(CAMBIOS)} "
        f"nuevas_permissions={len(BLOQUE_PERMISSIONS)} nuevas_policies={len(BLOQUE_POLICIES)}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
