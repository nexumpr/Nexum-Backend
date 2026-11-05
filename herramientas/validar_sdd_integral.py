#!/usr/bin/env python3
"""Nexum SDD H07 validator, offline and read-only. Not a deployment / source-completeness certification."""
from pathlib import Path
import argparse,csv,hashlib,json,subprocess,sys,re

def sha(b):return hashlib.sha256(b).hexdigest()

def validate(root):
 root=root.resolve();bad=[]
 def ensure(cond,msg):
  if not cond:bad.append(msg)
 for name in ('constitution.md','AGENTS.md','memoria.md','opencode.json','README.md','.gitignore'):
  ensure((root/name).is_file(),'falta raiz '+name)
 ag=list((root/'.opencode/agents').glob('*.md'))
 skills=list((root/'.opencode/skills').glob('*/SKILL.md'))
 ensure(len(ag)==8,'agentes !=8');ensure(len(skills)==9,'skills !=9')
 sums={}
 for tag,folder,expected in [('A','sdd/01-arquitectura-general',12),('I','sdd/02-infraestructura',16),('E','sdd/03-endpoints',75)]:
  paths=sorted((root/folder).glob((tag+'[0-9][0-9][0-9]-*/spec.md') if tag=='E' else (tag+'-[0-9][0-9][0-9]-*/spec.md')))
  ensure(len(paths)==expected,f'{tag} cantidad {len(paths)} != {expected}')
  ids=set()
  for p in paths:
   sid=p.parent.name.split('-')[0] if tag=='E' else '-'.join(p.parent.name.split('-')[:2])
   ensure(sid not in ids,'duplicado '+sid);ids.add(sid)
   for name in ('spec.md','plan.md','tasks.md','checklist.md'):
    q=p.parent/name
    ensure(q.is_file() and q.stat().st_size>200,f'falta/incompleto {q.relative_to(root)}')
  sums[tag]=len(paths)
 mat=root/'gobierno/H07/MATRIZ_ENTREGABLES_103.csv'
 if mat.is_file():
  with mat.open(encoding='utf-8',newline='') as f: m=list(csv.DictReader(f))
  ensure(len(m)==103,'matriz 103 != '+str(len(m)))
  ensure(len(set(r['id'] for r in m))==103,'ids repetidos en matriz')
 else:bad.append('falta matriz 103')
 for needed in ('sdd/02-infraestructura/CONTRATO_IAC_OPERACION_H05_V4.md','sdd/02-infraestructura/APROBACION_H05_DOCUMENTAL_CONDICIONADA_20261105.md','sdd/03-endpoints/GATE_H06_PENDIENTE.md','gobierno/H07/GATE_H06_PARA_APROBACION.md','gobierno/H07/GATE_H07_PARA_APROBACION.md'):
  ensure((root/needed).is_file(),'falta '+needed)
 # Reuse the actual H06 validator exactly: stdout is transparent
 import runpy
 h06=root/'sdd/03-endpoints/herramientas/validar_h06.py'
 if h06.is_file():
  ns=runpy.run_path(str(h06),run_name='nexum_h06_lib')
  try:ns['main'](root)
  except Exception as exc:bad.append('H06 invalido: '+str(exc))
 else:bad.append('falta validador H06')
 manifest=root/'gobierno/H07/BASELINE_SHA256.json'
 if not manifest.is_file():bad.append('falta baseline SHA')
 else:
  base=json.loads(manifest.read_text(encoding='utf-8'))
  entries=base['files']; ensure(len(entries)>=460,'baseline insuficiente')
  for rel,expected in entries.items():
   p=root/rel
   if '..' in Path(rel).parts or Path(rel).is_absolute() or p.is_symlink() or not p.is_file():bad.append('ruta invalida/ausente '+rel);continue
   if sha(p.read_bytes())!=expected:bad.append('SHA_CHANGED '+rel)
  ensure(len(set(entries))==len(entries),'duplicados en baseline')
  on_disk={str(p.relative_to(root)).replace('\\','/') for p in root.rglob('*') if p.is_file() and '.git' not in p.parts and '__pycache__' not in p.parts and 'node_modules' not in p.parts}
  extras=sorted(on_disk-set(entries)-{'gobierno/H07/BASELINE_SHA256.json'})
  # Extra source files could be future code; explicitly report, do not block future H08.
  if extras: print('EXTRA_FILES_NOT_IN_BASELINE:',len(extras),'(revisar staging; no son PASS de baseline)')
 for rel in (root/'.gitignore',):
  if rel.is_file():
   t=rel.read_text(encoding='utf-8');ensure('*.tfstate' in t and '.env' in t,'gitignore falta secretos/terraform')
 if bad:
  print('SDD_DOCUMENTAL_FAIL:',len(bad));print('\n'.join(bad[:25]));return 2
 print('SDD_DOCUMENTAL_PASS: 12 A + 16 I + 75 E = 103 specs; 412 docs; 8 agents; 9 skills; SHA256 baseline; AWS/GitHub NOT_RUN')
 return 0
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--root',default=None)
 a=p.parse_args();root=Path(a.root) if a.root else Path(__file__).resolve().parents[1]
 sys.exit(validate(root))
