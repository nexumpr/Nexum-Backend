package dynamo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// ResetAPI es el subconjunto del cliente DynamoDB que usa el flujo de recuperacion.
type ResetAPI interface {
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

// PasswordResetStore implementa ports.ResetStore y ports.ResetTokenStore sobre la
// tabla unica: RESET#<sha256> como token y QUEUE#PWRESET como cola durable.
type PasswordResetStore struct {
	client ResetAPI
	tabla  string
}

// NewPasswordResetStore construye el adaptador.
func NewPasswordResetStore(client ResetAPI, tabla string) *PasswordResetStore {
	return &PasswordResetStore{client: client, tabla: tabla}
}

const particionCola = "QUEUE#PWRESET"

func claveToken(hash string) (map[string]types.AttributeValue, string) {
	pk := "RESET#" + hash
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: pk},
		"SK": &types.AttributeValueMemberS{Value: "RESET"},
	}, pk
}

func (s *PasswordResetStore) claveTrabajo(t ports.ResetTrabajo) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: particionCola},
		"SK": &types.AttributeValueMemberS{Value: t.ExpiraEn.UTC().Format(time.RFC3339) + "#" + t.OutboxID},
	}
}

// Admitir escribe token y trabajo en una transaccion de 2 acciones sobre 2 items.
func (s *PasswordResetStore) Admitir(ctx context.Context, adm ports.ResetAdmission) error {
	claveTok, _ := claveToken(adm.TokenHash)
	tokenItem := map[string]types.AttributeValue{
		"PK":        claveTok["PK"],
		"SK":        claveTok["SK"],
		"userId":    &types.AttributeValueMemberS{Value: adm.UserID},
		"expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(adm.ExpiraEn.Unix(), 10)},
		"used":      &types.AttributeValueMemberBOOL{Value: false},
		"createdAt": &types.AttributeValueMemberS{Value: adm.Ahora.UTC().Format(time.RFC3339)},
	}
	trabajoItem := map[string]types.AttributeValue{
		"PK":             &types.AttributeValueMemberS{Value: particionCola},
		"SK":             &types.AttributeValueMemberS{Value: adm.ExpiraEn.UTC().Format(time.RFC3339) + "#" + adm.OutboxID},
		"userId":         &types.AttributeValueMemberS{Value: adm.UserID},
		"destino":        &types.AttributeValueMemberS{Value: adm.Destino},
		"estado":         &types.AttributeValueMemberS{Value: "PENDIENTE"},
		"intentos":       &types.AttributeValueMemberN{Value: "0"},
		"fence":          &types.AttributeValueMemberN{Value: "0"},
		"proximoIntento": &types.AttributeValueMemberN{Value: strconv.FormatInt(adm.Ahora.Unix(), 10)},
		"expiresAt":      &types.AttributeValueMemberN{Value: strconv.FormatInt(adm.ExpiraEn.Unix(), 10)},
	}
	if len(adm.TokenSello) > 0 {
		trabajoItem["tokenSello"] = &types.AttributeValueMemberB{Value: adm.TokenSello}
	}

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{TableName: aws.String(s.tabla), Item: tokenItem,
				ConditionExpression: aws.String("attribute_not_exists(PK)")}},
			{Put: &types.Put{TableName: aws.String(s.tabla), Item: trabajoItem}},
		},
	})
	if err != nil {
		return fmt.Errorf("admision de recuperacion: %w", err)
	}
	return nil
}

// Pendientes consulta la particion de cola y filtra por vencimiento.
func (s *PasswordResetStore) Pendientes(ctx context.Context, ahora time.Time, limite int) ([]ports.ResetTrabajo, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                aws.String(s.tabla),
		KeyConditionExpression:   aws.String("PK = :q"),
		FilterExpression:         aws.String("(#e = :pendiente AND proximoIntento <= :ahora) OR (#e = :enviando AND leaseUntil < :ahora)"),
		ExpressionAttributeNames: map[string]string{"#e": "estado"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":q":         &types.AttributeValueMemberS{Value: particionCola},
			":pendiente": &types.AttributeValueMemberS{Value: "PENDIENTE"},
			":enviando":  &types.AttributeValueMemberS{Value: "ENVIANDO"},
			":ahora":     &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Unix(), 10)},
		},
		Limit: aws.Int32(25), // acotado: evita G115
	})
	if err != nil {
		return nil, fmt.Errorf("consulta de cola: %w", err)
	}
	trabajos := make([]ports.ResetTrabajo, 0, len(out.Items))
	for _, it := range out.Items {
		t := ports.ResetTrabajo{}
		if v, ok := it["userId"].(*types.AttributeValueMemberS); ok {
			t.UserID = v.Value
		}
		if v, ok := it["destino"].(*types.AttributeValueMemberS); ok {
			t.Destino = v.Value
		}
		if v, ok := it["tokenSello"].(*types.AttributeValueMemberB); ok {
			t.TokenSello = v.Value
		}
		if v, ok := it["intentos"].(*types.AttributeValueMemberN); ok {
			if n, err := strconv.Atoi(v.Value); err == nil {
				t.Intentos = n
			}
		}
		if v, ok := it["expiresAt"].(*types.AttributeValueMemberN); ok {
			if n, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
				t.ExpiraEn = time.Unix(n, 0).UTC()
			}
		}
		if v, ok := it["SK"].(*types.AttributeValueMemberS); ok {
			if i := len(v.Value) - 1; i > 0 {
				if j := lastDash(v.Value); j > 0 {
					t.OutboxID = v.Value[j+1:]
				}
			}
		}
		trabajos = append(trabajos, t)
	}
	return trabajos, nil
}

func lastDash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '#' {
			return i
		}
	}
	return -1
}

// buscarTrabajo localiza un item de la cola por su outboxID (sin Scan).
func (s *PasswordResetStore) buscarTrabajo(ctx context.Context, outboxID string) (ports.ResetTrabajo, error) {
	trabajos, err := s.todosLosTrabajos(ctx)
	if err != nil {
		return ports.ResetTrabajo{}, err
	}
	for _, t := range trabajos {
		if t.OutboxID == outboxID {
			return t, nil
		}
	}
	return ports.ResetTrabajo{}, fmt.Errorf("trabajo %s no encontrado", outboxID)
}

// Reclamar toma el trabajo de forma condicional y devuelve el fence de ESTA
// reclamacion. El worker lo conserva: sin el no puede cerrar el trabajo.
func (s *PasswordResetStore) Reclamar(ctx context.Context, outboxID, owner string, ahora time.Time, lease time.Duration) (int, bool, error) {
	trabajo, err := s.buscarTrabajo(ctx, outboxID)
	if err != nil {
		return 0, false, nil
	}
	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tabla), Key: s.claveTrabajo(trabajo),
		UpdateExpression: aws.String("SET #e = :enviando, #owner = :owner, leaseUntil = :lease, fence = if_not_exists(fence, :zero) + :one"),
		ConditionExpression: aws.String(
			"(#e = :pendiente) OR (#e = :enviando AND leaseUntil < :ahora)"),
		ExpressionAttributeNames: map[string]string{"#e": "estado", "#owner": "owner"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":enviando":  &types.AttributeValueMemberS{Value: "ENVIANDO"},
			":pendiente": &types.AttributeValueMemberS{Value: "PENDIENTE"},
			":owner":     &types.AttributeValueMemberS{Value: owner},
			":lease":     &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Add(lease).Unix(), 10)},
			":ahora":     &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Unix(), 10)},
			":zero":      &types.AttributeValueMemberN{Value: "0"},
			":one":       &types.AttributeValueMemberN{Value: "1"},
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		return 0, false, nil // condicion no cumplida: otro nodo lo tomo
	}
	fence := 0
	if v, ok := out.Attributes["fence"].(*types.AttributeValueMemberN); ok {
		if n, errN := strconv.Atoi(v.Value); errN == nil {
			fence = n
		}
	}
	return fence, true, nil
}

// MarcarEnviado cierra el trabajo exigiendo estado, owner y fence vigentes.
func (s *PasswordResetStore) MarcarEnviado(ctx context.Context, trabajo ports.ResetTrabajo, _ string, ahora time.Time) error {
	return s.actualizarTrabajo(ctx, trabajo,
		"SET #e = :enviado, enviadoEn = :s REMOVE tokenSello",
		map[string]types.AttributeValue{":s": &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Unix(), 10)}},
		map[string]types.AttributeValue{":enviado": &types.AttributeValueMemberS{Value: "ENVIADO"}},
	)
}

// Reintentar reprograma (conservando el sobre) o cierra el trabajo como fallido.
func (s *PasswordResetStore) Reintentar(ctx context.Context, trabajo ports.ResetTrabajo, intentos int, proximo time.Time, _ time.Time, fallido bool) error {
	vals := map[string]types.AttributeValue{
		":i": &types.AttributeValueMemberN{Value: strconv.Itoa(intentos)},
		":p": &types.AttributeValueMemberN{Value: strconv.FormatInt(proximo.Unix(), 10)},
	}
	if fallido {
		vals[":fallido"] = &types.AttributeValueMemberS{Value: "FALLIDO"}
		return s.actualizarTrabajo(ctx, trabajo,
			"SET #e = :fallido, intentos = :i, proximoIntento = :p REMOVE tokenSello", vals, nil)
	}
	vals[":pendiente"] = &types.AttributeValueMemberS{Value: "PENDIENTE"}
	return s.actualizarTrabajo(ctx, trabajo,
		"SET #e = :pendiente, intentos = :i, proximoIntento = :p", vals, nil)
}

func (s *PasswordResetStore) actualizarTrabajo(ctx context.Context, trabajo ports.ResetTrabajo, expr string, extra map[string]types.AttributeValue, principal map[string]types.AttributeValue) error {
	vals := map[string]types.AttributeValue{
		":enviando": &types.AttributeValueMemberS{Value: "ENVIANDO"},
		":owner":    &types.AttributeValueMemberS{Value: trabajo.Owner},
		":fence":    &types.AttributeValueMemberN{Value: strconv.Itoa(trabajo.Fence)},
	}
	for k, v := range extra {
		vals[k] = v
	}
	for k, v := range principal {
		vals[k] = v
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tabla), Key: s.claveTrabajo(trabajo),
		UpdateExpression:          aws.String(expr),
		ConditionExpression:       aws.String("#e = :enviando AND #owner = :owner AND fence = :fence"),
		ExpressionAttributeNames:  map[string]string{"#e": "estado", "#owner": "owner"},
		ExpressionAttributeValues: vals,
	})
	if err != nil {
		return fmt.Errorf("actualizacion condicionada del trabajo: %w", err)
	}
	return nil
}

// Buscar resuelve el token de E006 (publico). No distingue el motivo del fallo.
func (s *PasswordResetStore) Buscar(ctx context.Context, tokenHash string, ahora time.Time) (string, time.Time, error) {
	clave, _ := claveToken(tokenHash)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tabla), Key: clave, ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lectura de token: %w", err)
	}
	if len(out.Item) == 0 {
		return "", time.Time{}, ports.ErrResetTokenInvalido
	}
	usado := false
	if v, ok := out.Item["used"].(*types.AttributeValueMemberBOOL); ok {
		usado = v.Value
	}
	expira := time.Time{}
	if v, ok := out.Item["expiresAt"].(*types.AttributeValueMemberN); ok {
		if n, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
			expira = time.Unix(n, 0).UTC()
		}
	}
	if usado || !expira.After(ahora) {
		return "", time.Time{}, ports.ErrResetTokenInvalido
	}
	userID := ""
	if v, ok := out.Item["userId"].(*types.AttributeValueMemberS); ok {
		userID = v.Value
	}
	if userID == "" {
		return "", time.Time{}, ports.ErrResetTokenInvalido
	}
	return userID, expira, nil
}

// Consumir marca el token como usado de forma condicional.
func (s *PasswordResetStore) Consumir(ctx context.Context, tokenHash string, ahora time.Time) error {
	clave, _ := claveToken(tokenHash)
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tabla), Key: clave,
		UpdateExpression:    aws.String("SET used = :true, usedAt = :t"),
		ConditionExpression: aws.String("(attribute_not_exists(used) OR used = :false) AND expiresAt > :ahora"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":true":  &types.AttributeValueMemberBOOL{Value: true},
			":false": &types.AttributeValueMemberBOOL{Value: false},
			":t":     &types.AttributeValueMemberS{Value: ahora.UTC().Format(time.RFC3339)},
			":ahora": &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Unix(), 10)},
		},
	})
	if err != nil {
		return ports.ErrResetTokenInvalido
	}
	return nil
}

// todosLosTrabajos devuelve la cola completa (paginada) sin filtrar por estado, para
// poder cerrar o reprogramar un trabajo que ya fue reclamado (estado ENVIANDO).
func (s *PasswordResetStore) todosLosTrabajos(ctx context.Context) ([]ports.ResetTrabajo, error) {
	var todos []ports.ResetTrabajo
	var inicio map[string]types.AttributeValue
	for {
		out, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.tabla),
			KeyConditionExpression: aws.String("PK = :q"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":q": &types.AttributeValueMemberS{Value: particionCola},
			},
			ExclusiveStartKey: inicio,
			Limit:             aws.Int32(50),
		})
		if err != nil {
			return nil, fmt.Errorf("consulta de cola: %w", err)
		}
		for _, it := range out.Items {
			todos = append(todos, trabajoDesdeItem(it))
		}
		if len(out.LastEvaluatedKey) == 0 {
			return todos, nil
		}
		inicio = out.LastEvaluatedKey
	}
}

// trabajoDesdeItem convierte un item de la cola en ResetTrabajo.
func trabajoDesdeItem(it map[string]types.AttributeValue) ports.ResetTrabajo {
	t := ports.ResetTrabajo{}
	if v, ok := it["userId"].(*types.AttributeValueMemberS); ok {
		t.UserID = v.Value
	}
	if v, ok := it["destino"].(*types.AttributeValueMemberS); ok {
		t.Destino = v.Value
	}
	if v, ok := it["tokenSello"].(*types.AttributeValueMemberB); ok {
		t.TokenSello = v.Value
	}
	if v, ok := it["intentos"].(*types.AttributeValueMemberN); ok {
		if n, err := strconv.Atoi(v.Value); err == nil {
			t.Intentos = n
		}
	}
	if v, ok := it["expiresAt"].(*types.AttributeValueMemberN); ok {
		if n, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
			t.ExpiraEn = time.Unix(n, 0).UTC()
		}
	}
	if v, ok := it["owner"].(*types.AttributeValueMemberS); ok {
		t.Owner = v.Value
	}
	if v, ok := it["fence"].(*types.AttributeValueMemberN); ok {
		if n, err := strconv.Atoi(v.Value); err == nil {
			t.Fence = n
		}
	}
	if v, ok := it["SK"].(*types.AttributeValueMemberS); ok {
		if j := lastDash(v.Value); j > 0 {
			t.OutboxID = v.Value[j+1:]
		}
	}
	return t
}
