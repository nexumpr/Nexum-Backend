# Gate H08-A — Hotfix A-050 (gosec G115 / endurecimiento de Argon2)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08A_HOTFIX_A050_G115.md` |
| Fecha | 2026-11-06 |
| Defecto | **D-10** — `gosec G115` + parámetros Argon2 sin acotar en `VerifyPassword` |
| Spec | A-050 (seguridad e identidad) |
| Base | `main` en `10f632799f00c998aaf648e1cb9d27c4b3e5bf52` |
| Rama | `fix/h08a-a050-gosec-g115` |
| Estado | **APROBADO — autorización humana expresa** (`APRUEBO_FIX_A050_GOSEC_G115`) |

## 1. Causa raíz verificada

`Argon2Hasher.VerifyPassword` tomaba los parámetros del **propio hash almacenado** sin acotarlos:

| Línea | Problema |
|---|---|
| 106 | `Sscanf(parts[3], "m=%d,t=%d,p=%d")` → un hash manipulado podía pedir memoria/iteraciones enormes → **agotamiento de memoria/CPU** |
| 126 | `uint32(len(expectedHash))` → `gosec G115`; longitud del digest sin validar |

`G115` era síntoma: el defecto real era **falta de validación de política y longitudes antes de derivar la clave**.

## 2. Corrección aplicada (solo A-050)

- Guardas anti-DoS con límites explícitos y documentados: memoria 8–256 MiB, iteraciones 1–10,
  paralelismo 1–16, sal 8–64 bytes, digest entre las longitudes admitidas {16,24,32,48,64}, hash
  codificado ≤ 4096 caracteres.
- La longitud del digest se obtiene de un mapa de **constantes uint32** (`allowedKeyLengths`), eliminando
  la conversión `int→uint32` **sin** `#nosec` ni exclusiones.
- El rechazo ocurre **antes** de `argon2.IDKey`.
- Sin cambios de algoritmo, formato `$argon2id$v=19$...`, parámetros por defecto (m=64 MiB, t=3, p=2,
  keyLen=32) ni comparación en tiempo constante.

## 3. Pruebas

`tests/unit/security/hasher_hardening_test.go`:

- hash válido, contraseña correcta/incorrecta (contrato `(false, ErrInvalidPassword)`);
- estructura inválida (partes, algoritmo, versión, parámetros corruptos, hash gigantesco);
- parámetros peligrosos rechazados **en < 250 ms** (memoria máxima uint32, iteraciones máximas, p=255…);
- longitudes de sal/digest inválidas;
- compatibilidad con hash de formato canónico;
- ausencia de pánicos con entradas adversas.

## 4. Evidencia

| Control | Resultado |
|---|---|
| `go build` / `go vet` / `go test ./...` | PASS |
| `gosec v2.30.0` | **PASS** (0 hallazgos; antes 1 HIGH G115) |
| `govulncheck v1.9.0` | PASS (0 vulnerabilidades invocadas) |
| `gofmt -l` (archivos del hotfix) | limpio |

## 5. Dependencia de CI (bootstrap)

El workflow Go vive únicamente en PR #19 (A-100). Este PR de A-050, nacido de `main`, **no tendrá CI Go
automático** hasta que A-100 se integre. La evidencia anterior es local y verificable. Cualquier merge de
este PR requiere una autorización independiente sobre su HEAD, con o sin excepción de bootstrap.
