package dynamo

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
)

// punteroDeToken construye el ítem `USER#<sub>/TOKENPTR#<sha256(token)>` que permite a E014
// borrar el token global (`RESET#<sha256>` de E005/E006 o `ONE_USE#<sha256>` de E010/E011)
// sin `Scan`. El token global guarda `userId` —y E010 también el correo— y sólo caducaría
// por TTL; el puntero es la única referencia que la purga puede recorrer.
//
// Contenido mínimo y sin secretos: hash del token, familia, titular y clave de la partición
// de la familia. Ni el token en claro ni el correo se escriben aquí.
//
// El `expiresAt` se fija 7 días después de la caducidad del token: el TTL de DynamoDB es
// asíncrono y no garantiza orden, así que el puntero debe sobrevivir al token para que éste
// nunca quede inalcanzable; ambos son acotados y la purga los elimina antes si el titular
// se da de baja.
func punteroDeToken(userID, family, tokenHash string, expira, creado time.Time) (map[string]types.AttributeValue, bool) {
	userID = strings.TrimSpace(userID)
	hash := strings.TrimSpace(tokenHash)
	if userID == "" || hash == "" {
		return nil, false
	}
	particion, ok := domainDeletion.TokenPartition(family, hash)
	if !ok {
		return nil, false
	}
	pk, sk := domainDeletion.TokenPointerKeys(userID, hash)
	if creado.IsZero() {
		creado = time.Now().UTC()
	}
	item := map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(sk),
		"entityType": stringAttr(domainDeletion.EntityTokenPointer),
		"family":     stringAttr(strings.TrimSpace(family)),
		"userId":     stringAttr(userID),
		"tokenHash":  stringAttr(hash),
		"skGlobal":   stringAttr(particion),
		"createdAt":  stringAttr(creado.UTC().Format(time.RFC3339)),
	}
	if !expira.IsZero() {
		item["expiresAt"] = numberAttr(expira.UTC().Add(7 * 24 * time.Hour).Unix())
	}
	return item, true
}
