"""Contrato H09-90: proteccion de datos y sink de logs sin ciclo Terraform."""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
MODULO_OBS = RAIZ / "infra" / "terraform" / "modules" / "observabilidad" / "main.tf"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _bloque(texto: str, cabecera: str, marcador: str | None = None) -> str:
    inicio = texto.index(cabecera)
    if marcador is not None:
        posicion = texto.index(marcador)
        candidatos = [i for i in range(0, posicion) if texto.startswith(cabecera, i)]
        if not candidatos:
            raise AssertionError(f"no hay {cabecera} antes de {marcador}")
        inicio = candidatos[-1]
    profundidad = 0
    for indice in range(inicio, len(texto)):
        if texto[indice] == "{":
            profundidad += 1
        elif texto[indice] == "}":
            profundidad -= 1
            if profundidad == 0:
                return texto[inicio : indice + 1]
    raise AssertionError(f"bloque no cerrado: {cabecera}")


class ProteccionDatos(unittest.TestCase):
    def test_deletion_protection_activa_por_defecto(self) -> None:
        variables = _leer(ENV / "variables.tf")
        bloque = _bloque(variables, 'variable "dynamodb_proteccion_borrado"')
        self.assertRegex(bloque, r"default\s*=\s*true")
        self.assertIn("Gate explicito", bloque)

    def test_modulo_recibe_la_variable(self) -> None:
        main = _leer(ENV / "main.tf")
        self.assertIn("proteccion_borrado = var.dynamodb_proteccion_borrado", main)


class SinkLogsSinCiclo(unittest.TestCase):
    def test_arn_determinista_sin_referenciar_observabilidad(self) -> None:
        main = _leer(ENV / "main.tf")
        bloque = _bloque(main, "locals {", "log_group_backend_name")
        self.assertIn('log_group_backend_name = "/nexum/${var.entorno}/backend"', bloque)
        self.assertIn('"arn:aws:logs:${var.region}', bloque)
        self.assertNotIn("module.observabilidad[", bloque)
        self.assertIn("cloudwatch_log_group_arns = local.iam_log_group_arns", main)

    def test_nombre_coincide_con_el_modulo_de_observabilidad(self) -> None:
        observabilidad = _leer(MODULO_OBS)
        self.assertIn('name              = "/${var.proyecto}/${var.entorno}/backend"', observabilidad)


if __name__ == "__main__":
    unittest.main()
