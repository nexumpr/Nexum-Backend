package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

var (
	// ErrSessionNotFound indica que la sesión o el refresh token no existen en el sistema.
	ErrSessionNotFound = errors.New("sesion no encontrada o token expirado")
	// ErrSessionRevoked indica que la sesión fue explícitamente revocada.
	ErrSessionRevoked = errors.New("la sesion ha sido revocada")
	// ErrReplayDetected indica detección de reutilización fraudulenta de refresh token (AC-A-050-01).
	ErrReplayDetected = errors.New("reutilizacion de token detectada: toda la familia de sesiones ha sido revocada por seguridad")
)

// RefreshTokenEntity representa la persistencia de un refresh token con trazabilidad de familia (A-050 §2.3).
type RefreshTokenEntity struct {
	TokenHash string    `json:"tokenHash"`
	FamilyID  string    `json:"familyId"`
	SessionID string    `json:"sessionId"`
	UserID    string    `json:"userId"`
	IsUsed    bool      `json:"isUsed"`
	IsRevoked bool      `json:"isRevoked"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// SessionStore define el contrato de almacenamiento compartido para tokens y sesiones distribuidas (GOV-02, AC-A-050-02).
type SessionStore interface {
	SaveToken(ctx context.Context, token RefreshTokenEntity) error
	GetTokenByHash(ctx context.Context, tokenHash string) (*RefreshTokenEntity, error)
	MarkTokenUsed(ctx context.Context, tokenHash string) error
	RevokeFamily(ctx context.Context, familyID string) error
	RevokeSession(ctx context.Context, sessionID string) error
	IsSessionActive(ctx context.Context, sessionID string) (bool, error)
}

// MemorySessionStore adaptador concurrente en memoria para pruebas de sesiones sin nube.
type MemorySessionStore struct {
	mu           sync.RWMutex
	tokensByHash map[string]RefreshTokenEntity
	revokedFams  map[string]bool
	revokedSess  map[string]bool
}

// NewMemorySessionStore inicializa un almacenamiento seguro en memoria.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		tokensByHash: make(map[string]RefreshTokenEntity),
		revokedFams:  make(map[string]bool),
		revokedSess:  make(map[string]bool),
	}
}

func (m *MemorySessionStore) SaveToken(ctx context.Context, token RefreshTokenEntity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokensByHash[token.TokenHash] = token
	return nil
}

func (m *MemorySessionStore) GetTokenByHash(ctx context.Context, tokenHash string) (*RefreshTokenEntity, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tok, exists := m.tokensByHash[tokenHash]
	if !exists {
		return nil, ErrSessionNotFound
	}
	// Verificar si la familia o sesión están revocadas globalmente
	if m.revokedFams[tok.FamilyID] || m.revokedSess[tok.SessionID] {
		tok.IsRevoked = true
	}
	return &tok, nil
}

func (m *MemorySessionStore) MarkTokenUsed(ctx context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, exists := m.tokensByHash[tokenHash]
	if !exists {
		return ErrSessionNotFound
	}
	tok.IsUsed = true
	m.tokensByHash[tokenHash] = tok
	return nil
}

func (m *MemorySessionStore) RevokeFamily(ctx context.Context, familyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokedFams[familyID] = true
	for hash, tok := range m.tokensByHash {
		if tok.FamilyID == familyID {
			tok.IsRevoked = true
			m.tokensByHash[hash] = tok
		}
	}
	return nil
}

func (m *MemorySessionStore) RevokeSession(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokedSess[sessionID] = true
	for hash, tok := range m.tokensByHash {
		if tok.SessionID == sessionID {
			tok.IsRevoked = true
			m.tokensByHash[hash] = tok
		}
	}
	return nil
}

func (m *MemorySessionStore) IsSessionActive(ctx context.Context, sessionID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.revokedSess[sessionID] {
		return false, nil
	}
	return true, nil
}

// SessionTokens conjunto de tokens retornados al usuario tras login o rotación.
type SessionTokens struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	SessionID    string `json:"sessionId"`
	ExpiresIn    int64  `json:"expiresIn"`
}

// SessionService orquesta la autenticación, rotación segura y revocación de sesiones (A-050 §2.3, §2.4).
type SessionService struct {
	tokens ports.TokenProvider
	store  SessionStore
}

// NewSessionService crea un nuevo servicio de gestión de sesiones.
func NewSessionService(tokens ports.TokenProvider, store SessionStore) *SessionService {
	return &SessionService{
		tokens: tokens,
		store:  store,
	}
}

// HashToken calcula el digest SHA-256 en formato hex de un token opaco (solo hash persistido, A-050 §2.3).
func HashToken(plainToken string) string {
	h := sha256.Sum256([]byte(plainToken))
	return hex.EncodeToString(h[:])
}

// CreateSession crea una sesión inicial, emite par de tokens y registra la familia.
func (s *SessionService) CreateSession(ctx context.Context, userID, sessionID, familyID string) (*SessionTokens, error) {
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	if familyID == "" {
		familyID = fmt.Sprintf("fam-%d", time.Now().UnixNano())
	}

	accessToken, err := s.tokens.GenerateAccessToken(ctx, ports.TokenClaims{
		Subject:   userID,
		SessionID: sessionID,
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute), // 10 min duración (A-050 §2.2)
	})
	if err != nil {
		return nil, fmt.Errorf("error al emitir access token: %w", err)
	}

	rawRefreshToken, err := s.tokens.GenerateRefreshToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("error al emitir refresh token: %w", err)
	}

	tokenHash := HashToken(rawRefreshToken)
	entity := RefreshTokenEntity{
		TokenHash: tokenHash,
		FamilyID:  familyID,
		SessionID: sessionID,
		UserID:    userID,
		IsUsed:    false,
		IsRevoked: false,
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour), // 30 días TTL (A-050 §2.3)
	}

	if err := s.store.SaveToken(ctx, entity); err != nil {
		return nil, fmt.Errorf("error al persistir refresh token: %w", err)
	}

	return &SessionTokens{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		SessionID:    sessionID,
		ExpiresIn:    600, // 10 minutos en segundos
	}, nil
}

// RotateRefreshToken rota el refresh token con detección estricta de replay (AC-A-050-01).
// Si un token ya usado es presentado de nuevo, revoca inmediatamente toda la familia de sesiones.
func (s *SessionService) RotateRefreshToken(ctx context.Context, oldRefreshToken string) (*SessionTokens, error) {
	if oldRefreshToken == "" {
		return nil, ErrSessionNotFound
	}

	oldHash := HashToken(oldRefreshToken)
	record, err := s.store.GetTokenByHash(ctx, oldHash)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	// 1. Detección de replay: el token ya fue utilizado previamente
	if record.IsUsed {
		_ = s.store.RevokeFamily(ctx, record.FamilyID)
		return nil, ErrReplayDetected
	}

	// 2. Verificar si está revocado o expirado
	if record.IsRevoked || time.Now().UTC().After(record.ExpiresAt) {
		return nil, ErrSessionRevoked
	}

	// 3. Marcar el token anterior como consumido (rotación de un solo uso)
	if err := s.store.MarkTokenUsed(ctx, oldHash); err != nil {
		return nil, fmt.Errorf("error al marcar token como usado: %w", err)
	}

	// 4. Emitir un nuevo par dentro de la misma familia y sesión
	return s.CreateSession(ctx, record.UserID, record.SessionID, record.FamilyID)
}

// Logout revoca la sesión de forma durable en el almacenamiento compartido (AC-A-050-02).
func (s *SessionService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("identificador de sesion vacio")
	}
	return s.store.RevokeSession(ctx, sessionID)
}
