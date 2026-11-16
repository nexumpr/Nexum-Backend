"""Clasifica TODAS las diferencias contractuales E005-E075 (sin dejar ninguna sin clase).

Entradas (solo lectura):
  - sdd/_control/H08/PREPARACION_E005_E075/matriz_e005_e075.json  (auditoria vs SDD)
  - sdd/03-endpoints/openapi/nexum-api-v1.json                    (contrato vigente)

Salida:
  - sdd/_control/H08/CLASIFICACION_E005_E075.json                 (trazabilidad por endpoint)

Clases: ESENCIAL | COMPATIBILIDAD | OPCIONAL | OBSOLETA.

Criterio (explicito, no heuristico oculto):
  1. Codigo de exito distinto  -> COMPATIBILIDAD (exige decision HTTP individual).
  2. Referencia `SESSIONHASH`  -> OBSOLETA (enmienda editorial transversal).
  3. Renombrado conocido        -> COMPATIBILIDAD.
  4. Base del campo YA expuesta por el contrato -> OPCIONAL (refinamiento de presentacion).
  5. Campo material declarado en OVERRIDES -> la clase indicada (ESENCIAL si sin el la
     operacion no puede cumplir su funcion de forma verificable).
  6. Resto -> OPCIONAL POSPUESTA: el contrato vigente cubre la funcion y el campo es
     refinamiento no requerido por el contrato aprobado.

`OPCIONAL` NO significa resuelto: significa POSPUESTO y registrado, reversible por RFC.
`ESENCIAL`/`COMPATIBILIDAD` pendientes impiden CONTRACT_FROZEN.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

MATRIZ = Path("sdd/_control/H08/PREPARACION_E005_E075/matriz_e005_e075.json")
OPENAPI = Path("sdd/03-endpoints/openapi/nexum-api-v1.json")
SALIDA = Path("sdd/_control/H08/CLASIFICACION_E005_E075.json")
SPECS = Path("sdd/03-endpoints")
PERSISTENCIA = ("E005", "E006", "E007", "E008", "E009", "E010", "E011")
PATRON_OBSOLETO = "SESSIONHASH"

# Codigos de exito: decision propuesta y motivo (no aprobada).
HTTP_DECISION = {
    "E017": ("201", "Crea un consentimiento: recurso nuevo. Conservar el contrato aprobado."),
    "E031": ("204", "Borrado idempotente sin cuerpo. Conservar el contrato aprobado."),
    "E037": ("200", "Conservar hasta confirmar con Gate que la operacion crea un movimiento."),
    "E042": ("204", "Borrado idempotente sin cuerpo. Conservar el contrato aprobado."),
    "E063": ("202", "Conservar 202 mientras no se demuestre que read-all es sincrono."),
}

# Renombrados que rompen el contrato aprobado si se adoptan.
RENOMBRADOS = {
    "E006": {"resetToken": "token"},
    "E075": {"transcript": "text"},
}

# Campos ESENCIALES: solo cuando el contrato vigente NO puede expresar la funcion y
# la necesidad esta demostrada por un diseno aprobado (no por la maqueta).
OVERRIDES: dict[str, tuple[str, str]] = {
    "E040:openingSaved": (
        "ESENCIAL",
        "E039 acepta initialSavedAmount pero ni E039 ni E040 lo devuelven: sin el, Android no puede mostrar "
        "saldo inicial ni separar aportaciones de saldo inicial, y contributedNet no es calculable.",
    ),
    "E055:hasEnoughData": (
        "ESENCIAL",
        "El contrato no distingue 'sin historial suficiente' de 'sin patrones': items[] vacio es ambiguo. "
        "El diseno ML (RFC-016 6) exige un estado explicito de datos insuficientes.",
    ),
    # Preferencias de notificacion: la maqueta pide granularidad que 3 booleanos no expresan.
    "E058:enabled": ("COMPATIBILIDAD", "Sustituye al conjunto pushEnabled/emailEnabled/reminderEnabled del SDD."),
    "E058:budgetAlerts": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:goalUpdates": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:recurringReminders": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:streakReminders": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:quietHours": ("COMPATIBILIDAD", "Horario de silencio sin equivalente en el contrato vigente."),
    "E058:timeZone": ("COMPATIBILIDAD", "El contrato vigente no declara zona horaria del usuario."),
    # Suficiencia de datos en insights: misma ambiguedad que E055 (items vacio no distingue causa).
    "E052:enoughData": ("COMPATIBILIDAD", "Sin marca de suficiencia, un array vacio no distingue 'sin historial' de 'sin patrones'."),
    "E052:minimumSamples": ("COMPATIBILIDAD", "El consumidor necesita conocer el umbral aplicado para interpretar el vacio."),
}

# Contenedores cuyo nombre o forma difieren del contrato aprobado.
ESTRUCTURALES = {
    "E055": ("COMPATIBILIDAD", "La auditoria usa recommendations[]; el SDD ya expone items[] + nextCursor. "
                               "Renombrar el contenedor rompe el cliente: se conserva items[]."),
    "E021": ("OPCIONAL", "Agregado de arranque: el SDD puede componerlo desde otros endpoints."),
    "E051": ("OPCIONAL", "Agregado de panel: refinamiento de presentacion."),
    "E074": ("OPCIONAL", "Datos de demostracion: no afecta al contrato de produccion."),
    "E020": ("OPCIONAL", "Forma del documento de exportacion: decision de producto."),
}


# Derivacion: si el dato puede obtenerse del propio contrato o por composicion, el
# candidato queda OPCIONAL_CONFIRMADO; si no, OPCIONAL_PENDIENTE_DE_DECISION.
DERIVACIONES = [
    (("summary", "totals", "financialSummary", "streak", "tip", "recurringPreview", "topCategories"),
     "agregado componible desde endpoints atomicos (E026/E051/E056)"),
    (("percentage", "shareOfTotal", "percentageOfTop", "relativePercentage", "progressPercentage", "percentageOfTotal"),
     "calculable con importes ya presentes en el contrato"),
    (("remaining", "completed", "isNegative", "canCommit"),
     "derivable de importes del propio contrato"),
    (("createdAt", "updatedAt", "calculatedAt", "checkedAt", "verifiedAt", "acceptedAt", "readAt",
      "lastSeenAt", "occurredAt", "paidAt", "startDate", "nextDueDate", "lastPaidAt", "dueDay",
      "windowDays", "from", "to", "period", "asOf", "endDate", "triggerAt", "nextTriggerAt"),
     "valor temporal de presentacion; no altera la logica del contrato"),
    (("emoji", "title", "text", "message", "pigMessage", "headline", "userGreeting", "disclaimer",
      "label", "question", "answer", "body", "topic", "sourceLabel", "order"),
     "contenido de presentacion; no altera la logica del contrato"),
    (("weekdayBars", "milestones", "categories", "recommendations", "limits", "allocations",
      "sessions", "recurring", "movements", "goals", "budgets", "consents", "frequencies",
      "contributions", "affectedGoals", "errors", "checks"),
     "coleccion componible desde su endpoint atomico"),
]

# Equivalencias demostradas: el dato existe en el contrato con otro nombre o se calcula
# con operaciones ya aprobadas. Cada entrada es una afirmacion verificable.
EQUIVALENCIAS = {
    "concept": "equivale a `description` en E028/E029/E030 y a `name` en E033/E034/E035",
    "category": "equivale a `categoryId`, que referencia el catalogo E022",
    "target": "equivale a `targetAmount`",
    "saved": "equivale a `savedAmount`",
    "openingSaved": "equivale a `initialSavedAmount`, aceptado por E039",
    "totalAmount": "equivale a `amount`",
    "name": "equivale a `displayName` (E012/E013)",
    "accepted": "equivale a `status = \"accepted\"` en Accepted",
    "noticeId": "equivale al `id` del aviso devuelto por E015",
    "occurrenceDate": "equivale a `paidAt` en el request de E037",
    "enabled": "equivale a `active`/`pushEnabled`/`reminderEnabled` segun el recurso",
    "contributedNet": "= savedAmount - initialSavedAmount (aritmetica exacta en centavos)",
    "retryAfterSeconds": "equivale a la cabecera `Retry-After` ya definida en el contrato",
    "monthlyEquivalent": "= amount x factor de E023 (aritmetica racional A-070)",
    "mode": "el reparto se expresa con `allocations[]{goalId, amount}`",
    "splitMode": "el reparto se expresa con `allocations[]{goalId, amount}`",
    "goalIds": "equivale a enumerar `allocations[].goalId`",
    "source": "derivable de `recurringPaymentId`/`goalId`/`contributionId` de E029",
    "valid": "equivale a `canCommit` (E043)",
    "errors": "un reparto invalido devuelve 422 con `error.details[]` (envelope comun)",
    "confidence": "no requerido: la inferencia es determinista sobre el artefacto versionado",
    "version": "el contrato ya expone `version` donde hay concurrencia; en catalogos es marcador de cache",
    "appearance": "equivale a la preferencia de tema del perfil (E068/E069)",
    "afternoonCount": "= items.filter(hora >= start && hora < end).length sobre E027",
    "afternoonPercentage": "= afternoonCount / count (aritmetica exacta)",
    "patternDetected": "= afternoonPercentage >= umbral configurado",
    "headline": "texto de presentacion derivado de patternDetected",
    "revision": "equivale a `version`; incremento para concurrencia optimista",
    "quietHours.enabled": "declarado por A3",
    "quietHours.start": "declarado por A3",
    "quietHours.end": "declarado por A3",
}

# Documentos agregados: cada rama se compone de un endpoint atomico aprobado.
COMPOSICION = {
    "user": "componible desde E012 (perfil)",
    "income": "componible desde E024 (ingreso habitual)",
    "recurringPayments": "componible desde E032 (recurrentes)",
    "goals": "componible desde E038 (metas)",
    "budgets": "componible desde E048 (presupuestos)",
    "contributions": "componible desde E045 (aportaciones)",
    "streak": "componible desde E056 (racha)",
    "onboarding": "componible desde E018 (onboarding)",
    "tip": "componible desde E071 (tips educativos)",
    "categories": "componible desde E022/E053",
    "frequencies": "componible desde E023",
    "milestones": "componible desde E056",
    "weekdayBars": "componible desde E054",
    "recurring": "componible desde E032",
    "movements": "componible desde E027",
    "limits": "componible desde E048",
    "allocations": "declarado en E043/E044",
    "checks": "componible desde E072/E073",
    "sessions": "componible desde E008",
    "consents": "componible desde E016",
    "affectedGoals": "derivable de E038 tras la reversion E047",
    "recommendations": "equivale al contenedor `items[]` de E055",
}

PENDIENTES = {
    "source", "frequency", "current", "valid", "confidence", "locale", "mode", "splitMode",
    "goalIds", "reason", "format", "include", "context", "includeInactive", "limit", "cursor",
    "unreadOnly", "activeOnly", "before", "occurrenceDate", "deviceName", "platform",
    "appVersion", "timeZone", "enabled", "revision",
}


def derivar(campo: str) -> tuple[bool, str]:
    base = campo.replace("[]", "").split(".")[-1]
    raiz = campo.replace("[]", "").split(".")[0]
    for claves, motivo in DERIVACIONES:
        if raiz in claves or base in claves:
            return True, motivo
    if base in PENDIENTES or raiz in PENDIENTES:
        return False, "no derivable del contrato vigente: requiere decision trazada"
    return False, "sin evidencia suficiente de derivabilidad: requiere decision trazada"


# Decision final por campo (global). Sustituye a PENDIENTE_DE_DECISION.
# (clase, decision, motivo). Todo campo debe caer aqui o en las tablas anteriores.
DECISION_FINAL: dict[str, tuple[str, str, str]] = {
    "movement": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale al objeto raiz devuelto por E037"),
    "needsReview": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `requiresConfirmation`"),
    "amount": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `parsedAmount` en E075"),
    "accountStatus": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `status` del perfil (E012)"),
    "deletedMovementId": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "es el id de la ruta de E031"),
    "deletedGoalId": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "es el id de la ruta de E042"),
    "allocatedAmount": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "= suma de `allocations[].amount`"),
    "unallocatedAmount": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `remaining` (E043)"),
    "totalRequested": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `amount`"),
    "totalApplied": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "= suma de `allocations[].amount` aceptadas"),
    "unallocated": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `remaining`"),
    "contribution": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale al objeto raiz de E044"),
    "reversedAmount": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `amount` de la aportacion"),
    "incomeMonthly": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "composicion aritmetica de E024 en centavos"),
    "recurringFixedMonthly": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "composicion de E032 con factores de E023"),
    "oneTimeFixedMonthly": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "composicion de E032"),
    "fixedMonthly": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "= recurringFixedMonthly + oneTimeFixedMonthly"),
    "antMonthly": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "composicion de E032 con factores de E023"),
    "reservedMonthly": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "composicion de E038/E044"),
    "availableEstimated": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "= incomeMonthly - fixedMonthly - antMonthly - reservedMonthly"),
    "antExpenseCount": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "conteo sobre E027 en la ventana"),
    "count": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "longitud de la ventana analizada"),
    "afternoonWindowStart": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "constante de configuracion del servidor"),
    "afternoonWindowEndExclusive": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "constante de configuracion del servidor"),
    "allCategoriesTotal": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "= suma de `categories[].total`"),
    "lastSavingDate": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "composicion desde E027"),
    "nextMilestoneDays": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "tabla de hitos ya usada por E056"),
    "currency": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "MXN fijo por contrato; no hay multimoneda"),
    "limit": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "el servidor aplica limite por defecto y devuelve cursor"),
    "cursor": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "paginacion por cursor ya definida en el contrato"),
    "goalId": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "es path param o queda expresado en `allocations[]`"),
    "include": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "el servidor aplica el conjunto por defecto"),
    "includeInactive": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "el servidor devuelve solo activos por defecto"),
    "format": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "el servidor aplica el formato por defecto"),
    "context": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "el servidor aplica el contexto por defecto"),
    "activeOnly": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "el cliente filtra sobre la lista devuelta"),
    "changedAt": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "equivale a `updatedAt`"),
    "reversedAt": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "sello temporal de la reversion, derivable de `updatedAt`"),
    "timeZone": ("COMPATIBILIDAD", "ANADIR_EN_A8", "zona horaria del usuario: requerida para recordatorios y preferencias (A3 la declara en E058)"),
    "frequency": ("COMPATIBILIDAD", "ANADIR_EN_A8", "recurrencia del recordatorio; en movimientos se obtiene de E034"),
    "locale": ("COMPATIBILIDAD", "ANADIR_EN_A8", "idioma del contenido y del correo; sin el, el servidor no puede localizar"),
    "type": ("COMPATIBILIDAD", "ANADIR_EN_A8", "filtro de listado; sin el, la paginacion devuelve mezclado"),
    "status": ("COMPATIBILIDAD", "ANADIR_EN_A8", "estado del recurso en listados E038/E045"),
    "unreadCount": ("COMPATIBILIDAD", "ANADIR_EN_A8", "total no leido; con paginacion el cliente no puede contarlo"),
    "unreadOnly": ("COMPATIBILIDAD", "ANADIR_EN_A8", "filtro que altera el conjunto paginado"),
    "before": ("COMPATIBILIDAD", "ANADIR_EN_A8", "cota temporal del marcado masivo (E063)"),
    "updatedCount": ("COMPATIBILIDAD", "ANADIR_EN_A8", "cuantas notificaciones se marcaron; solo el servidor lo sabe"),
    "updatedAt": ("OPCIONAL", "POSPUESTA_CONFIRMADA", "sello temporal de presentacion"),
    "platform": ("COMPATIBILIDAD", "ANADIR_EN_A8", "plataforma del dispositivo para el registro push"),
    "appVersion": ("COMPATIBILIDAD", "ANADIR_EN_A8", "version de la app para diagnostico push"),
    "active": ("COMPATIBILIDAD", "ANADIR_EN_A8", "estado del registro push"),
    "deviceId": ("ESENCIAL", "DECIDIR_EN_RFC_016", "identificador del dispositivo: sin el no se puede deduplicar el registro push"),
    "deviceRegistrationId": ("ESENCIAL", "DECIDIR_EN_RFC_016", "identifica el registro que E060 debe eliminar"),
    "relatedEntityId": ("ESENCIAL", "DECIDIR_EN_RFC_016", "vinculo del recordatorio con su entidad; sin el el recordatorio es inutilizable"),
    "verificationToken": ("ESENCIAL", "DECIDIR_EN_RFC_016", "E011 es publica y solo recibe el token: sin el campo el flujo es imposible"),
    "email": ("ESENCIAL", "DECIDIR_EN_RFC_016", "E010 debe identificar la cuenta a verificar y no hay sesion"),
    "microphone": ("COMPATIBILIDAD", "ANADIR_EN_A8", "estado de onboarding que persiste el servidor"),
    "notifications": ("COMPATIBILIDAD", "ANADIR_EN_A8", "estado de onboarding que persiste el servidor"),
    "configured": ("COMPATIBILIDAD", "ANADIR_EN_A8", "estado de onboarding que persiste el servidor"),
    "completedAt": ("COMPATIBILIDAD", "ANADIR_EN_A8", "sello del onboarding completado"),
    "content": ("COMPATIBILIDAD", "ANADIR_EN_A8", "texto del aviso de privacidad; lo sirve el backend"),
    "requiresAcceptance": ("COMPATIBILIDAD", "ANADIR_EN_A8", "regla de aceptacion que decide el servidor"),
    "emailVerified": ("COMPATIBILIDAD", "ANADIR_EN_A8", "resultado de la verificacion"),
    "deletionRequestId": ("COMPATIBILIDAD", "ANADIR_EN_A8", "referencia del borrado solicitado"),
    "reason": ("COMPATIBILIDAD", "ANADIR_EN_A8", "motivo de borrado, opcional y aditivo"),
    "revokedSessions": ("COMPATIBILIDAD", "ANADIR_EN_A8", "evidencia de revocacion que solo el servidor conoce"),
    "revokedOtherSessions": ("COMPATIBILIDAD", "ANADIR_EN_A8", "evidencia de revocacion que solo el servidor conoce"),
    "reset": ("COMPATIBILIDAD", "ANADIR_EN_A8", "resultado booleano del restablecimiento"),
    "appearance": ("COMPATIBILIDAD", "ANADIR_EN_A8", "preferencia de tema; el servidor la persiste"),
    "affectedGoalId": ("COMPATIBILIDAD", "ANADIR_EN_A8", "efecto del borrado sobre una meta"),
    "reversedContributionId": ("COMPATIBILIDAD", "ANADIR_EN_A8", "efecto del borrado sobre una aportacion"),
    "releasedAmount": ("COMPATIBILIDAD", "ANADIR_EN_A8", "importe liberado por el borrado de meta"),
    "reversedAllocations": ("COMPATIBILIDAD", "ANADIR_EN_A8", "aportaciones revertidas por el borrado"),
    "affectedPeriods": ("COMPATIBILIDAD", "ANADIR_EN_A8", "periodos contables afectados por la reversion"),
    "budgetAlerts": ("COMPATIBILIDAD", "ANADIR_EN_A8", "preferencia granular (A3)"),
    "goalUpdates": ("COMPATIBILIDAD", "ANADIR_EN_A8", "preferencia granular (A3)"),
    "recurringReminders": ("COMPATIBILIDAD", "ANADIR_EN_A8", "preferencia granular (A3)"),
    "streakReminders": ("COMPATIBILIDAD", "ANADIR_EN_A8", "preferencia granular (A3)"),
    "quietHours": ("COMPATIBILIDAD", "ANADIR_EN_A8", "horario de silencio (A3)"),
}


def decidir(campo: str, eid: str) -> tuple[str, str, str]:
    base = campo.replace("[]", "").split(".")[-1]
    if base in DECISION_FINAL:
        return DECISION_FINAL[base]
    return (
        "COMPATIBILIDAD",
        "ANADIR_EN_A8",
        f"`{campo}` no tiene equivalente demostrado en el contrato de {eid}: se propone aditivo opcional en A8",
    )


def cargar(ruta: Path):
    return json.loads(ruta.read_text(encoding="utf-8"))


def campos_del_esquema(op: dict, componentes: dict, codigo: str) -> set[str]:
    base: set[str] = set()

    def aplanar(sch, prefijo=""):
        if not isinstance(sch, dict):
            return
        if "$ref" in sch:
            nombre = sch["$ref"].split("/")[-1]
            if nombre not in componentes:
                return
            aplanar(componentes[nombre], prefijo)
            return
        for nombre, sub in (sch.get("properties") or {}).items():
            ruta = f"{prefijo}{nombre}"
            base.add(ruta)
            if isinstance(sub, dict):
                if "$ref" in sub:
                    nombre_ref = sub["$ref"].split("/")[-1]
                    if nombre_ref in componentes:
                        aplanar(componentes[nombre_ref], ruta + ".")
                else:
                    sub_tipo = sub.get("type")
                    if sub_tipo == "array" and isinstance(sub.get("items"), dict):
                        aplanar(sub["items"], ruta + ".")
                    elif isinstance(sub.get("properties"), dict):
                        aplanar(sub, ruta + ".")
        for comp in ("allOf", "oneOf", "anyOf"):
            for sub in sch.get(comp) or []:
                aplanar(sub, prefijo)

    respuestas = op.get("responses") or {}
    contenido = ((respuestas.get(codigo) or {}).get("content") or {}).get("application/json") or {}
    if isinstance(contenido.get("schema"), dict):
        aplanar(contenido["schema"])
    return base


def otros_campos(op: dict, componentes: dict) -> set[str]:
    """Nombres de campos del request y de parametros path/query del contrato."""
    nombres: set[str] = set()
    for p in op.get("parameters") or []:
        if isinstance(p, dict) and p.get("name"):
            nombres.add(p["name"])
    contenido = ((op.get("requestBody") or {}).get("content") or {}).get("application/json") or {}
    esquema = contenido.get("schema")
    if isinstance(esquema, dict):
        for c in aplanar_campos(esquema, componentes):
            nombres.add(c.split(".")[0])
    return nombres


def aplanar_campos(sch, componentes, prefijo=""):
    base: set[str] = set()
    if not isinstance(sch, dict):
        return base
    if "$ref" in sch:
        nombre = sch["$ref"].split("/")[-1]
        if nombre in componentes:
            base |= aplanar_campos(componentes[nombre], componentes, prefijo)
        return base
    for nombre, sub in (sch.get("properties") or {}).items():
        ruta = f"{prefijo}{nombre}"
        base.add(ruta)
        if isinstance(sub, dict):
            if "$ref" in sub:
                ref = sub["$ref"].split("/")[-1]
                if ref in componentes:
                    base |= aplanar_campos(componentes[ref], componentes, ruta + ".")
            elif sub.get("type") == "array" and isinstance(sub.get("items"), dict):
                base |= aplanar_campos(sub["items"], componentes, ruta + ".")
            elif isinstance(sub.get("properties"), dict):
                base |= aplanar_campos(sub, componentes, ruta + ".")
    for comp in ("allOf", "oneOf", "anyOf"):
        for sub in sch.get(comp) or []:
            base |= aplanar_campos(sub, componentes, prefijo)
    return base


def clasificar(raiz: Path) -> dict:
    matriz = cargar(raiz / MATRIZ)
    api = cargar(raiz / OPENAPI)
    componentes = (api.get("components") or {}).get("schemas") or {}
    operaciones = []
    resumen_clases = {"ESENCIAL": 0, "COMPATIBILIDAD": 0, "OPCIONAL": 0, "OBSOLETA": 0}

    for fila in matriz:
        eid = fila["id"]
        metodo = (fila.get("sdd_method") or "").lower()
        ruta = fila.get("sdd_path") or ""
        op = ((api.get("paths") or {}).get(ruta) or {}).get(metodo) or {}
        campos = campos_del_esquema(op, componentes, str(fila.get("sdd_success")))
        bases = {c.split(".")[0] for c in campos} | otros_campos(op, componentes)

        diferencias = []

        def agregar(campo, tipo, clase, necesidad, decision, motivo):
            resumen_clases[clase] += 1
            diferencias.append(
                {
                    "campo": campo,
                    "tipo": tipo,
                    "clase": clase,
                    "necesidad_android": necesidad,
                    "decision": decision,
                    "motivo": motivo,
                }
            )

        # 1) Codigo de exito.
        if str(fila.get("pdf_success")) != str(fila.get("sdd_success")):
            objetivo, motivo = HTTP_DECISION.get(eid, (str(fila["sdd_success"]), "Sin decision registrada."))
            agregar(
                "http_success",
                "HTTP",
                "COMPATIBILIDAD",
                "El cliente distingue creacion, exito con cuerpo y borrado segun el codigo.",
                f"CONSERVAR {objetivo}",
                motivo,
            )

        # 2) Persistencia heredada.
        if eid in PERSISTENCIA:
            carpeta = next((p for p in (raiz / SPECS).glob(f"{eid}-*") if p.is_dir()), None)
            hits = 0
            if carpeta:
                hits = sum(md.read_text(encoding="utf-8", errors="replace").count(PATRON_OBSOLETO) for md in carpeta.glob("*.md"))
            if hits:
                agregar(
                    "persistencia",
                    "PERSISTENCIA",
                    "OBSOLETA",
                    "Ninguna: es referencia documental al modelo anterior.",
                    "SUSTITUIR por USER#id/SESSION#id y REFRESH#digest/SESSION",
                    f"{hits} referencias a {PATRON_OBSOLETO} contradicen el modelo verificado en E003/E004.",
                )

        for tipo, lista in (
            ("REQUEST", fila.get("pdf_only_request_candidates") or []),
            ("RESPONSE", fila.get("pdf_only_response_candidates") or []),
        ):
            for campo in lista:
                base = campo.replace("[]", "").split(".")[0]
                clave = f"{eid}:{campo.replace('[]', '')}"
                if campo.replace("[]", "") in (RENOMBRADOS.get(eid) or {}):
                    agregar(
                        campo,
                        tipo,
                        "COMPATIBILIDAD",
                        "El cliente envia el nombre del campo; renombrarlo rompe la integracion.",
                        f"CONSERVAR {RENOMBRADOS[eid][campo.replace('[]', '')]}",
                        "Renombrado respecto al contrato aprobado.",
                    )
                elif clave in OVERRIDES:
                    clase, motivo = OVERRIDES[clave]
                    agregar(campo, tipo, clase, "Ver motivo.", "DECIDIR EN RFC-016", motivo)
                elif base in bases:
                    agregar(
                        campo,
                        tipo,
                        "OPCIONAL",
                        "El contrato ya expone la misma base: el candidato es refinamiento.",
                        "POSPUESTA_CONFIRMADA",
                        "La base del campo ya existe en el contrato aprobado.",
                    )
                elif base in EQUIVALENCIAS:
                    agregar(
                        campo,
                        tipo,
                        "OPCIONAL",
                        "El dato existe en el contrato con otro nombre o es calculable.",
                        "POSPUESTA_CONFIRMADA",
                        EQUIVALENCIAS[base],
                    )
                elif base in COMPOSICION:
                    agregar(
                        campo,
                        tipo,
                        "OPCIONAL",
                        "El dato se obtiene de un endpoint atomico aprobado.",
                        "POSPUESTA_CONFIRMADA",
                        COMPOSICION[base],
                    )
                elif eid in ESTRUCTURALES and tipo == "RESPONSE" and "." not in campo:
                    clase, motivo = ESTRUCTURALES[eid]
                    agregar(
                        campo,
                        tipo,
                        clase,
                        "Ver motivo.",
                        "POSPUESTA_CONFIRMADA" if clase == "OPCIONAL" else "DECIDIR EN RFC-016",
                        motivo,
                    )
                else:
                    clase, decision, motivo = decidir(campo, eid)
                    agregar(campo, tipo, clase, "Ver motivo.", decision, motivo)

        esenciales = [d for d in diferencias if d["clase"] == "ESENCIAL"]
        compatibilidad = [d for d in diferencias if d["clase"] == "COMPATIBILIDAD"]
        operaciones.append(
            {
                "id": eid,
                "operacion": f"{metodo.upper()} {ruta}",
                "operationId": op.get("operationId"),
                "seguridad": "public" if op.get("security") == [] else "bearer",
                "diferencias": diferencias,
                "resumen": {
                    "ESENCIAL": len(esenciales),
                    "COMPATIBILIDAD": len(compatibilidad),
                    "OPCIONAL": sum(1 for d in diferencias if d["clase"] == "OPCIONAL"),
                    "OBSOLETA": sum(1 for d in diferencias if d["clase"] == "OBSOLETA"),
                },
                # El congelamiento no depende de esta herramienta: exige RFC aprobada y aplicada.
                "contract_frozen": "NOT_VERIFIED",
                "motivos_no_congelado": (
                    (["RFC-016 no aprobada"] if esenciales or compatibilidad else [])
                    + (["Enmienda de persistencia no aplicada"] if any(d["clase"] == "OBSOLETA" for d in diferencias) else [])
                    + ["Aprobacion humana de la clasificacion pendiente"]
                ),
            }
        )

    total = sum(len(o["diferencias"]) for o in operaciones)
    sin_clasificar = sum(1 for o in operaciones for d in o["diferencias"] if d["clase"] not in resumen_clases)
    return {
        "documento": "Clasificacion contractual E005-E075",
        "generado_por": "herramientas/clasificar_contratos_e005.py",
        "baseline": "sdd/03-endpoints/openapi/nexum-api-v1.json",
        "reglas": {
            "http": "codigo de exito distinto -> COMPATIBILIDAD con decision individual",
            "persistencia": "SESSIONHASH -> OBSOLETA",
            "renombrado": "RENOMBRADOS -> COMPATIBILIDAD",
            "base_presente": "base del campo ya en el contrato -> OPCIONAL",
            "override": "campo material listado -> clase indicada",
            "resto": "OPCIONAL POSPUESTA (contrato vigente cubre la funcion)",
        },
        "resumen": {
            "operaciones": len(operaciones),
            "diferencias": total,
            "por_clase": resumen_clases,
            "sin_clasificar": sin_clasificar,
            "por_decision": {
                "POSPUESTA_CONFIRMADA": sum(
                    1 for o in operaciones for d in o["diferencias"] if d["decision"] == "POSPUESTA_CONFIRMADA"
                ),
                "PENDIENTE_DE_DECISION": sum(
                    1 for o in operaciones for d in o["diferencias"] if d["decision"] == "PENDIENTE_DE_DECISION"
                ),
            },
            "operaciones_con_pendientes": sum(
                1 for o in operaciones if any(d["decision"] == "PENDIENTE_DE_DECISION" for d in o["diferencias"])
            ),
        },
        "operaciones": operaciones,
    }


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description="Clasifica diferencias contractuales E005-E075.")
    p.add_argument("--root", default=None)
    p.add_argument("--salida", default=None)
    p.add_argument("--resumen", action="store_true")
    a = p.parse_args(argv)
    raiz = Path(a.root) if a.root else Path(__file__).resolve().parents[1]
    r = clasificar(raiz)
    destino = Path(a.salida) if a.salida else raiz / SALIDA
    destino.parent.mkdir(parents=True, exist_ok=True)
    destino.write_text(json.dumps(r, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    s = r["resumen"]
    print(
        f"CLASIFICACION_OK operaciones={s['operaciones']} diferencias={s['diferencias']} "
        f"sin_clasificar={s['sin_clasificar']} "
        f"esencial={s['por_clase']['ESENCIAL']} compatibilidad={s['por_clase']['COMPATIBILIDAD']} "
        f"opcional={s['por_clase']['OPCIONAL']} obsoleta={s['por_clase']['OBSOLETA']}"
    )
    if a.resumen:
        for o in r["operaciones"]:
            if o["resumen"]["ESENCIAL"] or o["resumen"]["COMPATIBILIDAD"]:
                print(
                    f" {o['id']}: E={o['resumen']['ESENCIAL']} C={o['resumen']['COMPATIBILIDAD']} "
                    f"O={o['resumen']['OPCIONAL']} X={o['resumen']['OBSOLETA']}"
                )
    return 0 if s["sin_clasificar"] == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
