"""Contrato estatico del respaldo y la restauracion de I-130."""

from __future__ import annotations

import json
import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "respaldo"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i" / "backup.tf"
INVENTARIO = RAIZ / "infra" / "terraform" / "respaldo" / "inventario_backup.json"
CONTROL = RAIZ / "sdd" / "_control" / "H08"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


class ModuloDeRespaldo(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.env = _leer(ENV)

    def test_no_crea_recursos_aws(self) -> None:
        self.assertNotIn('resource "aws_', self.main)
        self.assertNotIn('provider "aws"', self.main)

    def test_usa_terraform_data(self) -> None:
        self.assertIn('resource "terraform_data" "guardas_backup"', self.main)
        self.assertIn(">= 1.4.0", self.main)

    def test_pitr_obligatorio(self) -> None:
        bloque = self.vars.split('variable "pitr_habilitado"')[1].split("variable ")[0]
        self.assertIn("var.pitr_habilitado", bloque)
        self.assertIn("sin PITR", bloque)
        self.assertIn("condition     = var.pitr_habilitado", self.main)

    def test_state_exige_tres_condiciones(self) -> None:
        self.assertIn(
            "var.state_versionado && var.state_cifrado && var.state_proteccion_borrado",
            self.main,
        )
        for nombre in ("state_versionado", "state_cifrado", "state_proteccion_borrado"):
            self.assertIn(f'variable "{nombre}"', self.vars)

    def test_patron_de_restauracion_no_alcanza_la_tabla_activa(self) -> None:
        self.assertIn("patron_no_alcanza_tabla_activa", self.main)
        self.assertIn("!can(regex(var.patron_tabla_destino, var.tabla_origen))", self.main)
        self.assertIn("tabla nueva aislada", self.main)

    def test_retencion_limitada_a_pitr(self) -> None:
        bloque = self.vars.split('variable "retencion_dias"')[1].split("variable ")[0]
        self.assertIn("var.retencion_dias >= 1 && var.retencion_dias <= 35", bloque)
        self.assertIn("var.retencion_dias <= 35", self.main)

    def test_destroys_prohibidos_por_validacion_permanente(self) -> None:
        for nombre in ("permitir_destroy_datos", "permitir_borrado_copias"):
            bloque = self.vars.split(f'variable "{nombre}"')[1].split("variable ")[0]
            self.assertIn("default     = false", bloque)
            self.assertIn(f"var.{nombre} == false", bloque)
        self.assertIn("var.permitir_destroy_datos == false && var.permitir_borrado_copias == false", self.main)

    def test_arn_de_tabla_validado(self) -> None:
        bloque = self.vars.split('variable "tabla_arn"')[1].split("variable ")[0]
        self.assertIn("arn:aws:dynamodb:us-east-2", bloque)

    def test_outputs_sin_arn_de_backup_inexistente(self) -> None:
        self.assertIn('output "politica"', self.outs)
        self.assertIn("PROCEDIMIENTO_FUERA_DE_TERRAFORM", self.outs)
        self.assertIn("RESTORE_AWS_NO_VERIFICADO", self.outs)
        self.assertIn("PROHIBIDA", self.outs)
        self.assertNotIn("aws_dynamodb_backup", self.outs)

    def test_entorno_cablea_pitr_de_i040(self) -> None:
        self.assertIn('module "backup"', self.env)
        self.assertIn("pitr_habilitado = var.dynamodb_pitr_habilitado", self.env)
        self.assertIn("tabla_arn    = module.dynamodb.table_arn", self.env)
        self.assertRegex(self.env, r"permitir_destroy_datos\s+= false")
        self.assertRegex(self.env, r"permitir_borrado_copias\s+= false")


class InventarioDeRespaldo(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = json.loads(INVENTARIO.read_text(encoding="utf-8"))

    def test_objetivos_declarados_y_no_medidos(self) -> None:
        objetivos = self.inv["objetivos"]
        self.assertEqual(objetivos["rto_horas"], "4")
        self.assertEqual(objetivos["rpo_horas"], "24")
        self.assertFalse(objetivos["medido"])
        self.assertEqual(objetivos["estado"], "OBJETIVO_PROPUESTO_NO_MEDIDO")

    def test_restore_solo_a_tabla_nueva(self) -> None:
        restore = self.inv["restore"]
        self.assertTrue(restore["requiere_tabla_nueva"])
        self.assertTrue(restore["prohibido_sobreescribir"])
        self.assertEqual(restore["patron_tabla_destino"], "^NEXUM_CORE_RESTORE_[a-z0-9]{4,20}$")
        patron = re.compile(restore["patron_tabla_destino"])
        self.assertIsNotNone(patron.match("NEXUM_CORE_RESTORE_h08i"))
        self.assertIsNone(patron.match("NEXUM_CORE_h08i"))

    def test_activos_cubren_datos_state_y_secretos(self) -> None:
        nombres = {activo["nombre"] for activo in self.inv["activos"]}
        self.assertEqual(
            nombres,
            {"dynamodb-core", "dynamodb-backup-on-demand", "terraform-state", "secretos-bootstrap"},
        )
        for activo in self.inv["activos"]:
            self.assertIn("mecanismo", activo)
            self.assertIn("retencion_dias", activo)
            self.assertTrue(activo["protegido"])

    def test_retencion_dentro_del_limite_de_pitr(self) -> None:
        for activo in self.inv["activos"]:
            self.assertLessEqual(int(activo["retencion_dias"]), 35, activo["nombre"])

    def test_secretos_bloqueados_por_rfc004(self) -> None:
        activos = {a["nombre"]: a for a in self.inv["activos"]}
        self.assertEqual(activos["secretos-bootstrap"]["estado"], "BLOQUEADO_RFC004")

    def test_borrado_cuenta_sin_afirmaciones_falsas(self) -> None:
        borrado = self.inv["borrado_cuenta"]
        self.assertFalse(borrado["borrado_inmediato_backups"])
        self.assertFalse(borrado["afirma_borrado_inmutable"])


class DocumentosObligatorios(unittest.TestCase):
    def test_politica_backups(self) -> None:
        texto = _leer(CONTROL / "POLITICA_BACKUPS.md")
        for marcador in ("RPO", "RTO", "PITR", "retención", "NOT_RUN"):
            self.assertIn(marcador, texto)

    def test_pruebas_restore(self) -> None:
        texto = _leer(CONTROL / "PRUEBAS_RESTORE.md")
        self.assertIn("tabla nueva", texto)
        self.assertIn("RESTORE_AWS_NO_VERIFICADO", texto)
        self.assertIn("RTO", texto)

    def test_matriz_retencion(self) -> None:
        texto = _leer(CONTROL / "MATRIZ_RETENCION.md")
        self.assertIn("DynamoDB", texto)
        self.assertIn("state", texto)
        self.assertIn("borrado", texto.lower())

    def test_matriz_inf_130(self) -> None:
        texto = _leer(CONTROL / "MATRIZ_INF_130.md")
        for contrato in ("INF-130-01", "INF-130-02", "INF-130-03", "INF-130-04", "INF-130-05", "INF-130-06"):
            self.assertIn(contrato, texto)
        self.assertIn("RESTORE_AWS_NO_VERIFICADO", texto)


class AmpliacionV2(unittest.TestCase):
    """RTO/RPO, state, privacidad y FinOps documentados y verificables."""

    def test_output_de_reconfiguracion_tras_restore(self) -> None:
        outs = _leer(MODULO / "outputs.tf")
        bloque = outs.split('output "reconfiguracion_tras_restore"')[1]
        for campo in (
            "ttl_atributo", "streams_habilitado", "pitr_habilitado",
            "iam_politicas", "alarmas", "etiquetas", "escalamiento",
        ):
            self.assertIn(campo, bloque, campo)

    def test_matriz_rto_rpo_de_seis_capas(self) -> None:
        texto = _leer(CONTROL / "MATRIZ_RTO_RPO.md")
        for capa in (
            "Objetivo propuesto", "Objetivo ratificado", "Objetivo medido",
            "Capacidad teórica", "Simulación local", "Resultado real AWS",
        ):
            self.assertIn(capa, texto, capa)
        self.assertIn("RTO_RPO_PENDING_HUMAN_APPROVAL", texto)

    def test_runbook_de_trece_fases(self) -> None:
        texto = _leer(CONTROL / "RUNBOOK_RESTAURACION.md")
        for fase in (
            "Incidente", "Contención", "Pausa controlada", "Identificación del respaldo",
            "Verificación de integridad", "Revisión FinOps", "Gate humano",
            "Restore aislado", "Validación financiera", "Reconciliación",
            "Reanudación", "Monitoreo", "Cierre",
        ):
            self.assertIn(fase, texto, fase)
        self.assertIn("RESTORE_AWS_NOT_RUN", texto)

    def test_procedimiento_de_state_con_ocho_pasos(self) -> None:
        texto = _leer(CONTROL / "PROCEDIMIENTO_STATE_RECOVERY.md")
        for paso in (
            "Identificar versión exacta", "Verificar integridad", "Registrar aprobación",
            "Preservar el state actual", "Garantizar reversión",
            "Rechazar escrituras concurrentes", "No exponer secretos", "Gate de despliegue",
        ):
            self.assertIn(paso, texto, paso)
        self.assertIn("TERRAFORM_STATE_RECOVERY_AWS_NOT_VERIFIED", texto)

    def test_privacidad_siete_conceptos(self) -> None:
        texto = _leer(CONTROL / "POLITICA_PRIVACIDAD_BACKUPS.md")
        for concepto in (
            "Eliminación de cuenta solicitada", "Desactivación de acceso",
            "Eliminación de datos activos", "Expiración de copias",
            "Retención por obligación", "Eliminación definitiva",
            "Restauración",
        ):
            self.assertIn(concepto, texto, concepto)
        self.assertIn("CUENTA_ELIMINADA_REACTIVADA", texto)
        self.assertIn("SESION_REVOCADA_REACTIVADA", texto)

    def test_finops_2_vs_3_gb(self) -> None:
        texto = _leer(CONTROL / "MATRIZ_FINOPS_130.md")
        self.assertIn("2 GB", texto)
        self.assertIn("3 GB", texto)
        for estado in ("ESTIMADO", "PRESUPUESTADO", "VERIFICADO_EN_CUENTA", "CONSUMO_REAL"):
            self.assertIn(estado, texto, estado)
        self.assertIn("NOT_VERIFIED", texto)

if __name__ == "__main__":  # pragma: no cover
    unittest.main()
