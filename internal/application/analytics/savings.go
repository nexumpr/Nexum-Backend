package analytics

import (
	"context"
	"errors"
	"strings"
	"time"
	_ "time/tzdata"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
)

const (
	// SavingsWindowDays acota la lectura a un año civil móvil. La racha se cuenta
	// hacia atrás desde el día local actual; los días anteriores a la ventana no
	// pueden alterar el resultado.
	SavingsWindowDays = 365

	// savingsMaxPages acota el trabajo por petición a 1000 aportaciones. Superarlo
	// es un fallo controlado: no se devuelve una racha calculada con datos parciales.
	savingsMaxPages = 10

	// DefaultSavingsTimezone es el fallback aprobado en oleadas anteriores cuando la
	// preferencia falta o no es una zona IANA utilizable.
	DefaultSavingsTimezone = "America/Mexico_City"
)

// SavingsSource es la capacidad mínima de E056 sobre el almacén de aportaciones.
// La implementación real es *dynamo.ContributionsStore: sólo Query por usuario,
// nunca Scan.
type SavingsSource interface {
	ListContributions(ctx context.Context, query appContrib.ListQuery, startSK string, limit int) ([]appContrib.Contribution, string, error)
}

// TimezoneSource lee la zona IANA guardada en las preferencias del titular.
type TimezoneSource interface {
	LoadTimezone(ctx context.Context, userID string) (string, error)
}

// SavingsStreak mapea 1:1 con el esquema `SavingsStreak` del OpenAPI.
type SavingsStreak struct {
	Days     int
	Timezone string
	AsOfDate string
}

// WireSavingsStreak es la proyección JSON estricta del esquema `SavingsStreak`.
type WireSavingsStreak struct {
	Days     int    `json:"days"`
	Timezone string `json:"timezone"`
	AsOfDate string `json:"asOfDate"`
}

// Wire convierte la entidad al contrato de transporte.
func (s *SavingsStreak) Wire() WireSavingsStreak {
	if s == nil {
		return WireSavingsStreak{}
	}
	return WireSavingsStreak{Days: s.Days, Timezone: s.Timezone, AsOfDate: s.AsOfDate}
}

// WithSavings inyecta las dependencias de E056 sin alterar E052–E055.
func (s *Service) WithSavings(savings SavingsSource, zonas TimezoneSource) *Service {
	if s == nil {
		return nil
	}
	copia := *s
	copia.savings = savings
	copia.zonas = zonas
	return &copia
}

// SavingsStreak ejecuta `GET /api/v1/analytics/savings-streak`.
//
// Decisión de implementación local, pendiente de confirmación de Gate: un día
// civil local sin aportación activa ROMPE la racha. Se cuentan días consecutivos
// hacia atrás desde `asOfDate`; si el día actual no tiene aportación, `days=0`.
// Sólo se contabilizan aportaciones `active`, con importe positivo y no
// revertidas, deduplicadas por id: el movimiento vinculado no se suma dos veces.
func (s *Service) SavingsStreak(ctx context.Context, userID string) (*SavingsStreak, error) {
	if s == nil || s.savings == nil || s.zonas == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}

	nombreZona, err := s.zonas.LoadTimezone(ctx, titular)
	if err != nil {
		return nil, traducirContribError(err)
	}
	zona, err := loadSavingsLocation(nombreZona)
	if err != nil {
		return nil, err
	}

	ahora := time.Now().UTC()
	if s.now != nil {
		ahora = s.now().UTC()
	}
	hoyLocal := ahora.In(zona)
	hoy := time.Date(hoyLocal.Year(), hoyLocal.Month(), hoyLocal.Day(), 0, 0, 0, 0, zona)
	desde := hoy.AddDate(0, 0, -(SavingsWindowDays - 1))
	hasta := hoy.AddDate(0, 0, 1)

	consulta := appContrib.ListQuery{
		UserID:  titular,
		Limit:   appContrib.MaxLimit,
		From:    desde.UTC(),
		HasFrom: true,
		To:      hasta.UTC(),
		HasTo:   true,
		Status:  appContrib.StatusActive,
	}

	porDia := make(map[string]struct{})
	vistos := make(map[string]struct{})
	cursor := ""
	for pagina := 0; pagina < savingsMaxPages; pagina++ {
		items, siguiente, errLista := s.savings.ListContributions(ctx, consulta, cursor, consulta.Limit)
		if errLista != nil {
			return nil, traducirContribError(errLista)
		}
		for _, aportacion := range items {
			id := strings.TrimSpace(aportacion.ID)
			if id == "" || aportacion.Status != appContrib.StatusActive ||
				aportacion.AmountCents <= 0 || !aportacion.ReversedAt.IsZero() ||
				aportacion.OccurredAt.After(ahora) {
				continue
			}
			if _, duplicado := vistos[id]; duplicado {
				continue
			}
			vistos[id] = struct{}{}
			clave := aportacion.OccurredAt.In(zona).Format("2006-01-02")
			porDia[clave] = struct{}{}
		}
		if strings.TrimSpace(siguiente) == "" {
			// Se publica la zona RESUELTA, no la preferencia inválida recibida.
			return construirRacha(porDia, hoy, zona.String()), nil
		}
		if siguiente == cursor {
			return nil, ErrUnavailable
		}
		cursor = siguiente
	}
	return nil, ErrUnavailable
}

func construirRacha(porDia map[string]struct{}, hoy time.Time, nombre string) *SavingsStreak {
	dias := 0
	for {
		clave := hoy.AddDate(0, 0, -dias).Format("2006-01-02")
		if _, ok := porDia[clave]; !ok {
			break
		}
		dias++
	}
	return &SavingsStreak{Days: dias, Timezone: nombre, AsOfDate: hoy.Format("2006-01-02")}
}

func loadSavingsLocation(nombre string) (*time.Location, error) {
	limpio := strings.TrimSpace(nombre)
	if limpio != "" {
		if zona, err := time.LoadLocation(limpio); err == nil && zona != nil {
			return zona, nil
		}
	}
	zona, err := time.LoadLocation(DefaultSavingsTimezone)
	if err != nil {
		return nil, ErrUnavailable
	}
	return zona, nil
}

func traducirContribError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidRequest),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrUnavailable),
		errors.Is(err, ErrInternal):
		return err
	case errors.Is(err, appContrib.ErrInvalidRequest):
		return ErrInvalidRequest
	case errors.Is(err, appContrib.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(err, appContrib.ErrUnavailable):
		return ErrUnavailable
	default:
		return ErrInternal
	}
}
