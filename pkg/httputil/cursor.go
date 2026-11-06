package httputil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	// DefaultPageLimit es el límite por defecto para listados paginados (A-040 §2.3).
	DefaultPageLimit = 25
	// MaxPageLimit es el límite máximo permitido para prevenir sobrecarga (A-040 §2.3).
	MaxPageLimit = 100
	// DefaultCursorTTL es la vigencia estándar de un cursor opaco (ej: 2 horas).
	DefaultCursorTTL = 2 * time.Hour
)

// CursorPayload contiene los datos inmutables del cursor de paginación (A-040 §2.3).
type CursorPayload struct {
	UserID    string    `json:"uid"`
	SortBy    string    `json:"sb"`
	LastID    string    `json:"lid"`
	Timestamp time.Time `json:"ts"`
	ExpiresAt time.Time `json:"exp"`
}

// OpaqueCursor representa el token empaquetado y firmado para el cliente.
type OpaqueCursor struct {
	Payload   CursorPayload `json:"p"`
	Signature string        `json:"s"`
}

// GenerateCursor crea y firma un cursor opaco codificado en Base64 URL-safe.
func GenerateCursor(secret []byte, userID, sortBy, lastID string, ts time.Time, ttl time.Duration) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("secreto de firma de cursor no puede estar vacio")
	}
	if ttl == 0 {
		ttl = DefaultCursorTTL
	}

	payload := CursorPayload{
		UserID:    userID,
		SortBy:    sortBy,
		LastID:    lastID,
		Timestamp: ts.UTC(),
		ExpiresAt: time.Now().UTC().Add(ttl),
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error al serializar payload de cursor: %w", err)
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(rawPayload)
	sig := mac.Sum(nil)

	cursor := OpaqueCursor{
		Payload:   payload,
		Signature: base64.RawURLEncoding.EncodeToString(sig),
	}

	cursorBytes, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("error al serializar cursor firmado: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(cursorBytes), nil
}

// ValidateCursor decodifica, verifica la firma criptográfica y la expiración de un cursor opaco.
func ValidateCursor(secret []byte, rawCursor string, expectedUserID string) (*CursorPayload, error) {
	if len(secret) == 0 {
		return nil, errors.New("secreto de firma de cursor no puede estar vacio")
	}
	if rawCursor == "" {
		return nil, errors.New("cursor vacio")
	}

	cursorBytes, err := base64.RawURLEncoding.DecodeString(rawCursor)
	if err != nil {
		return nil, fmt.Errorf("cursor invalido: formato base64 incorrecto: %w", err)
	}

	var cursor OpaqueCursor
	if err := json.Unmarshal(cursorBytes, &cursor); err != nil {
		return nil, fmt.Errorf("cursor invalido: estructura no decodificable: %w", err)
	}

	rawPayload, err := json.Marshal(cursor.Payload)
	if err != nil {
		return nil, fmt.Errorf("error al reconstruir payload: %w", err)
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(rawPayload)
	expectedSig := mac.Sum(nil)

	actualSig, err := base64.RawURLEncoding.DecodeString(cursor.Signature)
	if err != nil {
		return nil, errors.New("cursor alterado: firma corrupta")
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("cursor alterado: verificacion de firma HMAC fallida")
	}

	// Validar expiración
	if time.Now().UTC().After(cursor.Payload.ExpiresAt) {
		return nil, errors.New("cursor expirado")
	}

	// Validar pertenencia al usuario (no permitir compartir cursores entre cuentas)
	if expectedUserID != "" && cursor.Payload.UserID != expectedUserID {
		return nil, errors.New("cursor no pertenece al usuario autenticado")
	}

	return &cursor.Payload, nil
}

// PaginatedMeta metadatos de paginación incluidos en respuestas de lista (A-040 §2.3).
type PaginatedMeta struct {
	NextCursor *string `json:"nextCursor,omitempty"`
	HasMore    bool    `json:"hasMore"`
	Limit      int     `json:"limit"`
}

// PaginatedResponse envuelve una lista de datos paginados.
type PaginatedResponse[T any] struct {
	Data []T           `json:"data"`
	Meta PaginatedMeta `json:"meta"`
}

// WritePaginatedResponse escribe una respuesta paginada inyectando las cabeceras estándar (AC-A-040-03).
func WritePaginatedResponse[T any](w http.ResponseWriter, status int, data []T, meta PaginatedMeta) error {
	if data == nil {
		data = make([]T, 0)
	}
	if meta.NextCursor != nil && *meta.NextCursor != "" {
		w.Header().Set("X-Cursor-Next", *meta.NextCursor)
	}
	w.Header().Set("X-Has-More", strconv.FormatBool(meta.HasMore))

	response := PaginatedResponse[T]{
		Data: data,
		Meta: meta,
	}
	return WriteJSON(w, status, response)
}

// ParsePaginationLimit extrae y normaliza el parámetro 'limit' de una consulta HTTP (default 25, max 100).
func ParsePaginationLimit(r *http.Request) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return DefaultPageLimit
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return DefaultPageLimit
	}
	if val > MaxPageLimit {
		return MaxPageLimit
	}
	return val
}
