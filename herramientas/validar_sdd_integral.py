#!/usr/bin/env python3
"""Validación estricta y de solo lectura del SDD Nexum. NO valida AWS, runtime ni gates humanos."""
from pathlib import Path
import argparse,csv,hashlib,json,re,subprocess,sys,runpy

MANIFEST="sdd/_control/H07/BASELINE_SHA256.json"
SKIP_PARTS={'.git','__pycache__','node_modules','.pytest_cache','.terraform'}
# Artefactos locales excluidos por .gitignore: no son documentos ni código versionable.
# El plan se conserva en disco para su Gate, pero nunca se publica.
SKIP_SUFFIXES=('.tfplan',)
# Artefactos de ejecución local que NO son entregables: build nativo (`dist/`) y
# state operativo de Terraform. Se excluyen del manifiesto porque cambian en cada
# ejecución local; la seguridad no depende de esta exclusión sino de `git`: un
# tfstate o un binario *rastreado* sigue siendo un error (`GIT_UNDECLARED` en
# --check-git) y `herramientas/verificar_iac_devsecops.py` bloquea tfstate/secretos.
LOCAL_ARTIFACT_PARTS={'dist'}
LOCAL_ARTIFACT_SUFFIXES=('.tfstate','.tfstate.backup','.tfstate.lock.info','.tflock')


def is_local_artifact(rel_posix: str) -> bool:
    partes=set(rel_posix.split('/'))
    return bool(partes&LOCAL_ARTIFACT_PARTS) or rel_posix.endswith(LOCAL_ARTIFACT_SUFFIXES)


def is_h08_code(rel_posix: str) -> bool:
    return rel_posix.startswith(('cmd/', 'internal/', 'pkg/', 'tests/')) or rel_posix in ('go.mod', 'go.sum')

def sha256(data:bytes)->str:return hashlib.sha256(data).hexdigest()

def paths_of(root:Path):
    return {p.relative_to(root).as_posix() for p in root.rglob('*') if p.is_file() and not (set(p.relative_to(root).parts)&SKIP_PARTS) and not p.name.endswith(SKIP_SUFFIXES) and not is_local_artifact(p.relative_to(root).as_posix()) and not is_h08_code(p.relative_to(root).as_posix())}

def validate(root:Path,check_git=False):
    root=root.resolve(); errors=[]
    def need(cond,reason):
        if not cond: errors.append(reason)
    for rel in ('constitution.md','AGENTS.md','memoria.md','README.md','opencode.json','.gitignore','.gitattributes',
                '.github/workflows/sdd-validacion.yml', 'sdd/_control/H07/ESTADO_Y_GATES.md'):
        need((root/rel).is_file(),'FALTA '+rel)
    need(not (root/'gobierno').exists(),'RUTA_NO_AUTORIZADA gobierno/')
    need(len(list((root/'.opencode/agents').glob('*.md')))==8,'AGENTES != 8')
    need(len(list((root/'.opencode/skills').glob('*/SKILL.md')))==9,'SKILLS != 9')
    specs={}
    for prefix, folder, quantity in (('A','01-arquitectura-general',12),('I','02-infraestructura',16),('E','03-endpoints',75)):
        pattern=(prefix+'[0-9][0-9][0-9]-*/spec.md') if prefix=='E' else (prefix+'-[0-9][0-9][0-9]-*/spec.md')
        group=sorted((root/'sdd'/folder).glob(pattern))
        specs[prefix]=group
        need(len(group)==quantity,f'NUMERO_{prefix} {len(group)} != {quantity}')
        ids=[]
        for p in group:
            ids.append(p.parent.name[:4] if prefix=='E' else p.parent.name[:5])
            for basename in ('spec.md','plan.md','tasks.md','checklist.md'):
                f=p.parent/basename
                need(f.is_file() and f.stat().st_size>200,f'INCOMPLETO {f.relative_to(root).as_posix()}')
        need(len(ids)==len(set(ids)),f'DUPLICADOS_{prefix}')
    # Sólo tres bloques con specs; no confundir documentos de control con especificaciones
    for p in (root/'sdd/_control').rglob('spec.md'):
        errors.append('CONTROL_CONTIENE_SPEC '+p.relative_to(root).as_posix())
    matrix=root/'sdd/_control/H07/MATRIZ_ENTREGABLES_103.csv'
    if matrix.is_file():
        with matrix.open(encoding='utf-8',newline='') as f: rows=list(csv.DictReader(f))
        need(len(rows)==103,'MATRIZ_103 '+str(len(rows)))
        need(len({r['id'] for r in rows})==103,'MATRIZ_IDS_REPETIDOS')
    else:errors.append('FALTA_MATRIZ_103')
    for required in ('sdd/02-infraestructura/CONTRATO_IAC_OPERACION_H05_V4.md',
                     'sdd/02-infraestructura/APROBACION_H05_DOCUMENTAL_CONDICIONADA_20261105.md',
                     'sdd/03-endpoints/GATE_H06_PENDIENTE.md',
                     'sdd/_control/H07/GATE_H06_PARA_APROBACION.md',
                     'sdd/_control/H07/GATE_H07_PARA_APROBACION.md',
                     'sdd/_control/H07/CAUSA_RAIZ_CI_Y_MIGRACION.md'):
        need((root/required).is_file(),'FALTA '+required)
    # H06 schema/examples are validated independently, with exact existing code.
    tool=root/'sdd/03-endpoints/herramientas/validar_h06.py'
    if tool.exists():
        try:
            ns=runpy.run_path(str(tool),run_name='nexum_h06_lib');ns['main'](root)
        except Exception as ex: errors.append('H06_INVALIDO '+repr(ex))
    else: errors.append('FALTA_VALIDAR_H06')
    manifest=root/MANIFEST
    expected={}
    if not manifest.is_file():errors.append('FALTA_BASELINE_SHA256')
    else:
        try:
            obj=json.loads(manifest.read_text(encoding='utf-8'));expected=obj['files']
        except Exception as ex:errors.append('MANIFEST_JSON '+repr(ex))
        if expected:
            need(obj.get('algorithm')=='SHA256','ALGORITMO_INVALIDO')
            need(len(expected)>=480,'BASELINE_INSUFICIENTE')
            for rel, digest in expected.items():
                pp=Path(rel)
                if pp.is_absolute() or '..' in pp.parts or '\\' in rel or (root/rel).is_symlink():
                    errors.append('RUTA_INVALIDA '+rel);continue
                f=root/pp
                if not f.is_file():errors.append('NO_EXISTE '+rel)
                elif sha256(f.read_bytes())!=digest:errors.append('SHA_CHANGED '+rel)
            wanted=set(expected)|{MANIFEST}
            disk=paths_of(root)
            missing=sorted(wanted-disk)
            extras=sorted(disk-wanted)
            if missing:errors.extend('MANIFEST_MISSING '+r for r in missing[:20])
            if extras:errors.extend('UNDECLARED_FILE '+r for r in extras[:20])
            if check_git:
                try:
                    r=subprocess.run(['git','ls-files','-z'],cwd=root,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True,timeout=20)
                    tracked=set(s.decode('utf-8') for s in r.stdout.split(b'\x00') if s and not is_h08_code(s.decode('utf-8')))
                    if tracked!=wanted:
                        errors.extend('GIT_UNDECLARED '+r for r in sorted(tracked-wanted)[:20])
                        errors.extend('GIT_MISSING '+r for r in sorted(wanted-tracked)[:20])
                except Exception as ex:errors.append('GIT_LIST_FAILED '+repr(ex))
    if (root/'.gitignore').is_file():
        ign=(root/'.gitignore').read_text(encoding='utf-8')
        need('.env' in ign and '*.tfstate' in ign,'GITIGNORE_NO_SECRETS')
    if errors:
        print(f'SDD_DOCUMENTAL_FAIL: {len(errors)}')
        for e in errors[:30]:print(e)
        return 2
    print('SDD_DOCUMENTAL_PASS: 12 A + 16 I + 75 E = 103 specs; 412 docs; 8 agents; 9 skills; '+str(len(expected)+1)+' tracked files; SHA256 exact; AWS/HTTP NOT_RUN')
    return 0

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--root');p.add_argument('--check-git',action='store_true')
    a=p.parse_args();sys.exit(validate(Path(a.root) if a.root else Path(__file__).resolve().parents[1],a.check_git))
