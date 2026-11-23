// Package bootstrap implementa E021 (`GET /api/v1/bootstrap`). Agrega en una
// sola lectura autenticada los dos datos estrictamente necesarios para arrancar
// la aplicación: el perfil propio y el resumen financiero del mes vigente.
//
// Invariantes:
//   - El usuario llega del claim `sub` verificado; nunca de query ni cuerpo.
//   - No se devuelven sesiones, tokens, credenciales ni atributos internos.
//   - Los dos agregados se leen de la persistencia real del usuario; sin
//     repositorios o sin identidad, la operación falla cerrado.
//   - No se introduce caché compartida: la respuesta lleva `Cache-Control:
//     private, no-store` para que proxies y navegadores no la reutilicen.
package bootstrap

import (
	"context"
	"errors"
	"strings"
	"time"

	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
)

var (
	ErrUnauthorized = errors.New("bootstrap no autorizado")
	ErrNotFound     = errors.New("datos de arranque no encontrados")
	ErrUnavailable  = errors.New("servicio de arranque no disponible")
	ErrInternal     = errors.New("datos de arranque incoherentes")
)

// ProfileProvider es el subconjunto de appProfile.Service que consume E021.
type ProfileProvider interface {
	Get(ctx context.Context, userID string) (*appProfile.Profile, error)
}

// SummaryProvider es el subconjunto de appSummary.Service que consume E021.
type SummaryProvider interface {
	Summary(ctx context.Context, userID, period string) (*appSummary.Summary, error)
}

// WireProfile es la proyección exacta de `UserProfile` dentro de `Bootstrap`.
type WireProfile struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	EmailVerified bool   `json:"emailVerified"`
	CreatedAt     string `json:"createdAt"`
	Version       int64  `json:"version"`
}

// Wire es el DTO estricto del esquema `Bootstrap`.
type Wire struct {
	Profile          WireProfile            `json:"profile"`
	FinancialSummary appSummary.WireSummary `json:"financialSummary"`
	CatalogVersion   string                 `json:"catalogVersion"`
}

// Service compone los dos casos de uso existentes sin reimplementarlos.
type Service struct {
	perfiles  ProfileProvider
	resumenes SummaryProvider
	catalogo  string
}

// NewService construye el caso de uso. Sin proveedores, Load falla cerrado.
func NewService(perfiles ProfileProvider, resumenes SummaryProvider, catalogVersion string) *Service {
	return &Service{
		perfiles:  perfiles,
		resumenes: resumenes,
		catalogo:  strings.TrimSpace(catalogVersion),
	}
}

// Disponible indica si hay proveedores reales configurados.
func (s *Service) Disponible() bool {
	return s != nil && s.perfiles != nil && s.resumenes != nil
}

// Load obtiene el perfil y el resumen del periodo vigente para el sujeto.
func (s *Service) Load(ctx context.Context, userID string) (*Wire, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}

	perfil, err := s.perfiles.Get(ctx, titular)
	if err != nil {
		return nil, mapProfileError(err)
	}
	if perfil == nil {
		return nil, ErrNotFound
	}
	resumen, err := s.resumenes.Summary(ctx, titular, "")
	if err != nil {
		return nil, mapSummaryError(err)
	}
	if resumen == nil {
		return nil, ErrInternal
	}
	catalogo := s.catalogo
	if catalogo == "" {
		catalogo = time.Now().UTC().Format("2006-01")
	}
	return &Wire{
		Profile: WireProfile{
			ID:            perfil.ID,
			Email:         perfil.Email,
			DisplayName:   perfil.DisplayName,
			EmailVerified: perfil.EmailVerified,
			CreatedAt:     perfil.CreatedAt.UTC().Truncate(time.Second).Format(time.RFC3339),
			Version:       perfil.Version,
		},
		FinancialSummary: resumen.Wire(),
		CatalogVersion:   catalogo,
	}, nil
}

func mapProfileError(err error) error {
	switch {
	case errors.Is(err, appProfile.ErrProfileUnauthorized):
		return ErrUnauthorized
	case errors.Is(err, appProfile.ErrProfileNotFound):
		return ErrNotFound
	case errors.Is(err, appProfile.ErrProfileUnavailable):
		return ErrUnavailable
	default:
		return ErrInternal
	}
}

func mapSummaryError(err error) error {
	switch {
	case errors.Is(err, appSummary.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(err, appSummary.ErrUnavailable):
		return ErrUnavailable
	case errors.Is(err, appSummary.ErrInvalidPeriod), errors.Is(err, appSummary.ErrInconsistent),
		errors.Is(err, appSummary.ErrOverflow), errors.Is(err, appSummary.ErrInternal):
		return ErrInternal
	default:
		return ErrUnavailable
	}
}
