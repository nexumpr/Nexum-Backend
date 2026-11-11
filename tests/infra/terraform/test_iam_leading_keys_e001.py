"""Regresión: los prefijos de clave que usa el código deben estar en la política IAM.

El 503 de `POST /api/v1/auth/register` se debía a que `IDEM#` no figuraba en
`local.leading_key_patterns`. `TransactWriteItems` evalúa `dynamodb:LeadingKeys`
por elemento y el statement usa `ForAllValues:StringLike`, así que la tercera
partición (`IDEM#<sha256>`) quedaba denegada y el registro fallaba entero.

Esta prueba cruza **código real** (las funciones de clave que invoca el almacén de
registro de E001) con la **política real** del módulo IAM: si se añade una
partición nueva al código sin actualizar IAM, falla aquí y no en producción.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
KEYS = RAIZ / "internal" / "domain" / "dynamo" / "keys.go"
STORE = RAIZ / "internal" / "adapters" / "dynamo" / "registration_store.go"
IAM = RAIZ / "infra" / "terraform" / "modules" / "iam" / "main.tf"


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


def prefijos_del_codigo(texto: str) -> dict[str, str]:
    """`PrefixX = "VALOR"` → {nombre: valor}."""
    return {
        nombre: valor
        for nombre, valor in re.findall(r'(Prefix[A-Za-z]+)\s*=\s*"([^"]+)"', texto)
    }


def patrones_iam(texto: str) -> list[str]:
    """Literales de la lista `leading_key_patterns` (sin la rama condicional)."""
    inicio = texto.index("leading_key_patterns = concat(")
    fin = texto.index("var.dynamodb_gsi2_enabled", inicio)
    return re.findall(r'"([A-Za-z_#*]+)"', texto[inicio:fin])


def funciones_de_clave_del_store(texto: str) -> set[str]:
    """Funciones `domainDynamo.XxxKeys(...)` que el almacén usa como clave."""
    return set(re.findall(r"domainDynamo\.([A-Za-z]+Keys)\(", texto))


def prefijo_de_la_funcion(codigo: str, funcion: str) -> str | None:
    """`PrefixX` que devuelve el cuerpo de la función de clave."""
    inicio = codigo.find(f"func {funcion}(")
    if inicio < 0:
        return None
    cuerpo = codigo[inicio : inicio + 800]
    encontrado = re.search(r"(Prefix[A-Za-z]+)", cuerpo)
    return encontrado.group(1) if encontrado else None


def problemas_leading_keys(codigo: str, store: str, iam: str) -> list[str]:
    """Particiones usadas por el código sin patrón IAM equivalente."""
    prefijos = prefijos_del_codigo(codigo)
    permitidos = set(patrones_iam(iam))
    problemas: list[str] = []
    for funcion in sorted(funciones_de_clave_del_store(store)):
        nombre = prefijo_de_la_funcion(codigo, funcion)
        if nombre is None:
            problemas.append(f"{funcion} no usa ninguna constante Prefix")
            continue
        valor = prefijos.get(nombre)
        if valor is None:
            problemas.append(f"{nombre} no está declarado en keys.go")
            continue
        if f"{valor}*" not in permitidos:
            problemas.append(f"{nombre}={valor} sin patrón '{valor}*' en leading_key_patterns")
    return problemas


class LeadingKeysE001(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.codigo = _leer(KEYS)
        cls.store = _leer(STORE)
        cls.iam = _leer(IAM)

    def test_el_store_usa_las_tres_particiones_de_e001(self) -> None:
        usadas = funciones_de_clave_del_store(self.store)
        for esperada in ("EmailReservationKeys", "UserKeys", "IdempotencyKeys"):
            self.assertIn(esperada, usadas, esperada)

    def test_resolucion_de_prefijos(self) -> None:
        self.assertEqual(prefijo_de_la_funcion(self.codigo, "UserKeys"), "PrefixUser")
        self.assertEqual(prefijo_de_la_funcion(self.codigo, "EmailReservationKeys"), "PrefixEmail")
        self.assertEqual(prefijo_de_la_funcion(self.codigo, "IdempotencyKeys"), "PrefixIdempotency")

    def test_idem_esta_entre_los_permitidos(self) -> None:
        self.assertIn("IDEM#*", patrones_iam(self.iam))

    def test_todas_las_particiones_de_e001_estan_autorizadas(self) -> None:
        self.assertEqual(problemas_leading_keys(self.codigo, self.store, self.iam), [])

    def test_se_conservan_los_prefijos_previos(self) -> None:
        permitidos = patrones_iam(self.iam)
        for previo in ("USER#*", "EMAIL#*", "SESSION#*", "REFRESH#*", "LOOKUP#SESSION#*", "PUBLIC#CATALOG"):
            self.assertIn(previo, permitidos, previo)

    def test_la_condicion_sigue_siendo_forallvalues_y_arn_exacto(self) -> None:
        self.assertIn("ForAllValues:StringLike", self.iam)
        self.assertIn("Null", self.iam)
        self.assertIn("var.dynamodb_table_arn", self.iam)
        self.assertNotIn('Resource  = "*"', self.iam)

    def test_no_se_concede_scan(self) -> None:
        self.assertNotIn("dynamodb:Scan", self.iam)
        self.assertNotIn("dynamodb:*", self.iam)

    # Fixtures: el detector debe fallar al retirar IDEM# o añadir un prefijo nuevo.
    def test_detector_rechaza_idem_ausente(self) -> None:
        iam_sin_idem = self.iam.replace('      "IDEM#*",\n', "")
        problemas = " ".join(problemas_leading_keys(self.codigo, self.store, iam_sin_idem))
        self.assertIn("PrefixIdempotency=IDEM#", problemas)

    def test_detector_rechaza_prefijo_nuevo_sin_politica(self) -> None:
        codigo = self.codigo.replace('PrefixIdempotency = "IDEM#"', 'PrefixIdempotency = "IDEM2#"')
        problemas = " ".join(problemas_leading_keys(codigo, self.store, self.iam))
        self.assertIn("IDEM2#", problemas)


if __name__ == "__main__":
    unittest.main()
