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

// Reclamar toma el trabajo de forma condicional (PENDIENTE -> ENVIANDO).
func (s *PasswordResetStore) Reclamar(ctx context.Context, outboxID string, ahora time.Time, lease time.Duration) (bool, error) {
	// El SK se reconstruye desde la propia cola para no duplicar estado en memoria.
	trabajos, err := s.Pendientes(ctx, ahora, 50)
	if err != nil {
		return false, err
	}
	for _, t := range trabajos {
		if t.OutboxID != outboxID {
			continue
		}
		clave := s.claveTrabajo(t)
		_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(s.tabla), Key: clave,
			UpdateExpression:         aws.String("SET #e = :enviando, owner = :owner, leaseUntil = :lease, fence = if_not_exists(fence, :zero) + :one"),
			ConditionExpression:      aws.String("(#e = :pendiente) OR (#e = :enviando AND leaseUntil < :ahora)"),
			ExpressionAttributeNames: map[string]string{"#e": "estado"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":enviando":  &types.AttributeValueMemberS{Value: "ENVIANDO"},
				":pendiente": &types.AttributeValueMemberS{Value: "PENDIENTE"},
				":lease":     &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Add(lease).Unix(), 10)},
				":zero":      &types.AttributeValueMemberN{Value: "0"},
				":one":       &types.AttributeValueMemberN{Value: "1"},
				":owner":     &types.AttributeValueMemberS{Value: "worker"},
				":ahora":     &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Unix(), 10)},
			},
		})
		if err != nil {
			return false, nil // condicion no cumplida: otro nodo lo tomo
		}
		return true, nil
	}
	return false, nil
}

// MarcarEnviado cierra el trabajo y elimina el token cifrado.
func (s *PasswordResetStore) MarcarEnviado(ctx context.Context, outboxID, _ string, ahora time.Time) error {
	return s.actualizarTrabajo(ctx, outboxID, ahora, "SET #e = :enviado, enviadoEn = :s REMOVE tokenSello")
}

// Reintentar reprograma o cierra el trabajo.
func (s *PasswordResetStore) Reintentar(ctx context.Context, outboxID string, intentos int, proximo time.Time, _ time.Time, fallido bool) error {
	vals := map[string]types.AttributeValue{
		":i": &types.AttributeValueMemberN{Value: strconv.Itoa(intentos)},
		":p": &types.AttributeValueMemberN{Value: strconv.FormatInt(proximo.Unix(), 10)},
	}
	if fallido {
		// Solo el cierre definitivo elimina el material cifrado.
		vals[":fallido"] = &types.AttributeValueMemberS{Value: "FALLIDO"}
		return s.actualizarTrabajoCon(ctx, outboxID,
			"SET #e = :fallido, intentos = :i, proximoIntento = :p REMOVE tokenSello", vals)
	}
	vals[":pendiente"] = &types.AttributeValueMemberS{Value: "PENDIENTE"}
	return s.actualizarTrabajoCon(ctx, outboxID,
		"SET #e = :pendiente, intentos = :i, proximoIntento = :p", vals)
}

func (s *PasswordResetStore) actualizarTrabajo(ctx context.Context, outboxID string, ahora time.Time, expr string) error {
	return s.actualizarTrabajoCon(ctx, outboxID, expr, map[string]types.AttributeValue{
		":s": &types.AttributeValueMemberN{Value: strconv.FormatInt(ahora.Unix(), 10)},
	})
}

func (s *PasswordResetStore) actualizarTrabajoCon(ctx context.Context, outboxID, expr string, extra map[string]types.AttributeValue) error {
	trabajos, err := s.todosLosTrabajos(ctx)
	if err != nil {
		return err
	}
	for _, t := range trabajos {
		if t.OutboxID != outboxID {
			continue
		}
		vals := map[string]types.AttributeValue{
			":enviado":   &types.AttributeValueMemberS{Value: "ENVIADO"},
			":pendiente": &types.AttributeValueMemberS{Value: "PENDIENTE"},
			":fallido":   &types.AttributeValueMemberS{Value: "FALLIDO"},
			":fence":     &types.AttributeValueMemberN{Value: strconv.Itoa(t.Fence)},
		}
		for k, v := range extra {
			vals[k] = v
		}
		_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(s.tabla), Key: s.claveTrabajo(t),
			UpdateExpression:          aws.String(expr),
			ConditionExpression:       aws.String("fence = :fence"),
			ExpressionAttributeNames:  map[string]string{"#e": "estado"},
			ExpressionAttributeValues: vals,
		})
		return err
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
