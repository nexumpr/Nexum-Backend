package dynamo_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
)

// Pruebas de los punteros de purga introducidos para que E014 alcance sin `Scan` dos
// residuos que antes sólo caducaban por TTL:
//   - los tokens de un solo uso (`RESET#<sha256>` de E005/E006 y `ONE_USE#<sha256>` de
//     E010/E011), referenciados por `USER#<sub>/TOKENPTR#<sha256>`;
//   - el dedupe de entrega de E062 (`OUTBOX#GLOBAL/IDEM#<sha256>`), referenciado por
//     `USER#<sub>/DELIVERYPTR#<sha256(eventId)>`.
//
// Se ejecutan sobre el adaptador REAL y el worker REAL con el doble con estado, que
// interpreta las mismas condiciones, transacciones y consultas que emite el adaptador.

// e014Hash reproduce la forma canónica del digest: SHA-256 hexadecimal en minúscula.
func e014Hash(texto string) string {
	suma := sha256.Sum256([]byte(texto))
	return hex.EncodeToString(suma[:])
}

// e014SembrarToken escribe el ítem global de un token de un solo uso y su puntero, con la
// misma forma que producen E005 (`RESET`) y E010 (`ONE_USE`). Devuelve la clave global, la
// SK global y la SK del puntero.
func e014SembrarToken(m *purgaDynamo, userID, family, hash string, expira time.Time) (string, string, string) {
	skGlobal, _ := domainDeletion.TokenGlobalSK(family)
	pkGlobal := family + "#" + hash
	m.sembrar(pkGlobal, skGlobal, map[string]types.AttributeValue{
		"userId":    strAttr(userID),
		"expiresAt": numAttr(expira.Unix()),
		"used":      &types.AttributeValueMemberBOOL{Value: false},
		"createdAt": strAttr(e014T0.Format(time.RFC3339)),
	})
	ptrPK, ptrSK := domainDeletion.TokenPointerKeys(userID, hash)
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityTokenPointer),
		"family":     strAttr(family),
		"userId":     strAttr(userID),
		"tokenHash":  strAttr(hash),
		"skGlobal":   strAttr(pkGlobal),
		"createdAt":  strAttr(e014T0.Format(time.RFC3339)),
	})
	return pkGlobal, skGlobal, ptrSK
}

// TestE014PurgaTokensDeUnSoloUso cubre (a) token RESET y ONE_USE activos con su puntero,
// (d) el titular B intacto y (e) la ausencia de resurrección tras PURGED.
func TestE014PurgaTokensDeUnSoloUso(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	resetHash := e014Hash("reset-token-sintetico-a")
	verifyHash := e014Hash("verify-token-sintetico-a")
	resetPK, resetSK, resetPtrSK := e014SembrarToken(m, e014Usuario, domainDeletion.TokenFamilyReset, resetHash, e014T0.Add(30*time.Minute))
	verifyPK, verifySK, verifyPtrSK := e014SembrarToken(m, e014Usuario, domainDeletion.TokenFamilyOneUse, verifyHash, e014T0.Add(24*time.Hour))

	// Residuo del mismo tipo en la cuenta de otro titular: no puede tocarse.
	otroHash := e014Hash("reset-token-sintetico-b")
	otroPK, otroSK, otroPtrSK := e014SembrarToken(m, e014OtroUser, domainDeletion.TokenFamilyReset, otroHash, e014T0.Add(time.Hour))

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	for _, caso := range []struct {
		nombre string
		PK, SK string
	}{
		{"token RESET del titular", resetPK, resetSK},
		{"token ONE_USE del titular", verifyPK, verifySK},
	} {
		if m.leer(caso.PK, caso.SK) != nil {
			t.Fatalf("%s debe borrarse: el puntero lo hace alcanzable sin Scan", caso.nombre)
		}
	}
	ptrPK := domainDeletion.UserPartition(e014Usuario)
	if m.leer(ptrPK, resetPtrSK) != nil || m.leer(ptrPK, verifyPtrSK) != nil {
		t.Fatalf("los punteros TOKENPTR del titular deben desaparecer")
	}
	for _, item := range m.porParticion(ptrPK) {
		if sk := stringAttrDe(item, "SK"); sk != domainDeletion.PrefixDeletion+e014RequestID {
			t.Fatalf("residuo en la particion del titular tras PURGED: %q", sk)
		}
	}

	// El otro titular conserva su token y su puntero.
	if m.leer(otroPK, otroSK) == nil {
		t.Fatalf("el token de otro titular no puede borrarse")
	}
	if m.leer(domainDeletion.UserPartition(e014OtroUser), otroPtrSK) == nil {
		t.Fatalf("el puntero de otro titular no puede borrarse")
	}

	// (e) Sin resurrección: ciclos posteriores no recrean el token ni el puntero.
	m.carreraDelete = nil
	for ciclo := 0; ciclo < 3; ciclo++ {
		reloj.avanzar(time.Minute)
		worker.Ciclo(e014Contexto())
	}
	if m.leer(resetPK, resetSK) != nil || m.leer(verifyPK, verifySK) != nil {
		t.Fatalf("la purga no puede recrear items globales")
	}
	if m.leer(ptrPK, resetPtrSK) != nil || m.leer(ptrPK, verifyPtrSK) != nil {
		t.Fatalf("la purga no puede recrear punteros")
	}
	if estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("el tombstone debe seguir en PURGED, obtenido %q", estado)
	}
}

// TestE014PurgaDedupeDeEntrega cubre (b) el dedupe global de E062 y su DELIVERYPTR
// escritos por el adaptador real, el replay sin sobrescritura, (d) el titular B intacto y
// (e) la ausencia de resurrección.
func TestE014PurgaDedupeDeEntrega(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	outbox := dynamoAdapter.NewPushOutboxStore(m, e014Tabla)

	if err := outbox.MarcarEntregado(e014Contexto(), e014Usuario, "evt-a", "nA", e014T0); err != nil {
		t.Fatalf("no se pudo marcar la entrega: %v", err)
	}
	globales := m.porParticion(domainDeletion.PartitionOutboxGlobal)
	if len(globales) != 1 {
		t.Fatalf("debe existir exactamente un dedupe global, hay %d", len(globales))
	}
	idemSK := stringAttrDe(globales[0], "SK")
	if !strings.HasPrefix(idemSK, "IDEM#") {
		t.Fatalf("el dedupe debe vivir en IDEM#, obtenido %q", idemSK)
	}
	expiraEsperado := e014T0.Add(90 * 24 * time.Hour).Unix()
	if expira, ok := attributeIntDe(globales[0], "expiresAt"); !ok || expira != expiraEsperado {
		t.Fatalf("expiresAt del dedupe=%d (%v), esperado %d", expira, ok, expiraEsperado)
	}

	// El puntero debe permitir a la purga alcanzar el dedupe sin Scan.
	ptrPK, ptrSK := domainDeletion.DeliveryPointerKeys(e014Usuario, "evt-a")
	puntero := m.leer(ptrPK, ptrSK)
	if puntero == nil {
		t.Fatalf("la entrega debe registrar su DELIVERYPTR en la particion del titular")
	}
	for campo, esperado := range map[string]string{
		"entityType": domainDeletion.EntityDeliveryPointer,
		"userID":     e014Usuario,
		"eventId":    "evt-a",
		"skGlobal":   idemSK,
	} {
		if got := stringAttrDe(puntero, campo); got != esperado {
			t.Fatalf("puntero.%s=%q, esperado %q", campo, got, esperado)
		}
	}
	if expira, ok := attributeIntDe(puntero, "expiresAt"); !ok || expira != expiraEsperado {
		t.Fatalf("expiresAt del puntero=%d (%v), esperado %d", expira, ok, expiraEsperado)
	}

	// Replay: no es error ni sobrescribe el registro existente.
	creadoOriginal := stringAttrDe(m.leer(domainDeletion.PartitionOutboxGlobal, idemSK), "createdAt")
	if err := outbox.MarcarEntregado(e014Contexto(), e014Usuario, "evt-a", "nA", e014T0.Add(time.Hour)); err != nil {
		t.Fatalf("el replay de entrega debe ser inocuo: %v", err)
	}
	if got := stringAttrDe(m.leer(domainDeletion.PartitionOutboxGlobal, idemSK), "createdAt"); got != creadoOriginal {
		t.Fatalf("el replay no puede reescribir el dedupe: %q != %q", got, creadoOriginal)
	}

	// Entrega del otro titular.
	if err := outbox.MarcarEntregado(e014Contexto(), e014OtroUser, "evt-b", "nB", e014T0); err != nil {
		t.Fatalf("no se pudo marcar la entrega de B: %v", err)
	}
	idemB := ""
	for _, item := range m.porParticion(domainDeletion.PartitionOutboxGlobal) {
		if stringAttrDe(item, "userID") == e014OtroUser {
			idemB = stringAttrDe(item, "SK")
		}
	}
	if idemB == "" {
		t.Fatalf("no se encontro el dedupe de B")
	}
	ptrBPK, ptrBSK := domainDeletion.DeliveryPointerKeys(e014OtroUser, "evt-b")
	if m.leer(ptrBPK, ptrBSK) == nil {
		t.Fatalf("B debe tener su propio DELIVERYPTR")
	}

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	if m.leer(domainDeletion.PartitionOutboxGlobal, idemSK) != nil {
		t.Fatalf("el dedupe del titular debe borrarse")
	}
	if m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("el DELIVERYPTR del titular debe borrarse")
	}
	// B conserva su dedupe y su puntero.
	if m.leer(domainDeletion.PartitionOutboxGlobal, idemB) == nil {
		t.Fatalf("el dedupe de B no puede borrarse")
	}
	if m.leer(ptrBPK, ptrBSK) == nil {
		t.Fatalf("el DELIVERYPTR de B no puede borrarse")
	}

	// (e) Sin resurrección.
	for ciclo := 0; ciclo < 3; ciclo++ {
		reloj.avanzar(time.Minute)
		worker.Ciclo(e014Contexto())
	}
	if m.leer(domainDeletion.PartitionOutboxGlobal, idemSK) != nil || m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("la purga no puede recrear el dedupe ni el puntero del titular")
	}
}

// TestE014DedupeConResiduoParcialSeCompleta comprueba que una cancelación de la
// transacción de entrega no pierde la marca: si sólo existía el puntero (TTL asíncrono del
// dedupe o residuo de una escritura anterior), el dedupe se completa aparte.
func TestE014DedupeConResiduoParcialSeCompleta(t *testing.T) {
	m := newPurgaDynamo()
	outbox := dynamoAdapter.NewPushOutboxStore(m, e014Tabla)

	if err := outbox.MarcarEntregado(e014Contexto(), e014Usuario, "evt-a", "nA", e014T0); err != nil {
		t.Fatalf("alta inicial: %v", err)
	}
	globales := m.porParticion(domainDeletion.PartitionOutboxGlobal)
	if len(globales) != 1 {
		t.Fatalf("dedupe esperado, hay %d", len(globales))
	}
	idemSK := stringAttrDe(globales[0], "SK")

	// El TTL de DynamoDB es asíncrono y puede retirar el dedupe antes que el puntero.
	m.mutar(mutacionPurga{pk: domainDeletion.PartitionOutboxGlobal, sk: idemSK})
	if m.leer(domainDeletion.PartitionOutboxGlobal, idemSK) != nil {
		t.Fatalf("el dedupe debe haberse retirado para simular el TTL")
	}
	if err := outbox.MarcarEntregado(e014Contexto(), e014Usuario, "evt-a", "nA", e014T0.Add(time.Minute)); err != nil {
		t.Fatalf("el residuo parcial no puede devolver error: %v", err)
	}
	if m.leer(domainDeletion.PartitionOutboxGlobal, idemSK) == nil {
		t.Fatalf("la marca de entrega debe completarse aunque el puntero ya existiera")
	}
}

// TestE014PunteroMovidoNoSeBorraACiegas cubre (c): si el puntero cambia entre la lectura y
// el borrado (otra transacción lo movió), el borrado condicional falla, la página se
// reintenta y el puntero se resuelve con la referencia NUEVA. Nunca se borra a ciegas.
func TestE014PunteroMovidoNoSeBorraACiegas(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	viejaSK := "DUE#2026-11-20T10:00:00Z#nA"
	nuevaSK := "DUE#2026-11-21T10:00:00Z#nA"
	m.sembrar(domainDeletion.PartitionOutboxGlobal, viejaSK, map[string]types.AttributeValue{
		"entityType":     strAttr("PUSH_OUTBOX"),
		"userID":         strAttr(e014Usuario),
		"notificationID": strAttr("nA"),
		"status":         strAttr("pending"),
	})
	ptrPK, ptrSK := domainDeletion.OutboxPointerKeys(e014Usuario, "nA")
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType":     strAttr(domainDeletion.EntityOutboxPointer),
		"userID":         strAttr(e014Usuario),
		"notificationID": strAttr("nA"),
		"skGlobal":       strAttr(viejaSK),
	})

	movido := false
	m.carreraDelete = func(pk, sk string) []mutacionPurga {
		if movido || pk != domainDeletion.UserPartition(e014Usuario) || !strings.HasPrefix(sk, domainDeletion.PrefixOutboxPointer) {
			return nil
		}
		movido = true
		// La transacción concurrente mueve el trabajo y actualiza `skGlobal` de forma
		// atómica, exactamente como hace el outbox al reprogramar.
		return []mutacionPurga{
			{pk: domainDeletion.PartitionOutboxGlobal, sk: nuevaSK, item: map[string]types.AttributeValue{
				"entityType":     strAttr("PUSH_OUTBOX"),
				"userID":         strAttr(e014Usuario),
				"notificationID": strAttr("nA"),
				"status":         strAttr("pending"),
			}},
			{pk: ptrPK, sk: ptrSK, item: map[string]types.AttributeValue{
				"entityType":     strAttr(domainDeletion.EntityOutboxPointer),
				"userID":         strAttr(e014Usuario),
				"notificationID": strAttr("nA"),
				"skGlobal":       strAttr(nuevaSK),
			}},
		}
	}

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)

	reloj.avanzar(time.Minute)
	worker.Ciclo(e014Contexto())

	if !movido {
		t.Fatalf("la carrera debe haberse simulado")
	}
	if estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"); estado == appAccount.PurgeStatePurged {
		t.Fatalf("no se puede declarar PURGED con un puntero movido sin resolver")
	}
	puntero := m.leer(ptrPK, ptrSK)
	if puntero == nil {
		t.Fatalf("el puntero no puede borrarse con la referencia obsoleta")
	}
	if got := stringAttrDe(puntero, "skGlobal"); got != nuevaSK {
		t.Fatalf("skGlobal del puntero=%q, esperado %q", got, nuevaSK)
	}
	if m.leer(domainDeletion.PartitionOutboxGlobal, nuevaSK) == nil {
		t.Fatalf("la referencia nueva debe seguir intacta para el reintento")
	}

	// Sin carrera, el reintento resuelve la referencia nueva y cierra la purga.
	m.carreraDelete = nil
	e014CorrerHastaPurga(t, worker, reloj, m, 20)
	if m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("el puntero debe retirarse tras resolver la referencia nueva")
	}
	if m.leer(domainDeletion.PartitionOutboxGlobal, nuevaSK) != nil {
		t.Fatalf("el item global referenciado por el puntero debe borrarse")
	}
	for _, item := range m.porParticion(domainDeletion.UserPartition(e014Usuario)) {
		if sk := stringAttrDe(item, "SK"); sk != domainDeletion.PrefixDeletion+e014RequestID {
			t.Fatalf("residuo en la particion del titular tras PURGED: %q", sk)
		}
	}
}

// TestE014PunteroDeTokenAjenoNoBorraNada cubre (d) para tokens: un puntero manipulado en la
// partición del titular que apunte al token de otra cuenta se retira, pero el token ajeno
// NO se borra. La comprobación de titular debe cubrir `userId` (tokens) además de `userID`
// (outbox/READALL); sin ella, el token de terceros era borrable.
func TestE014PunteroDeTokenAjenoNoBorraNada(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	hashB := e014Hash("reset-token-sintetico-b")
	pkGlobalB, skGlobalB, ptrBSK := e014SembrarToken(m, e014OtroUser, domainDeletion.TokenFamilyReset, hashB, e014T0.Add(time.Hour))

	// Puntero manipulado del titular apuntando al token de B. La forma del hash coincide
	// (misma SK), así que ResolveGlobalTarget lo resuelve: la única defensa posible es la
	// comprobación de titular del ítem global.
	ptrPK, ptrSK := domainDeletion.TokenPointerKeys(e014Usuario, hashB)
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityTokenPointer),
		"family":     strAttr(domainDeletion.TokenFamilyReset),
		"userId":     strAttr(e014Usuario),
		"tokenHash":  strAttr(hashB),
		"skGlobal":   strAttr(pkGlobalB),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	if m.leer(pkGlobalB, skGlobalB) == nil {
		t.Fatalf("un puntero manipulado no puede borrar el token de otra cuenta")
	}
	if m.leer(domainDeletion.UserPartition(e014OtroUser), ptrBSK) == nil {
		t.Fatalf("el puntero legitimo de B no puede borrarse")
	}
	if m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("el puntero manipulado del titular debe retirarse")
	}
	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrDe(control, "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("estado esperado PURGED, obtenido %q", estado)
	}
	// `unknownKeys` se elimina del tombstone final (minimización): se acepta su ausencia.
	if desconocidos, ok := attributeIntDe(control, "unknownKeys"); ok && desconocidos < 1 {
		t.Fatalf("el puntero ajeno debe contarse como clave desconocida, obtenido %d", desconocidos)
	}
}

// TestE014PunteroDeTokenIncoherenteNoBloquea comprueba que un TOKENPTR con `skGlobal` de
// otra familia o con un hash distinto no resuelve ninguna partición: se retira sin tocar
// nada y la purga termina.
func TestE014PunteroDeTokenIncoherenteNoBloquea(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	hash := e014Hash("reset-token-sintetico-a")
	otroHash := e014Hash("reset-token-sintetico-otro")
	pkGlobal, skGlobal, _ := e014SembrarToken(m, e014Usuario, domainDeletion.TokenFamilyReset, hash, e014T0.Add(time.Hour))

	// Puntero cuyo nombre declara `hash` pero cuyo skGlobal apunta a OTRO token del mismo
	// titular: incoherente, no debe resolver el objetivo.
	ptrPK, ptrSK := domainDeletion.TokenPointerKeys(e014Usuario, hash)
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityTokenPointer),
		"family":     strAttr(domainDeletion.TokenFamilyReset),
		"userId":     strAttr(e014Usuario),
		"tokenHash":  strAttr(hash),
		"skGlobal":   strAttr(domainDeletion.TokenFamilyReset + "#" + otroHash),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	if m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("el puntero incoherente debe retirarse")
	}
	// El contrato es "no resolver": un `skGlobal` cuya partición no coincide con el hash
	// del nombre del puntero no puede borrar ningún token.
	if m.leer(pkGlobal, skGlobal) == nil {
		t.Fatalf("un puntero incoherente no puede resolver ni borrar el token declarado")
	}
}
