#!/usr/bin/env python3
"""H06 deterministic offline integrity audit. No network and NO filesystem mutation."""
from pathlib import Path
import json,re,csv,sys

def main(root):
    root=Path(root).resolve()
    api_dir=root/'sdd'/'03-endpoints'
    manifest=api_dir/'openapi'/'nexum-api-v1.json'
    if not manifest.is_file():raise RuntimeError('Falta OpenAPI JSON')
    data=json.loads(manifest.read_text(encoding='utf-8'))
    if data.get('openapi')!='3.1.0':raise RuntimeError('OpenAPI no es 3.1.0')
    if data['servers'][0]['url']!='https://api.example.invalid':raise RuntimeError('No se permite afirmar URL AWS aún')
    with (api_dir/'MATRIZ_OPERACIONES_H06.csv').open(encoding='utf-8',newline='') as f:
        rows=list(csv.DictReader(f))
    if len(rows)!=75:raise RuntimeError('Esperados 75, hay '+str(len(rows)))
    ids={r['id'] for r in rows}
    if ids!={f'E{i:03d}' for i in range(1,76)}:raise RuntimeError('Identificadores incompletos/duplicados')
    expected={'M':33,'S':23,'E':12,'O':7}
    if {c:sum(r['clase_h04']==c for r in rows) for c in expected}!=expected:raise RuntimeError('Clases H04 erróneas')
    seen=set();count=0
    for r in rows:
        pp=r['ruta'];method=r['metodo'].lower();code=r['http_exito']
        if (pp,method) in seen:raise RuntimeError('Duplicado HTTP')
        seen.add((pp,method));op=data['paths'][pp][method];count+=1
        if op.get('x-nexum-catalog-id')!=r['id'] or op.get('operationId')!='nexum_'+r['id'].lower():raise RuntimeError('ID en contrato: '+r['id'])
        if str(code) not in op['responses']:raise RuntimeError('Código éxito '+r['id'])
        if bool(op.get('security'))!=(r['auth']=='bearer'):raise RuntimeError('Autenticación '+r['id'])
        if bool(op.get('requestBody'))!=(r['request_schema']!=''):raise RuntimeError('Request schema '+r['id'])
        if bool(op.get('parameters') and any(p['name']=='Idempotency-Key' for p in op['parameters']))!=(r['idempotency_key']=='True'):raise RuntimeError('Idempotency mismatch '+r['id'])
        if bool(op.get('parameters') and any(p['name']=='If-Match' for p in op['parameters']))!=(r['if_match']=='True'):raise RuntimeError('If-Match mismatch '+r['id'])
        for filename in ('spec.md','plan.md','tasks.md','checklist.md'):
            p=api_dir/r['carpeta']/filename
            if not p.is_file() or len(p.read_text(encoding='utf-8'))<500:raise RuntimeError('Incompleto '+str(p))
            if r['id'] not in p.read_text(encoding='utf-8'):raise RuntimeError('ID ausente '+str(p))
    ops=sum(1 for v in data['paths'].values() for m in v if m.lower() in {'get','post','put','patch','delete'})
    if ops!=75 or count!=75:raise RuntimeError('OpenAPI distinto de matriz')
    models=data['components']['schemas'];refprefix='#/components/schemas/'
    def walk(obj):
        if isinstance(obj,dict):
            for k,v in obj.items():
                if k=='$ref' and isinstance(v,str):
                    if not v.startswith(refprefix) or v[len(refprefix):] not in models:raise RuntimeError('Referencia inválida '+v)
                else:walk(v)
        elif isinstance(obj,list):
            for item in obj:walk(item)
    walk(data)
    for sub in ('README_H06.md','MATRIZ_COBERTURA_H06.md','DYNAMODB_ACCESOS_H06.md','POLITICAS_TRANSVERSALES_H06.md','IA_ML_Y_NOTIFICACIONES_H06.md','PRUEBAS_CONTRATOS_H06.md','GATE_H06_PENDIENTE.md'):
        if not (api_dir/sub).is_file():raise RuntimeError('Falta anexo '+sub)
    print('H06_CONTRACT_PASS: 75 operations; 300 specs; 33M+23S+12E+7O; OpenAPI 3.1 refs valid; no AWS writes')
    return 0

if __name__=='__main__':
    if len(sys.argv)>2:raise SystemExit('Uso: py -3.13 validar_h06.py [C:\\Proyectos\\Nexum\\Nexum-Backend]')
    base=Path(sys.argv[1]) if len(sys.argv)==2 else Path(__file__).resolve().parents[3]
    try:raise SystemExit(main(base))
    except Exception as e:print('H06_CONTRACT_FAIL:',e);raise SystemExit(2)
