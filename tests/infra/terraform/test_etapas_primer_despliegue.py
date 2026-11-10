"""Contrato de despliegue declarativo en dos etapas, sin -target ni AWS."""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]
TF = (ROOT / "infra/terraform/envs/h08i/main.tf").read_text(encoding="utf-8")
VAR = (ROOT / "infra/terraform/envs/h08i/variables.tf").read_text(encoding="utf-8")
COMPUTO = (ROOT / "infra/terraform/modules/computo/main.tf").read_text(encoding="utf-8")

class PrimerDespliegue(unittest.TestCase):
    def test_interruptor_es_opt_in_para_etapa_base(self):
        self.assertIn('variable "desplegar_aplicacion" {', VAR)
        self.assertIn('default     = false', VAR)
        for nombre in ("computo", "balanceo", "observabilidad"):
            bloque = TF.split(f'module "{nombre}" {{',1)[1].split('\n}',1)[0]
            self.assertIn('count  = var.desplegar_aplicacion ? 1 : 0',bloque)
    def test_dos_ec2_no_se_relajan(self):
        self.assertIn('condition     = var.asg_capacidad == 2', COMPUTO)
        self.assertIn('min_size            = var.asg_capacidad', COMPUTO)
        self.assertIn('desired_capacity    = var.asg_capacidad', COMPUTO)
    def test_outputs_no_acceden_modulos_sin_instanciar(self):
        for nombre in ('computo','balanceo','observabilidad'):
            self.assertNotIn(f'module.{nombre}.', TF)
        self.assertIn('var.desplegar_aplicacion ? module.computo[0].asg_name : null', TF)
    def test_el_estado_base_no_crea_balanceador(self):
        self.assertIn('asg_name      = module.computo[0].asg_name', TF)
        self.assertIn('alb_arn          = module.balanceo[0].alb_arn', TF)
        self.assertIn('value = var.desplegar_aplicacion ? {', TF)

if __name__ == '__main__':
    unittest.main()
