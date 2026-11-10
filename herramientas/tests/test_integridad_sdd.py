"""Regresión sin AWS: comprueba cobertura, SHA negativas, CRLF y rutas de gobierno."""
from pathlib import Path
import unittest, tempfile, shutil, os, hashlib, json, sys
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
import validar_sdd_integral as vi
ROOT=Path(__file__).resolve().parents[2]

class IntegridadSDD(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.m=json.loads((ROOT/vi.MANIFEST).read_text(encoding='utf-8'))['files']

    def test_estructura_y_tres_bloques(self):
        self.assertEqual(len(list((ROOT/'sdd/01-arquitectura-general').glob('A-*/spec.md'))),12)
        self.assertEqual(len(list((ROOT/'sdd/02-infraestructura').glob('I-*/spec.md'))),16)
        self.assertEqual(len(list((ROOT/'sdd/03-endpoints').glob('E*/spec.md'))),75)
        self.assertFalse((ROOT/'gobierno').exists())
        self.assertFalse(any((ROOT/'sdd/_control').rglob('spec.md')))

    def test_manifest_sha_y_todos_los_archivos(self):
        self.assertEqual(set(self.m)|{vi.MANIFEST},vi.paths_of(ROOT))
        for relative, expected in self.m.items():
            with self.subTest(file=relative):
                self.assertEqual(vi.sha256((ROOT/relative).read_bytes()),expected)

    def test_normalizacion_no_oculta_un_sha_distinto(self):
        p='sdd/02-infraestructura/FINOPS_45_DIAS_DESGLOSE_V2.csv'
        data=(ROOT/p).read_bytes()
        self.assertNotIn(b'\r\n',data)
        self.assertEqual(vi.sha256(data),self.m[p])
        self.assertNotEqual(vi.sha256(data.replace(b'\n',b'\r\n')),self.m[p])

    def test_manifest_no_considera_hash_igual_si_hay_cambio(self):
        b=b'integridad test'
        self.assertNotEqual(vi.sha256(b),self.m['constitution.md'])

    def test_artefactos_locales_no_son_entregables(self):
        # Build nativo y state operativo: fuera del manifiesto por diseño.
        for ruta in ('dist/nexum-backend-linux-arm64','dist/nexum-backend-linux-arm64.manifest.json',
                     'infra/terraform/bootstrap/terraform.tfstate','infra/terraform/bootstrap/terraform.tfstate.backup',
                     'envs/h08i/terraform.tfstate.tflock'):
            with self.subTest(ruta=ruta):
                self.assertTrue(vi.is_local_artifact(ruta))
        # El plan binario se excluye por SKIP_SUFFIXES, no por esta guarda.
        self.assertTrue('x/fase-base.tfplan'.endswith(vi.SKIP_SUFFIXES))
        # Un documento o un .tf real sigue siendo exigible.
        for ruta in ('memoria.md','infra/terraform/envs/h08i/main.tf','sdd/_control/H08/RFC-011-IDENTIDADES_PUBLICACION.md',
                     'herramientas/validar_sdd_integral.py','tests/infra/terraform/test_red_contrato.py'):
            with self.subTest(ruta=ruta):
                self.assertFalse(vi.is_local_artifact(ruta))

    def test_no_referencias_a_gobierno_h07_en_archivos_activos(self):
        for relative in self.m:
            if relative.endswith(('.md','.py','.yaml','.yml','.json','.csv')):
                self.assertNotIn('](gobierno/'+'H07/',(ROOT/relative).read_text(encoding='utf-8'),relative)

if __name__=='__main__':unittest.main()
