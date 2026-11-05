# Inventario técnico de fuentes de H04

| Archivo analizado | Observación reproducible | SHA-256 de fragmento/fuente |
|---|---|---|
| Android app.js | 70296 bytes, sin `fetch(` | `193FDCEF454959036F9142D709E272861D92CDA7C5DA89F507FC6820127D8308` |
| Android index.html | 39372 bytes | `C0752F9CBB5659E0B4537F6D527AC28975FC791FC8E84DD3C948AC8F1809BB7A` |
| Android MainActivity.java | 7280 bytes | `5B4E93027644186FFBF3DE9D942551D5B62F26D2D60C93F2D3E4682BB7B4991D` |
| Android AndroidManifest.xml | 803 bytes | `3C113A53C5921B8826E2A0B85A19D7798CC731729EB2003D24D01279CBADA020` |
| Android README.md | 2793 bytes | `FDB7FBE59AE75B8354F6CD99A6E28AC58CBDFF9A61808DFA355228F91244F357` |
| Catálogo PDF 27 páginas | 75 IDs E001..E075 | `D8E955F1006F013DEDE4FFB1277BCD4D13DDCA7F6157855AE46F75B0494FD772` |

## Terminología y diferencias

- **Prototipo:** comportamiento local real de V6, no implementación REST.
- **Catálogo:** propuesta de contratos para 75 rutas. No elegir MongoDB ni Google Cloud del documento histórico.
- **Docente:** requerimientos académicos más amplios y mínimos operativos, adicionales a la maqueta.
- **Constitución:** decisiones vigentes para Nexum-Backend y límites de responsabilidad.

## Referencias externas técnicas usadas con fecha de consulta 2026-11-05

- Go: https://go.dev/dl/ — rama 1.27.3 mostrada para congelar toolchain al implementar.
- DynamoDB TransactWriteItems: https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/API_TransactWriteItems.html — 100 acciones, 4 MB.
- DynamoDB TTL: https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/TTL.html — eliminación eventual, no mecanismo de revocación instantánea.
- AWS Free Tier: https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/free-tier-plans.html — créditos y servicios elegibles deben verificarse en cuenta.
- OpenTelemetry Go: https://opentelemetry.io/docs/languages/go/ — trazas/métricas/logs.
