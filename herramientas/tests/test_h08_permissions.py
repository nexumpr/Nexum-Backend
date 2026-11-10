import json, fnmatch, unittest
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]

class PermisosH08I(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  cls.c=json.loads((ROOT/'opencode.json').read_text(encoding='utf-8'))
  cls.rules=cls.c['permissions']; cls.policies=cls.c.get('policies',[])
 def eff(self, action, resource):
  effect='ask'
  for r in self.rules:
   if fnmatch.fnmatchcase(action,r['action']) and fnmatch.fnmatchcase(resource,r['resource']):effect=r['effect']
  for p in self.policies:
   if p['action']=='tool.use' and p['effect']=='deny' and fnmatch.fnmatchcase(action+':'+resource,p['resource']):effect='deny'
  return effect
 def test_agent_primary_has_shell_edit(self):
  p=(ROOT/'.opencode/agents/nexum-orchestrator.md').read_text(encoding='utf8')
  self.assertIn('permissions: []',p)
  self.assertNotIn('effect: deny',p.split('---',2)[1])
  self.assertEqual(self.c['default_agent'],'nexum-orchestrator')
 def test_code_and_governance_asks(self):
  for p in ['cmd/server/main.go','internal/ports/a.go','tests/infra/finops/test_a.py','infra/finops/preflight.ps1','opencode.json','AGENTS.md','.github/workflows/ci.yml','sdd/_control/H08/RFC-004.md','sdd/02-infraestructura/I-000-cuenta-finops/tasks.md']:
   self.assertEqual(self.eff('edit',p),'ask',p)
 def test_normative_and_secrets_denied(self):
  for p in ['constitution.md','sdd/02-infraestructura/I-000-cuenta-finops/spec.md','sdd/01-arquitectura-general/A-000-contexto-sistema/plan.md','sdd/03-endpoints/E001-auth/spec.md','.env','dir/.env.secret','a.tfstate','a.tfvars','keys.pem','secrets.key']:
   self.assertEqual(self.eff('edit',p),'deny',p)
  for p in ['.env','a.tfstate','a.tfvars','private.pem']:
   self.assertEqual(self.eff('read',p),'deny',p)
 def test_git_cli_and_shell_enabled(self):
  for cmd in ['gh pr create --title x','git push origin feat/h08i-i000-finops','git add infra/finops/a.py','git commit -m msg']:
   self.assertEqual(self.eff('shell',cmd),'ask',cmd)
  self.assertEqual(self.eff('shell','go test ./...'),'allow')
  self.assertEqual(self.eff('shell','git status -sb'),'allow')
 def test_cloud_deny_readonly_ask(self):
  for cmd in ['aws ec2 run-instances --x','aws ec2 create-vpc --x','aws budgets create-budget --x','terraform apply -auto-approve','terraform init -backend=true','terraform plan','terraform destroy','git push origin main','git reset --hard HEAD','docker build .']:
   self.assertEqual(self.eff('shell',cmd),'deny',cmd)
  for cmd in ['aws sts get-caller-identity --profile nexum','aws freetier get-account-plan-state --region us-east-2','aws pricing get-products --service-code AmazonEC2','terraform fmt -check']:
   self.assertEqual(self.eff('shell',cmd),'ask',cmd)
 def test_specialists_and_skills(self):
  for sub in ['finops-guardian','aws-terraform-engineer','qa-evidence-verifier']:
   self.assertEqual(self.eff('subagent',sub),'ask')
  for skill in ['assess-aws-cost','validate-terraform','read-sdd']:
   self.assertEqual(self.eff('skill',skill),'ask')
  self.assertEqual(self.eff('external_directory','C:/Users/leoba/.aws/credentials'),'deny')
 def test_all_eight_profile_modes(self):
  agents=list((ROOT/'.opencode/agents').glob('*.md'))
  self.assertEqual(len(agents),8)
  for p in agents:
   txt=p.read_text(encoding='utf8'); self.assertTrue(txt.startswith('---\n'));self.assertIn('permissions:',txt)


class TerraformGateTests(unittest.TestCase):
    """Negativos del ajuste de permisos de I-010 (init local sin backend)."""

    @classmethod
    def setUpClass(cls) -> None:
        import json as _json
        from pathlib import Path as _Path

        raiz = _Path(__file__).resolve().parents[2]
        cfg = _json.loads((raiz / "opencode.json").read_text(encoding="utf-8"))
        cls.politicas = {(p.get("resource"), p.get("effect")) for p in cfg.get("policies", [])}
        cls.permisos = {(p.get("resource"), p.get("effect")) for p in cfg.get("permissions", [])}

    def test_init_local_sin_backend_permitido(self) -> None:
        self.assertIn(("shell:terraform init -backend=false*", "allow"), self.politicas)

    def test_init_con_backend_remoto_denegado(self) -> None:
        self.assertIn(("shell:terraform init -backend=true*", "deny"), self.politicas)

    def test_init_migracion_y_reconfigure_denegados(self) -> None:
        self.assertIn(("shell:terraform init -migrate-state*", "deny"), self.politicas)
        self.assertIn(("shell:terraform init -reconfigure*", "deny"), self.politicas)

    def test_operaciones_mutantes_denegadas(self) -> None:
        for verbo in ("plan", "apply", "destroy", "import", "state"):
            self.assertTrue(
                any(r == f"shell:terraform {verbo}*" and e == "deny" for r, e in self.politicas),
                verbo,
            )

    def test_no_hay_allow_para_plan_apply_destroy(self) -> None:
        # Excepcion unica autorizada (RFC-002, plan del bootstrap): cualquier otro
        # allow sobre plan/apply/destroy/import/state sigue siendo un fallo.
        permitido = "shell:terraform plan -input=false -out=bootstrap.tfplan"
        for recurso, efecto in self.politicas:
            if efecto != "allow" or recurso == permitido:
                continue
            self.assertNotRegex(recurso, r"terraform (plan|apply|destroy|import|state)")

    def test_la_excepcion_de_plan_es_exactamente_la_autorizada(self) -> None:
        allows = [
            recurso
            for recurso, efecto in self.politicas
            if efecto == "allow" and "terraform plan" in recurso
        ]
        self.assertEqual(allows, ["shell:terraform plan -input=false -out=bootstrap.tfplan"])

    def test_aws_mutante_sigue_denegado(self) -> None:
        for recurso, efecto in self.permisos:
            if recurso.startswith("aws *") and any(
                verbo in recurso for verbo in ("create-", "update-", "delete-", "put-")
            ):
                self.assertEqual(efecto, "deny", recurso)
