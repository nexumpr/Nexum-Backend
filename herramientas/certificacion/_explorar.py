import json
import sys

d = json.load(open('sdd/03-endpoints/openapi/nexum-api-v1.json', encoding='utf-8'))


def resolve(schema, depth=0):
    if depth > 8:
        return '...'
    if not isinstance(schema, dict):
        return schema
    if '$ref' in schema:
        name = schema['$ref'].split('/')[-1]
        return {'$ref': name, 'resolved': resolve(d['components']['schemas'][name], depth + 1)}
    out = {}
    for k in ('type', 'required', 'enum', 'format', 'pattern', 'additionalProperties', 'minLength', 'maxLength', 'minItems', 'maxItems', 'minimum', 'maximum'):
        if k in schema:
            out[k] = schema[k]
    if 'properties' in schema:
        out['properties'] = {p: resolve(v, depth + 1) for p, v in schema['properties'].items()}
    if 'items' in schema:
        out['items'] = resolve(schema['items'], depth + 1)
    for k in ('oneOf', 'anyOf', 'allOf'):
        if k in schema:
            out[k] = [resolve(x, depth + 1) for x in schema[k]]
    return out


ids = sys.argv[1:]
for p, item in d['paths'].items():
    for m, op in item.items():
        if m.lower() not in ('get', 'post', 'put', 'patch', 'delete'):
            continue
        oid = op.get('operationId', '')
        if ids and oid not in ids:
            continue
        print('=' * 70)
        print(oid, m.upper(), p)
        rb = op.get('requestBody')
        if rb:
            for ct, media in rb.get('content', {}).items():
                print(' request', resolve(media.get('schema')))
        for prm in op.get('parameters', []):
            print(' param', prm.get('in'), prm.get('name'), prm.get('required'), resolve(prm.get('schema')))
        for code, resp in op.get('responses', {}).items():
            if not code.startswith(('2', '3')):
                continue
            content = resp.get('content') or {}
            sch = None
            for ct, media in content.items():
                sch = resolve(media.get('schema'))
                break
            hdrs = list((resp.get('headers') or {}).keys())
            print(' resp', code, 'headers=', hdrs, sch if sch is not None else '(no body)')
