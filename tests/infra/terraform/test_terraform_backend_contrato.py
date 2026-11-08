"""Pruebas estructurales de I-070 (bootstrap S3, locking nativo y condiciones IaC).

No consultan AWS ni ejecutan plan/apply. Verifican protecciones del state, el
mecanismo de locking declarado (S3 `use_lockfile`, sin DynamoDB) y el contrato IAM
de acceso mínimo al state y al fichero de lock. La aplicación real sigue NOT_RUN.
"""

from __future__ import annotations

import json
import re
import unittest
from pathlib import Path

from test_balanceo_contrato import _leer, _sin_comentarios, bloque, recursos

RAIZ = Path(__file__).resolve().parents[3]
BOOTSTRAP = RAIZ / "infra" / "terraform" / "bootstrap"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
POLICY = RAIZ / "infra" / "terraform" / "policies" / "terraform-state-access.json.tftpl"
LOCKFILE = "envs/h08i/terraform.tfstate.tflock"


def _render_politica() -> dict:
    texto = _leer(POLICY)
    texto = texto.replace("${state_bucket}", "nexum-h08i-tfstate-ejemplo123")
    texto = texto.replace("${state_key}", "envs/h08i/terraform.tfstate")
    return json.loads(texto)


LOCALES_AUTORIZADOS = frozenset({"terraform_data.validacion_bootstrap"})


def evaluar_recuperacion(
    cambios: list[dict],
    inventario_aws: set[str],
    locales_autorizados: frozenset[str] = LOCALES_AUTORIZADOS,
) -> list[str]:
    """Evalúa un plan de recuperación completa por dirección y proveedor.

    Modela la estructura de `terraform show -json` (`resource_changes[].address`,
    `provider_name`, `type`, `change.actions`, `change.importing.id`). No se ha
    ejecutado ningún plan real contra AWS: es un evaluador determinista.

    Política de denegación por defecto: sólo se admiten recursos AWS y los recursos
    locales expresamente autorizados (`terraform_data.validacion_bootstrap`).
    """
    problemas: list[str] = []
    if not isinstance(cambios, list):
        return ["PLAN_MALFORMADO no_es_lista"]

    direcciones: set[str] = set()
    importados: dict[tuple[str, str, str], str] = {}
    direcciones_importadas: set[str] = set()

    for cambio in cambios:
        if not isinstance(cambio, dict):
            problemas.append("PLAN_MALFORMADO entrada_no_es_objeto")
            continue
        direccion = cambio.get("address")
        tipo = cambio.get("type")
        acciones_raw = cambio.get("actions")
        if not isinstance(direccion, str) or not direccion or not isinstance(tipo, str) or not tipo \
                or not isinstance(acciones_raw, list):
            problemas.append("PLAN_MALFORMADO entrada_incompleta")
            continue
        proveedor = cambio.get("provider_name") or ""
        if not isinstance(proveedor, str):
            problemas.append("PLAN_MALFORMADO provider_no_es_texto")
            continue
        acciones = set(acciones_raw)

        if direccion in direcciones:
            problemas.append("DIRECCION_DUPLICADA " + direccion)
        direcciones.add(direccion)

        if tipo.startswith("aws_"):
            if proveedor and "hashicorp/aws" not in proveedor:
                problemas.append("PROVEEDOR_NO_AUTORIZADO " + direccion)
                continue
            if acciones == {"delete", "create"}:
                problemas.append("AWS_REPLACE " + direccion)
                continue
            if "create" in acciones:
                problemas.append("AWS_CREATE " + direccion)
            if "update" in acciones:
                problemas.append("AWS_UPDATE_UNAUTHORIZED " + direccion)
            if "delete" in acciones:
                problemas.append("AWS_DESTROY " + direccion)
            importacion = cambio.get("importing")
            if importacion is not None:
                if not isinstance(importacion, dict) or not importacion.get("id"):
                    problemas.append("PLAN_MALFORMADO importacion_sin_id " + direccion)
                    continue
                if direccion not in inventario_aws:
                    problemas.append("IMPORT_NO_PREVISTO " + direccion)
                # Un mismo identificador físico puede ser legítimo en tipos distintos
                # (p. ej. el nombre del bucket para aws_s3_bucket y sus sub-recursos).
                clave = (proveedor or "aws", tipo, str(importacion["id"]))
                if clave in importados and importados[clave] != direccion:
                    problemas.append("IMPORT_DUPLICADO " + tipo + " " + str(importacion["id"]))
                else:
                    importados[clave] = direccion
                direcciones_importadas.add(direccion)
        else:
            if direccion not in locales_autorizados:
                problemas.append("PROVEEDOR_NO_AUTORIZADO " + direccion)
                continue
            if "delete" in acciones and "create" not in acciones:
                problemas.append("LOCAL_DESTROY " + direccion)

    for direccion in sorted(inventario_aws - direcciones_importadas):
        problemas.append("IMPORT_FALTANTE " + direccion)
    return problemas



class BucketDeState(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.vars = _leer(BOOTSTRAP / "variables.tf")
        cls.outs = _leer(BOOTSTRAP / "outputs.tf")
        cls.r = recursos(cls.main)

    def test_state_cifrado_versionado_y_privado(self) -> None:
        self.assertIn(("aws_s3_bucket_versioning", "state"), self.r)
        self.assertIn('status = "Enabled"', self.main)
        self.assertIn('sse_algorithm = "AES256"', self.main)
        self.assertIn(("aws_s3_bucket_public_access_block", "state"), self.r)
        for flag in (
            "block_public_acls       = true",
            "block_public_policy     = true",
            "ignore_public_acls      = true",
            "restrict_public_buckets = true",
        ):
            self.assertIn(flag, self.main)

    def test_politica_exige_tls_y_ownership(self) -> None:
        self.assertIn(("aws_s3_bucket_policy", "state"), self.r)
        self.assertIn('"aws:SecureTransport" = "false"', self.main)
        self.assertIn('Effect    = "Deny"', self.main)
        self.assertIn('object_ownership = "BucketOwnerEnforced"', self.main)

    def test_state_no_se_destruye_por_ahorro(self) -> None:
        self.assertIn("prevent_destroy = true", self.r[("aws_s3_bucket", "state")])

    def test_lifecycle_acota_versiones(self) -> None:
        self.assertIn("noncurrent_version_expiration", self.main)
        self.assertIn("dias_retencion_versiones", self.main)


class LockingNativoS3(unittest.TestCase):
    """El locking con DynamoDB está deprecado: se exige S3 `use_lockfile`."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.outs = _leer(BOOTSTRAP / "outputs.tf")
        cls.example = _leer(ENV / "backend.tf.example")
        cls.env_main = _leer(ENV / "main.tf")
        cls.env_vars = _leer(ENV / "variables.tf")

    def test_no_se_crea_tabla_dynamodb_de_lock(self) -> None:
        tipos = {t for t, _ in recursos(self.main)}
        self.assertNotIn("aws_dynamodb_table", tipos)
        self.assertNotIn("LockID", self.main)

    def test_backend_usa_lockfile_y_no_dynamodb(self) -> None:
        self.assertIn("use_lockfile = true", self.example)
        self.assertNotIn("dynamodb_table", self.example)
        self.assertIn("encrypt      = true", self.example)

    def test_version_minima_permite_lock_nativo(self) -> None:
        self.assertIn('required_version = ">= 1.10.0, < 2.0.0"', self.env_main)
        self.assertIn('required_version = ">= 1.10.0, < 2.0.0"', self.main)
        self.assertIn("1.10", self.example)

    def test_outputs_declaran_mecanismo_y_archivo_de_lock(self) -> None:
        self.assertIn('mecanismo          = "S3 nativo (use_lockfile)"', self.outs)
        self.assertIn("terraform.tfstate.tflock", self.outs)
        self.assertIn("dynamodb_lock      = \"NO", self.outs)

    def test_backend_remoto_sigue_inactivo(self) -> None:
        self.assertNotRegex(self.env_main, r'backend\s+"s3"')
        self.assertIn("Sin bloque backend", self.env_main)
        self.assertTrue((ENV / "backend.tf.example").is_file())


class PoliticaDeAccesoAlState(unittest.TestCase):
    """Contrato IAM de mínimo privilegio para state y `.tflock` (diseño, no aplicado)."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.doc = _render_politica()
        cls.statements = {s["Sid"]: s for s in cls.doc["Statement"]}

    def test_listado_acotado_al_prefijo_exacto(self) -> None:
        st = self.statements["ListOnlyStatePrefix"]
        self.assertEqual(st["Action"], "s3:ListBucket")
        self.assertEqual(st["Resource"], "arn:aws:s3:::nexum-h08i-tfstate-ejemplo123")
        self.assertEqual(st["Condition"]["StringEquals"]["s3:prefix"], "envs/h08i/terraform.tfstate")

    def test_state_solo_lectura_y_escritura(self) -> None:
        st = self.statements["ReadWriteStateObjectOnly"]
        self.assertEqual(sorted(st["Action"]), ["s3:GetObject", "s3:PutObject"])
        self.assertTrue(st["Resource"].endswith("/envs/h08i/terraform.tfstate"))
        self.assertNotIn("s3:DeleteObject", st["Action"])

    def test_lock_permite_delete_solo_en_tflock(self) -> None:
        st = self.statements["LockFileLifecycleOnly"]
        self.assertIn("s3:DeleteObject", st["Action"])
        self.assertTrue(st["Resource"].endswith("/" + LOCKFILE))

    def test_delete_del_state_denegado_explicitamente(self) -> None:
        st = self.statements["DenyStateDeletion"]
        self.assertEqual(st["Effect"], "Deny")
        self.assertIn("s3:DeleteObject", st["Action"])
        self.assertIn("s3:DeleteObjectVersion", st["Action"])
        self.assertFalse(st["Resource"].endswith(".tflock"))

    def test_tls_obligatorio_en_los_allow(self) -> None:
        for sid, st in self.statements.items():
            if st["Effect"] != "Allow":
                continue
            self.assertEqual(st["Condition"]["Bool"]["aws:SecureTransport"], "true", sid)

    def test_sin_wildcards_ni_dynamodb(self) -> None:
        texto = _leer(POLICY)
        self.assertNotIn('"s3:*"', texto)
        self.assertNotIn("dynamodb:", texto)
        self.assertNotIn('arn:aws:s3:::*', texto)
        for st in self.doc["Statement"]:
            recursos_st = st["Resource"] if isinstance(st["Resource"], list) else [st["Resource"]]
            for r in recursos_st:
                self.assertTrue(r.startswith("arn:aws:s3:::nexum-h08i-tfstate-ejemplo123"), r)

    def test_backend_no_contiene_credenciales(self) -> None:
        texto = _leer(ENV / "backend.tf.example")
        for prohibido in ("access_key", "secret_key", "AWS_ACCESS_KEY_ID", "token ="):
            self.assertNotIn(prohibido, texto)


class CondicionesDeSeguridad(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.vars = _leer(BOOTSTRAP / "variables.tf")

    def test_sin_auto_approve_ni_credenciales(self) -> None:
        texto = self.main + self.vars
        self.assertNotIn("auto-approve", texto)
        self.assertNotRegex(texto, r"AKIA[0-9A-Z]{16}")
        self.assertNotIn("aws_access_key", texto)

    def test_sufijo_de_cuenta_obligatorio(self) -> None:
        self.assertIn('startswith(var.sufijo_cuenta, "PENDIENTE")', self.main)
        self.assertIn('default     = ""', self.vars.split('variable "sufijo_cuenta"')[1].split("}")[0])

    def test_region_fija(self) -> None:
        self.assertIn('var.region == "us-east-2"', self.main)

    def test_no_crea_recursos_de_otras_specs(self) -> None:
        tipos = {t for t, _ in recursos(self.main)}
        for prohibido in (
            "aws_vpc",
            "aws_instance",
            "aws_autoscaling_group",
            "aws_lb",
            "aws_cloudfront_distribution",
            "aws_elasticache_replication_group",
            "aws_iam_role",
            "aws_kms_key",
        ):
            self.assertNotIn(prohibido, tipos)


class DocumentosObligatorios(unittest.TestCase):
    def test_entregables_de_i070_existen(self) -> None:
        for nombre in ("BLUEPRINT_MODULOS_TF.md", "MATRIZ_RECURSOS_STATE.md", "RUNBOOK_IAC.md"):
            ruta = RAIZ / "sdd" / "_control" / "H08" / nombre
            self.assertTrue(ruta.is_file(), nombre)
            self.assertGreater(ruta.stat().st_size, 500, nombre)

    def test_runbook_cubre_lock_huerfano_y_recuperacion(self) -> None:
        texto = _leer(RAIZ / "sdd" / "_control" / "H08" / "RUNBOOK_IAC.md")
        self.assertIn("tflock", texto)
        self.assertIn("force-unlock", texto)
        self.assertIn("prevent_destroy", texto)
        self.assertIn("auto-approve", texto)



class RecuperacionDelStateDeBootstrap(unittest.TestCase):
    """Custodia y recuperación del state PROPIO del bootstrap (INF-070-02/04)."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.runbook = _leer(RAIZ / "sdd" / "_control" / "H08" / "RUNBOOK_IAC.md")
        cls.blueprint = _leer(RAIZ / "sdd" / "_control" / "H08" / "BLUEPRINT_MODULOS_TF.md")
        cls.bootstrap_main = _leer(BOOTSTRAP / "main.tf")
        cls.gitignore = _leer(RAIZ / ".gitignore")

    def test_procedimiento_de_custodia_y_migracion_existe(self) -> None:
        for ancla in (
            "Custodia del state local del bootstrap",
            "Migración del state de bootstrap a backend persistente",
            "Recuperación si se pierde el state del bootstrap",
            "Migración del state",
        ):
            self.assertIn(ancla, self.runbook)

    def test_states_separados_por_clave_distinta(self) -> None:
        self.assertIn("bootstrap/terraform.tfstate", self.runbook)
        self.assertIn("envs/h08i/terraform.tfstate", self.runbook)
        self.assertIn('key          = "envs/h08i/terraform.tfstate"', _leer(ENV / "backend.tf.example"))
        # El bootstrap no lleva backend hasta que el bucket existe (sin circularidad).
        self.assertNotRegex(self.bootstrap_main, r'backend\s+"s3"')
        self.assertIn("dependencia\ncircular", self.runbook.replace("  ", " "))

    def test_state_nunca_en_git_ni_en_artifacts(self) -> None:
        self.assertIn("*.tfstate", self.gitignore)
        self.assertIn("*.tfstate.*", self.gitignore)
        self.assertIn("GitHub Actions artifacts", self.runbook)
        self.assertIn("no adjuntar", self.runbook.lower())
        self.assertNotIn("terraform.tfstate\")", self.runbook)

    def test_migracion_requiere_autorizacion_y_no_es_automatica(self) -> None:
        for prohibido in ("-auto-approve",):
            self.assertIn(prohibido, self.runbook)
        self.assertIn("init -migrate-state", self.runbook)
        self.assertIn("autorización humana", self.runbook)
        self.assertNotIn("terraform apply -auto-approve", self.runbook)

    def test_recuperacion_por_import_sin_recrear(self) -> None:
        self.assertIn("importación", self.runbook)
        self.assertIn("nunca por recreación", self.runbook)
        # El plan de importación NO se confunde con un plan normal sin cambios ni se evalúa por contadores globales.
        self.assertIn("por dirección y por proveedor", self.runbook)
        self.assertIn("contadores globales del resumen", self.runbook)
        self.assertIn("AWS_CREATE", self.runbook)
        self.assertIn("AWS_UPDATE_UNAUTHORIZED", self.runbook)
        self.assertIn("AWS_REPLACE", self.runbook)
        self.assertIn("AWS_DESTROY", self.runbook)
        self.assertIn("no** es creación de", self.runbook)
        self.assertIn("infraestructura AWS", self.runbook)
        self.assertIn("Spec = \"I-070-terraform\"", self.runbook)
        self.assertIn("una única dirección del mismo tipo", self.runbook)
        self.assertIn("identificador compartido entre tipos distintos es legítimo", self.runbook)
        self.assertIn("IMPORT_FALTANTE", self.runbook)
        self.assertIn("PROVEEDOR_NO_AUTORIZADO", self.runbook)
        self.assertIn("recuperación **completa**", self.runbook)

    def test_state_original_no_se_mueve_antes_de_migrar(self) -> None:
        # El único state operativo debe seguir disponible para init -migrate-state.
        self.assertIn("permanece en su ubicación operativa", self.runbook)
        self.assertIn("lo que se duplica es la **copia de seguridad**", self.runbook.lower())
        self.assertNotIn("Mover el archivo a un almacén cifrado", self.runbook)

    def test_respaldo_cifrado_con_verificacion_de_integridad(self) -> None:
        self.assertIn("copia de seguridad **cifrada fuera del repositorio**", self.runbook)
        self.assertIn("SHA-256", self.runbook)
        self.assertIn("verificar la integridad", self.runbook)
        self.assertIn("lineage", self.runbook)
        self.assertIn("serial", self.runbook)
        # No se exige un serial concreto: una migración no tiene por qué incrementarlo.
        self.assertIn("No se asume que una migración incremente", self.runbook)

    def test_inventario_coincide_con_los_recursos_declarados(self) -> None:
        direcciones = sorted(f"{t}.{n}" for t, n in recursos(self.bootstrap_main))
        self.assertEqual(len(direcciones), 8, direcciones)
        self.assertIn("terraform_data.validacion_bootstrap", direcciones)
        for direccion in direcciones:
            self.assertIn(direccion, self.runbook)
        # El inventario es declarativo: se rechaza una cifra histórica fija.
        self.assertIn("no de una cifra\nhistórica", self.runbook)
        self.assertNotIn("los 7 recursos del bootstrap", self.runbook)

    def test_recursos_aws_vs_terraform_data_distinguidos(self) -> None:
        self.assertIn("7 recursos AWS", self.runbook)
        self.assertIn("1 recurso local\n`terraform_data`", self.runbook)

    def test_preserva_use_lockfile_y_no_anade_dynamodb(self) -> None:
        self.assertIn("use_lockfile", self.runbook)
        self.assertNotIn("dynamodb_table", self.runbook)
        tipos = {t for t, _ in recursos(self.bootstrap_main)}
        self.assertNotIn("aws_dynamodb_table", tipos)
        blueprint = _leer(RAIZ / "sdd" / "_control" / "H08" / "BLUEPRINT_MODULOS_TF.md")
        self.assertIn("No hay tabla DynamoDB de lock", blueprint)
        self.assertNotIn("aws_dynamodb_table", blueprint)

    def test_politica_iam_de_state_sigue_vigente(self) -> None:
        texto = _leer(POLICY)
        self.assertIn("tflock", texto)
        self.assertIn("DenyStateDeletion", texto)
        st = {s["Sid"]: s for s in _render_politica()["Statement"]}
        self.assertEqual(sorted(st["ReadWriteStateObjectOnly"]["Action"]), ["s3:GetObject", "s3:PutObject"])
        self.assertIn("s3:DeleteObject", st["LockFileLifecycleOnly"]["Action"])
        self.assertNotIn("s3:DeleteObject", st["ReadWriteStateObjectOnly"]["Action"])
        gate = _leer(RAIZ / "sdd" / "_control" / "H08" / "GATE_H08I_I070.md")
        self.assertIn("Permisos IAM de state/lock", gate)



class PlanDeRecuperacionPorDireccion(unittest.TestCase):
    """Criterio A-L sobre estructuras de plan sintéticas y representativas.

    No se ejecuta ningún plan real contra AWS: se modela el contenido que Terraform
    produciría (por dirección y proveedor) y se verifica la regla de aceptación.
    """

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(BOOTSTRAP / "main.tf")
        cls.runbook = _leer(RAIZ / "sdd" / "_control" / "H08" / "RUNBOOK_IAC.md")
        cls.direcciones = sorted(f"{t}.{n}" for t, n in recursos(cls.main))
        cls.local = [d for d in cls.direcciones if d.startswith("terraform_data.")]
        cls.aws = {d for d in cls.direcciones if d.startswith("aws_")}
        cls.aws_inventario = {d for d in cls.direcciones if d.startswith("aws_")}

    PROVEEDOR_AWS = "registry.terraform.io/hashicorp/aws"
    PROVEEDOR_LOCAL = "terraform.io/builtin/terraform"
    BUCKET_ID = "nexum-h08i-tfstate-ejemplo123"

    def _entrada(
        self,
        direccion: str,
        acciones: list[str],
        identificador: str | None = None,
        proveedor: str | None = None,
    ) -> dict:
        tipo = direccion.split(".")[0]
        cambio = {
            "address": direccion,
            "type": tipo,
            "actions": list(acciones),
            "provider_name": proveedor or (
                self.PROVEEDOR_AWS if tipo.startswith("aws_") else self.PROVEEDOR_LOCAL
            ),
        }
        if identificador is not None:
            cambio["importing"] = {"id": identificador}
        return cambio

    def _plan_aceptable(self) -> list[dict]:
        # En S3 el nombre del bucket identifica legítimamente al bucket y a sus
        # sub-recursos (versioning, SSE, PAB...): mismo id en tipos distintos.
        cambios = [self._entrada(d, ["no-op"], self.BUCKET_ID) for d in sorted(self.aws)]
        cambios += [self._entrada(d, ["create"]) for d in self.local]
        return cambios

    def _reemplazar(self, cambios: list[dict], direccion: str, acciones: list[str]) -> list[dict]:
        resultado = [c for c in cambios if c["address"] != direccion]
        resultado.append(self._entrada(direccion, acciones))
        return resultado

    def test_a_id_compartido_entre_tipos_aws_distintos_es_legitimo(self) -> None:
        plan = self._plan_aceptable()
        self.assertEqual(evaluar_recuperacion(plan, self.aws), [])
        s3 = [c for c in plan if c["address"] in ("aws_s3_bucket.state", "aws_s3_bucket_versioning.state")]
        self.assertEqual(len(s3), 2)
        self.assertEqual({c["importing"]["id"] for c in s3}, {self.BUCKET_ID})
        self.assertEqual({c["type"] for c in s3}, {"aws_s3_bucket", "aws_s3_bucket_versioning"})

    def test_b_mismo_recurso_en_dos_direcciones_del_mismo_tipo_se_rechaza(self) -> None:
        plan = self._plan_aceptable() + [
            self._entrada("aws_s3_bucket.state_alias", ["no-op"], self.BUCKET_ID)
        ]
        problemas = evaluar_recuperacion(plan, self.aws)
        self.assertTrue(any(p.startswith("IMPORT_DUPLICADO aws_s3_bucket ") for p in problemas), problemas)
        self.assertIn("IMPORT_NO_PREVISTO aws_s3_bucket.state_alias", problemas)

    def test_c_direccion_duplicada_se_rechaza(self) -> None:
        plan = self._plan_aceptable() + [self._entrada("aws_s3_bucket_policy.state", ["no-op"], "otro-id")]
        self.assertIn("DIRECCION_DUPLICADA aws_s3_bucket_policy.state",
                      evaluar_recuperacion(plan, self.aws))

    def test_d_proveedor_no_autorizado_se_rechaza(self) -> None:
        externo = self._entrada(
            "random_pet.sufijo", ["create"], proveedor="registry.terraform.io/hashicorp/random"
        )
        self.assertIn("PROVEEDOR_NO_AUTORIZADO random_pet.sufijo",
                      evaluar_recuperacion(self._plan_aceptable() + [externo], self.aws))
        # Denegación por defecto también para recursos locales no autorizados.
        local_extra = self._entrada("terraform_data.otro", ["create"])
        self.assertIn("PROVEEDOR_NO_AUTORIZADO terraform_data.otro",
                      evaluar_recuperacion(self._plan_aceptable() + [local_extra], self.aws))

    def test_e_import_faltante_se_rechaza(self) -> None:
        plan = [c for c in self._plan_aceptable() if c["address"] != "aws_s3_bucket_policy.state"]
        self.assertIn("IMPORT_FALTANTE aws_s3_bucket_policy.state",
                      evaluar_recuperacion(plan, self.aws))

    def test_f_plan_completo_con_terraform_data_es_aceptable(self) -> None:
        plan = self._plan_aceptable()
        self.assertEqual(sum(1 for c in plan if c["actions"] == ["create"]), 1)
        self.assertIn("terraform_data.validacion_bootstrap", {c["address"] for c in plan})
        self.assertEqual(evaluar_recuperacion(plan, self.aws), [])

    def test_g_creacion_aws_inesperada_se_rechaza(self) -> None:
        cambios = self._plan_aceptable() + [self._entrada("aws_s3_bucket_extra.state", ["create"])]
        self.assertIn("AWS_CREATE aws_s3_bucket_extra.state",
                      evaluar_recuperacion(cambios, self.aws))

    def test_h_reemplazo_destruccion_y_modificacion_se_rechazan(self) -> None:
        reemplazo = self._reemplazar(self._plan_aceptable(), "aws_s3_bucket.state", ["delete", "create"])
        self.assertIn("AWS_REPLACE aws_s3_bucket.state", evaluar_recuperacion(reemplazo, self.aws))
        destruccion = self._reemplazar(self._plan_aceptable(), "aws_s3_bucket_policy.state", ["delete"])
        self.assertIn("AWS_DESTROY aws_s3_bucket_policy.state", evaluar_recuperacion(destruccion, self.aws))
        modificacion = self._reemplazar(self._plan_aceptable(), "aws_s3_bucket_versioning.state", ["update"])
        self.assertIn("AWS_UPDATE_UNAUTHORIZED aws_s3_bucket_versioning.state",
                      evaluar_recuperacion(modificacion, self.aws))

    def test_i_eliminacion_de_terraform_data_se_rechaza(self) -> None:
        plan = [
            self._entrada("terraform_data.validacion_bootstrap", ["delete"])
            if c["address"] == "terraform_data.validacion_bootstrap" else c
            for c in self._plan_aceptable()
        ]
        self.assertIn("LOCAL_DESTROY terraform_data.validacion_bootstrap",
                      evaluar_recuperacion(plan, self.aws))

    def test_j_plan_malformado_falla_cerrado(self) -> None:
        self.assertEqual(evaluar_recuperacion(None, self.aws), ["PLAN_MALFORMADO no_es_lista"])
        self.assertIn("PLAN_MALFORMADO entrada_incompleta",
                      evaluar_recuperacion([{"address": "aws_s3_bucket.state"}], self.aws))
        sin_id = [c for c in self._plan_aceptable() if c["address"] != "aws_s3_bucket.state"]
        sin_id.append({"address": "aws_s3_bucket.state", "type": "aws_s3_bucket",
                       "actions": ["no-op"], "importing": {}})
        self.assertIn("PLAN_MALFORMADO importacion_sin_id aws_s3_bucket.state",
                      evaluar_recuperacion(sin_id, self.aws))

    def test_k_contadores_globales_no_bastan(self) -> None:
        aceptable = self._plan_aceptable()
        self.assertEqual(sum(1 for c in aceptable if c["actions"] == ["create"]), 1)
        self.assertEqual(evaluar_recuperacion(aceptable, self.aws), [])
        riesgoso = [c for c in aceptable if c["address"] != "terraform_data.validacion_bootstrap"]
        riesgoso.append(self._entrada("aws_s3_bucket_nuevo.state", ["create"]))
        self.assertEqual(sum(1 for c in riesgoso if c["actions"] == ["create"]), 1)
        self.assertIn("AWS_CREATE aws_s3_bucket_nuevo.state",
                      evaluar_recuperacion(riesgoso, self.aws))

    def test_l_import_no_previsto_se_rechaza(self) -> None:
        plan = self._plan_aceptable() + [
            self._entrada("aws_s3_bucket.intruso", ["no-op"], "id-intruso")
        ]
        self.assertIn("IMPORT_NO_PREVISTO aws_s3_bucket.intruso",
                      evaluar_recuperacion(plan, self.aws))

    def test_m_inventario_de_la_prueba_coincide_con_el_codigo(self) -> None:
        self.assertEqual(len(self.aws), 7, sorted(self.aws))
        self.assertEqual(len(self.local), 1, self.local)
        self.assertEqual(len(self.direcciones), 8, self.direcciones)
        self.assertIn("terraform_data.validacion_bootstrap", self.local)

    def test_n_state_original_y_respaldo_verificado(self) -> None:
        self.assertIn("permanece en su ubicación operativa", self.runbook)
        self.assertIn("init -migrate-state", self.runbook)
        self.assertIn("SHA-256", self.runbook)
        self.assertIn("verificar la integridad", self.runbook)
        self.assertIn("Sin copia íntegra no se migra", self.runbook)

    def test_o_locking_s3_sigue_activo(self) -> None:
        self.assertIn("use_lockfile", self.runbook)
        self.assertNotIn("dynamodb_table", self.runbook)

    def test_p_iam_no_introduce_permisos_globales(self) -> None:
        texto = _leer(POLICY)
        self.assertNotIn('"s3:*"', texto)
        self.assertNotIn("arn:aws:s3:::*", texto)
        self.assertNotIn("dynamodb:", texto)
        for st in _render_politica()["Statement"]:
            recursos_st = st["Resource"] if isinstance(st["Resource"], list) else [st["Resource"]]
            for recurso in recursos_st:
                self.assertTrue(recurso.startswith("arn:aws:s3:::nexum-h08i-tfstate-ejemplo123"), recurso)


if __name__ == "__main__":
    unittest.main()
