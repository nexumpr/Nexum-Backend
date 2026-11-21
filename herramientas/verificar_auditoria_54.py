#!/usr/bin/env python3
"""Verificador documental de la auditoria semantica de los 54 endpoints.

Comprueba, sobre `sdd/_control/H08/MATRIZ_AUDITORIA_54_ENDPOINTS.md`:

- exactamente 54 E-ID, sin duplicados ni faltantes respecto del conjunto implementado;
- ficha existente con spec/plan/tasks/checklist;
- referencias de tests existentes (archivo y funcion `Test*`);
- versiones citadas con fuente localizable en memoria/Gates;
- ausencia de estados funcionales no sustentados y de `FUNCTIONAL_VERIFIED`;
- clasificacion SES sin certificacion de recepcion inexistente;
- contradicciones historico/vigente resueltas con nota de vigencia;
- enlaces internos no rotos.

No modifica archivos. Codigo 0 = PASS, 1 = FAIL, 2 = error de uso.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MATRIZ = ROOT / "sdd/_control/H08/MATRIZ_AUDITORIA_54_ENDPOINTS.md"

E001_E011 = [f"E{i:03d}" for i in range(1, 12)]
OLEADA1 = ["E012", "E013", "E015", "E016", "E017", "E018", "E019",
           "E022", "E023", "E024", "E025", "E068", "E069", "E070", "E071"]
OLEADA2 = ["E014", "E027", "E028", "E029", "E030", "E031", "E032", "E033",
           "E034", "E035", "E036", "E038", "E039", "E040", "E041"]
OLEADA3 = ["E020", "E026", "E037", "E042", "E043", "E044", "E045", "E046",
           "E047", "E048", "E049", "E050", "E051"]
ESPERADOS = E001_E011 + OLEADA1 + OLEADA2 + OLEADA3
SES = {"E005", "E006", "E010", "E011"}
RESULTADOS_VALIDOS = {
    "AUDITADA_SEMANTICA_CONFORME",
    "AUDITADA_SEMANTICA_CON_OBSERVACIONES",
    "DISCREPANCIA_DOCUMENTAL_REGISTRADA",
    "PARTIAL_SES",
    "NO_CONFORME_SEMANTICA",
}
MARCA_VIGENCIA = "NOTA_VIGENCIA_AUDITORIA_SEMANTICA"
RE_VERSION = re.compile(r"v\d+\.\d+\.\d+\+[0-9a-f]{8,40}")
RE_TICK = re.compile(r"`([^`]+)`")
RE_LINK = re.compile(r"\[[^\]]+\]\(([^)]+)\)")
POSITIVO = ("PASS", "DEPLOYED=YES", "VERIFIED=YES", "IMPLEMENTED=YES")
NEGATIVO = ("AWS_DEPLOYED=NO", "HTTPS_VERIFIED=NO", "FUNCTIONAL_VERIFIED=NO", "NOT_RUN")


def filas_matriz(texto: str) -> list[list[str]]:
    filas = []
    for linea in texto.splitlines():
        if re.match(r"^\|\s*E\d{3}\s*\|", linea):
            filas.append([c.strip() for c in linea.strip().strip("|").split("|")])
    return filas


def indice_fuentes() -> str:
    partes = []
    memoria = ROOT / "memoria.md"
    if memoria.is_file():
        partes.append(memoria.read_text(encoding="utf-8", errors="replace"))
    for base in (ROOT / "sdd/_control/H08", ROOT / "sdd/_control/H07"):
        if base.is_dir():
            for archivo in base.rglob("*.md"):
                partes.append(archivo.read_text(encoding="utf-8", errors="replace"))
    return "\n".join(partes)


def carpeta_ficha(eid: str) -> Path | None:
    base = ROOT / "sdd/03-endpoints"
    if not base.is_dir():
        return None
    encontradas = sorted(p for p in base.glob(f"{eid}-*") if p.is_dir() and (p / "spec.md").is_file())
    return encontradas[0] if len(encontradas) == 1 else None


def main() -> int:
    problemas: list[str] = []
    if not MATRIZ.is_file():
        print(f"FALTA la matriz: {MATRIZ}")
        return 1

    texto = MATRIZ.read_text(encoding="utf-8")
    filas = filas_matriz(texto)
    ids = [f[0] for f in filas]

    if len(filas) != 54:
        problemas.append(f"filas={len(filas)}; se exigen 54")
    if len(set(ids)) != len(ids):
        dup = sorted({i for i in ids if ids.count(i) > 1})
        problemas.append(f"E-ID duplicados: {dup}")
    faltan = sorted(set(ESPERADOS) - set(ids))
    sobran = sorted(set(ids) - set(ESPERADOS))
    if faltan:
        problemas.append(f"E-ID faltantes: {faltan}")
    if sobran:
        problemas.append(f"E-ID no esperados: {sobran}")

    fuentes = indice_fuentes()
    auditadas = 0
    for fila in filas:
        if len(fila) != 11:
            problemas.append(f"{fila[0]}: columnas={len(fila)}; se exigen 11")
            continue
        eid, spec, plan, tasks, checklist, contrato, codigo, tests, evidencia, resultado, correcciones = fila

        for etiqueta, valor in (("spec", spec), ("plan", plan), ("tasks", tasks), ("checklist", checklist)):
            if not re.search(r"LE[IÍ]DO|REVISAD", valor, re.IGNORECASE):
                problemas.append(f"{eid}: columna {etiqueta} sin lectura declarada ({valor!r})")

        carpeta = carpeta_ficha(eid)
        if carpeta is None:
            problemas.append(f"{eid}: no existe una unica ficha sdd/03-endpoints/{eid}-*")
        else:
            for exigido in ("spec.md", "plan.md", "tasks.md", "checklist.md"):
                if not (carpeta / exigido).is_file():
                    problemas.append(f"{eid}: falta {exigido}")
            for nombre in ("tasks.md", "checklist.md"):
                archivo = carpeta / nombre
                if archivo.is_file():
                    cuerpo = archivo.read_text(encoding="utf-8", errors="replace")
                    if any(p in cuerpo for p in POSITIVO) and any(n in cuerpo for n in NEGATIVO):
                        if MARCA_VIGENCIA not in cuerpo:
                            problemas.append(
                                f"{eid}: {nombre} mezcla estados historicos y vigentes sin {MARCA_VIGENCIA}"
                            )

        if not re.search(r"nexum_e\d{3}", contrato):
            problemas.append(f"{eid}: contrato sin operationId nexum_eNNN")

        for token in RE_TICK.findall(codigo) + RE_TICK.findall(tests):
            token = token.strip()
            if not token or token.startswith("nexum_e"):
                continue
            ruta = token.split("::")[0].split(":")[0].strip()
            if ruta.startswith("internal/") or ruta.startswith("cmd/") or ruta.startswith("tests/"):
                if not (ROOT / ruta).exists():
                    problemas.append(f"{eid}: ruta citada inexistente {ruta}")
                if "::" in token and ruta.endswith(".go"):
                    nombre = token.split("::", 1)[1].strip()
                    if nombre:
                        contenido = (ROOT / ruta).read_text(encoding="utf-8", errors="replace")
                        if not re.search(rf"func\s+{re.escape(nombre)}\b", contenido):
                            problemas.append(f"{eid}: test citado inexistente {token}")

        for version in RE_VERSION.findall(evidencia):
            if version not in fuentes:
                problemas.append(f"{eid}: version sin fuente localizable {version}")

        if "FUNCTIONAL_VERIFIED" in evidencia or "FUNCTIONAL_VERIFIED" in resultado:
            problemas.append(f"{eid}: usa FUNCTIONAL_VERIFIED; la matriz debe declarar auditoria, no certificacion")

        if resultado not in RESULTADOS_VALIDOS:
            problemas.append(f"{eid}: resultado no valido {resultado!r}")
        else:
            auditadas += 1

        if eid in SES:
            if "SES" not in evidencia:
                problemas.append(f"{eid}: endpoint SES sin clasificacion SES en Evidencia")
            if resultado == "AUDITADA_SEMANTICA_CONFORME":
                problemas.append(f"{eid}: SES no puede declararse conforme sin recepcion verificada")
            if not re.search(r"recepci[oó]n", evidencia, re.IGNORECASE):
                problemas.append(f"{eid}: SES sin declaracion explicita de recepcion")

    for destino in RE_LINK.findall(texto):
        if destino.startswith(("http://", "https://", "#", "mailto:")):
            continue
        limpio = destino.split("#", 1)[0].strip()
        if limpio and not (MATRIZ.parent / limpio).exists() and not (ROOT / limpio).exists():
            problemas.append(f"enlace interno roto: {destino}")

    resumen = {
        "filas": len(filas),
        "auditorias_semanticas": auditadas,
        "ses": sorted(SES),
        "problemas": len(problemas),
    }
    print(json.dumps(resumen, ensure_ascii=False, indent=2))
    for problema in problemas:
        print(f"  - {problema}")
    if problemas:
        print("AUDITORIA_54_FAIL")
        return 1
    print("AUDITORIA_54_PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
