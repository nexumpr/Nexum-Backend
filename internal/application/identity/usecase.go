package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

var (
	// ErrUserAlreadyExists indica que el email ya está registrado en la persistencia.
	ErrUserAlreadyExists = errors.New("el correo electronico ya se encuentra registrado")
)

// RegisterInput DTO de entrada para registrar usuario.
type RegisterInput struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserOutput DTO de salida sin datos sensibles ni contraseñas.
type UserOutput struct {
	ID        string            `json:"id"`
	Email     string            `json:"email"`
	Status    domain.UserStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
}

// AuthOutput DTO con token de acceso y datos de sesión.
type AuthOutput struct {
	AccessToken string     `json:"access_token"`
	User        UserOutput `json:"user"`
}

// IdentityService orquesta los casos de uso del módulo de identidad (A-020 §2.2, §2.3).
// Depende únicamente de domain y de los puertos abstractos de infraestructura.
type IdentityService struct {
	repo          ports.NoSQLRepository
	cache         ports.DistributedCache
	hasher        ports.PasswordHasher
	tokenProvider ports.TokenProvider
	tableName     string
}

// NewIdentityService inicializa el servicio de aplicación inyectando sus dependencias por interfaz.
func NewIdentityService(
	repo ports.NoSQLRepository,
	cache ports.DistributedCache,
	hasher ports.PasswordHasher,
	tokenProvider ports.TokenProvider,
	tableName string,
) *IdentityService {
	return &IdentityService{
		repo:          repo,
		cache:         cache,
		hasher:        hasher,
		tokenProvider: tokenProvider,
		tableName:     tableName,
	}
}

// RegisterUser orquesta la creación de una nueva cuenta.
func (s *IdentityService) RegisterUser(ctx context.Context, input RegisterInput) (*UserOutput, error) {
	// 1. Verificar idempotencia / unicidad en repositorio
	pk := fmt.Sprintf("USER#%s", input.ID)
	_, err := s.repo.GetItem(ctx, s.tableName, map[string]any{"PK": pk})
	if err == nil {
		return nil, ErrUserAlreadyExists
	}

	// 2. Hashear contraseña con Argon2id
	hashedPass, err := s.hasher.HashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("fallo al procesar credenciales: %w", err)
	}

	// 3. Crear entidad pura de dominio
	user, err := domain.NewUser(input.ID, input.Email, hashedPass)
	if err != nil {
		return nil, fmt.Errorf("invariante de usuario violada: %w", err)
	}

	// 4. Persistir en repositorio NoSQL
	item := map[string]any{
		"PK":           pk,
		"ID":           user.ID(),
		"Email":        user.Email(),
		"PasswordHash": user.PasswordHash(),
		"Status":       string(user.Status()),
		"CreatedAt":    user.CreatedAt().Format(time.RFC3339),
		"UpdatedAt":    user.UpdatedAt().Format(time.RFC3339),
	}

	cond := &ports.ConditionExpression{Expression: "attribute_not_exists(PK)"}
	if err := s.repo.PutItem(ctx, s.tableName, item, cond); err != nil {
		return nil, fmt.Errorf("fallo al persistir usuario: %w", err)
	}

	return &UserOutput{
		ID:        user.ID(),
		Email:     user.Email(),
		Status:    user.Status(),
		CreatedAt: user.CreatedAt(),
	}, nil
}

// Authenticate valida credenciales y emite tokens sin almacenar estado en la memoria de una instancia.
func (s *IdentityService) Authenticate(ctx context.Context, userID, rawPassword string) (*AuthOutput, error) {
	pk := fmt.Sprintf("USER#%s", userID)
	item, err := s.repo.GetItem(ctx, s.tableName, map[string]any{"PK": pk})
	if err != nil {
		return nil, ports.ErrInvalidPassword
	}

	// 1. Validar contraseña con puerto hasher
	storedHash, _ := item["PasswordHash"].(string)
	valid, err := s.hasher.VerifyPassword(rawPassword, storedHash)
	if err != nil || !valid {
		return nil, ports.ErrInvalidPassword
	}

	// 2. Reconstituir entidad para validar reglas de negocio (usuario no eliminado, etc.)
	status := domain.UserStatus(item["Status"].(string))
	createdTime, _ := time.Parse(time.RFC3339, item["CreatedAt"].(string))
	updatedTime, _ := time.Parse(time.RFC3339, item["UpdatedAt"].(string))

	user, err := domain.ReconstituteUser(
		item["ID"].(string),
		item["Email"].(string),
		storedHash,
		status,
		createdTime,
		updatedTime,
	)
	if err != nil {
		return nil, err
	}

	if err := user.CanAuthenticate(); err != nil {
		return nil, err
	}

	// 3. Emitir token firmado RS256
	sessionID := fmt.Sprintf("sess-%s-%d", user.ID(), time.Now().UnixNano())
	claims := ports.TokenClaims{
		Subject:   user.ID(),
		SessionID: sessionID,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Roles:     []string{"USER"},
	}

	token, err := s.tokenProvider.GenerateAccessToken(ctx, claims)
	if err != nil {
		return nil, fmt.Errorf("fallo al generar token: %w", err)
	}

	// 4. Registrar sesión en la caché distribuida compartida (Redis)
	cacheKey := fmt.Sprintf("session:%s", sessionID)
	if err := s.cache.Set(ctx, cacheKey, []byte(user.ID()), 1*time.Hour); err != nil {
		return nil, fmt.Errorf("fallo al registrar sesion compartida: %w", err)
	}

	return &AuthOutput{
		AccessToken: token,
		User: UserOutput{
			ID:        user.ID(),
			Email:     user.Email(),
			Status:    user.Status(),
			CreatedAt: user.CreatedAt(),
		},
	}, nil
}

