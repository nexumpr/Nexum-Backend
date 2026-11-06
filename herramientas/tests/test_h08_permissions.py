import unittest,json,re,fnmatch
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
class H08Permissions(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  cls.c=json.loads((ROOT/'opencode.json').read_text(encoding='utf-8'))
  cls.rules=cls.c['permissions'];cls.policies=cls.c['policies']
 def eff(self,action,resource):
  effect='allow'
  for r in self.rules:
   if fnmatch.fnmatchcase(action,r['action']) and fnmatch.fnmatchcase(resource,r['resource']):effect=r['effect']
  for p in self.policies:
   if p['action']=='tool.use' and p['effect']=='deny' and fnmatch.fnmatchcase(action+':'+resource,p['resource']):effect='deny'
  return effect
 def test_edits(self):
  for p in ['cmd/api/main.go','internal/domain/money.go','tests/unit/health_test.go','go.mod','go.sum']:
   self.assertEqual(self.eff('edit',p),'ask',p)
  for p in ['constitution.md','sdd/01-arquitectura-general/A-000-contexto-sistema/spec.md','sdd/03-endpoints/openapi/nexum-api-v1.json','opencode.json','.github/workflows/sdd-validacion.yml','infra/main.tf','.env','foo.pem']:
   self.assertEqual(self.eff('edit',p),'deny',p)
 def test_cloud_and_git_hard_denies(self):
  for c in ['aws ec2 describe-instances','terraform plan','terraform apply','gh pr merge 1','git push origin main','git reset --hard HEAD','docker run test']:
   self.assertEqual(self.eff('shell',c),'deny',c)
  self.assertEqual(self.eff('shell','go test ./...'),'ask')
 def test_secrets_and_external(self):
  self.assertEqual(self.eff('read','.env'),'deny')
  self.assertEqual(self.eff('read','a.tfstate'),'deny')
  self.assertEqual(self.eff('external_directory','C:/Users/leoba/.aws/credentials'),'deny')
 def test_agent_skills(self):
  self.assertEqual(self.c['default_agent'],'nexum-orchestrator')
  self.assertEqual(self.eff('subagent','go-backend-architect'),'ask')
  self.assertEqual(self.eff('subagent','aws-terraform-engineer'),'deny')
  self.assertEqual(self.eff('skill','validate-go'),'ask')
  self.assertEqual(self.eff('skill','validate-terraform'),'deny')
 def test_profiles_frontmatter(self):
  agents=list((ROOT/'.opencode/agents').glob('*.md'));self.assertEqual(len(agents),8)
  for p in agents:
   txt=p.read_text(encoding='utf-8');self.assertTrue(txt.startswith('---\n'))
   self.assertRegex(txt,r'(?m)^mode: (primary|subagent)$')
   self.assertIn('permissions:',txt)
   self.assertNotIn('permission:',txt.split('---',2)[1])
