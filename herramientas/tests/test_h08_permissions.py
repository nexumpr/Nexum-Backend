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
  for cmd in ['aws ec2 run-instances --x','aws ec2 create-vpc --x','aws budgets create-budget --x','terraform apply -auto-approve','terraform init','terraform plan','terraform destroy','git push origin main','git reset --hard HEAD','docker build .']:
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
