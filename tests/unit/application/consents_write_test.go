package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
)

// Pruebas de aplicación de E017 (`POST /api/v1/users/me/consents`): validación de la decisión
// (`noticeVersion`, `purpose` y `granted` obligatorio), conservación de `granted:false`,
// identidad tomada del sujeto autenticado, valor de contrato de cinco campos y traducción de
// conflictos y fallos de persistencia. El escritor es un doble de prueba; la composición
// productiva usa DynamoDB real.

const (
	e017Usuario = "11111111-2222-4333-8444-555555555555"

	e017ID = "018f90ef-08e5-7a68-b09f-0000000000e7"
)

var e017Ahora = time.Date(2026, 11, 18, 13, 0, 0, 123456789, time.UTC)

// e017Escritor captura las decisiones que la aplicación intenta persistir.
type e017Escritor struct {
	decisiones []appPrivacy.ConsentDecision
	fallo      error
}

func (w *e017Escritor) AppendConsent(_ context.Context, decision appPrivacy.ConsentDecision) error {
	if w.fallo != nil {
		return w.fallo
	}
	w.decisiones = append(w.decisiones, decision)
	return nil
}

func e017Servicio(escritor appPrivacy.ConsentWriter) *appPrivacy.ConsentsWriteService {
	return appPrivacy.NewConsentsWriteService(escritor).
		WithClock(func() time.Time { return e017Ahora }).
		WithIDGenerator(func() (string, error) { return e017ID, nil })
}

func e017Bool(valor bool) *bool { return &valor }

func e017Comando(version, purpose string, granted *bool) appPrivacy.ConsentCommand {
	return appPrivacy.ConsentCommand{
		UserID:        e017Usuario,
		NoticeVersion: version,
		Purpose:       purpose,
		Granted:       granted,
	}
}

// TestE017RegistraConsentimientoConcedido cubre el caso feliz: valor de contrato de cinco
// campos, identidad del sujeto y decisión completa lista para persistirse.
func TestE017RegistraConsentimientoConcedido(t *testing.T) {
	escritor := &e017Escritor{}
	consent, err := e017Servicio(escritor).Record(context.Background(),
		e017Comando("2026-11", appPrivacy.ConsentPurposeNotifications, e017Bool(true)))
	if err != nil {
		t.Fatalf("registro valido devolvio error: %v", err)
	}
	if consent == nil {
		t.Fatal("se esperaba un consentimiento registrado")
	}
	if consent.ID != e017ID || consent.NoticeVersion != "2026-11" ||
		consent.Purpose != appPrivacy.ConsentPurposeNotifications || !consent.Granted {
		t.Fatalf("valor de contrato incorrecto: %+v", consent)
	}
	if !consent.RecordedAt.Equal(e017Ahora) {
		t.Fatalf("recordedAt debe ser el instante del registro: %v vs %v", consent.RecordedAt, e017Ahora)
	}
	if len(escritor.decisiones) != 1 {
		t.Fatalf("debe persistirse exactamente una decision, se intentaron %d", len(escritor.decisiones))
	}
	decision := escritor.decisiones[0]
	if decision.UserID != e017Usuario {
		t.Fatalf("la persistencia debe recibir el sub del token, recibio %q", decision.UserID)
	}
	if decision.Source != appPrivacy.ConsentSourceAPI {
		t.Fatalf("source debe ser %q, recibio %q", appPrivacy.ConsentSourceAPI, decision.Source)
	}
	if !decision.RecordedAt.Equal(e017Ahora) || !decision.CreatedAt.Equal(e017Ahora) {
		t.Fatalf("recordedAt y createdAt deben ser el mismo instante UTC: %+v", decision)
	}
	if decision.Purpose != appPrivacy.ConsentPurposeNotifications || !decision.Granted {
		t.Fatalf("la decision persistida no coincide: %+v", decision)
	}
}

// TestE017GrantedFalseEsDecisionValida demuestra que una denegación explícita se conserva: no
// se confunde con la ausencia del campo ni se invierte por un valor por defecto.
func TestE017GrantedFalseEsDecisionValida(t *testing.T) {
	escritor := &e017Escritor{}
	consent, err := e017Servicio(escritor).Record(context.Background(),
		e017Comando("2026-11", appPrivacy.ConsentPurposeAnalytics, e017Bool(false)))
	if err != nil {
		t.Fatalf("una denegacion explicita es valida: %v", err)
	}
	if consent.Granted {
		t.Fatalf("granted:false debe conservarse, se obtuvo %+v", consent)
	}
	if len(escritor.decisiones) != 1 || escritor.decisiones[0].Granted {
		t.Fatalf("la persistencia debe recibir granted=false: %+v", escritor.decisiones)
	}
}

// TestE017GrantedAusenteNoMarcaOptInPorOmision: el contrato exige `granted`. Su ausencia es
// 422 y no se escribe nada; no se interpreta el silencio como consentimiento.
func TestE017GrantedAusenteNoMarcaOptInPorOmision(t *testing.T) {
	escritor := &e017Escritor{}
	_, err := e017Servicio(escritor).Record(context.Background(),
		e017Comando("2026-11", appPrivacy.ConsentPurposeNotifications, nil))
	if !errors.Is(err, appPrivacy.ErrConsentDecisionInvalid) {
		t.Fatalf("granted ausente debe ser 422, se obtuvo %v", err)
	}
	if len(escritor.decisiones) != 0 {
		t.Fatalf("una decision invalida no debe llegar a la persistencia: %+v", escritor.decisiones)
	}
}

// TestE017PropositoFueraDelEnumRechazado: el enum del contrato es cerrado y distingue
// mayúsculas: cualquier otro valor es 422 sin persistir.
func TestE017PropositoFueraDelEnumRechazado(t *testing.T) {
	for _, purpose := range []string{"", "  ", "marketing", "PrivacyNotice", "privacy_notice"} {
		escritor := &e017Escritor{}
		_, err := e017Servicio(escritor).Record(context.Background(),
			e017Comando("2026-11", purpose, e017Bool(true)))
		if !errors.Is(err, appPrivacy.ErrConsentDecisionInvalid) {
			t.Fatalf("proposito %q debe ser 422, se obtuvo %v", purpose, err)
		}
		if len(escritor.decisiones) != 0 {
			t.Fatalf("proposito %q no debe persistirse: %+v", purpose, escritor.decisiones)
		}
	}

	// Un propósito válido con espacios se normaliza en lugar de rechazarse.
	escritor := &e017Escritor{}
	consent, err := e017Servicio(escritor).Record(context.Background(),
		e017Comando("2026-11", "  notifications  ", e017Bool(true)))
	if err != nil || consent.Purpose != appPrivacy.ConsentPurposeNotifications {
		t.Fatalf("un proposito valido con espacios debe normalizarse: %+v (%v)", consent, err)
	}
}

// TestE017VersionDeAvisoInvalidaRechazada cubre el rango del contrato: no vacía, sin sólo
// espacios, máximo 50 caracteres Unicode y sin caracteres de control.
func TestE017VersionDeAvisoInvalidaRechazada(t *testing.T) {
	invalidas := []string{
		"",
		"   ",
		"\t\n",
		strings.Repeat("v", appPrivacy.ConsentNoticeVersionMaxLength+1),
		"2026-11\x00",
	}
	for _, version := range invalidas {
		escritor := &e017Escritor{}
		_, err := e017Servicio(escritor).Record(context.Background(),
			e017Comando(version, appPrivacy.ConsentPurposePrivacyNotice, e017Bool(true)))
		if !errors.Is(err, appPrivacy.ErrConsentDecisionInvalid) {
			t.Fatalf("version %q debe ser 422, se obtuvo %v", version, err)
		}
		if len(escritor.decisiones) != 0 {
			t.Fatalf("version %q no debe persistirse: %+v", version, escritor.decisiones)
		}
	}
}

// TestE017VersionDeAvisoLimiteSeAcepta: 50 caracteres cuentan runas, no bytes; el valor se
// recorta y se persiste normalizado.
func TestE017VersionDeAvisoLimiteSeAcepta(t *testing.T) {
	limite := strings.Repeat("á", appPrivacy.ConsentNoticeVersionMaxLength)
	escritor := &e017Escritor{}
	consent, err := e017Servicio(escritor).Record(context.Background(),
		e017Comando("  "+limite+"  ", appPrivacy.ConsentPurposePrivacyNotice, e017Bool(true)))
	if err != nil {
		t.Fatalf("50 runas deben aceptarse: %v", err)
	}
	if consent.NoticeVersion != limite {
		t.Fatalf("la version debe recortarse y conservarse: %q", consent.NoticeVersion)
	}
	if len(escritor.decisiones) != 1 || escritor.decisiones[0].NoticeVersion != limite {
		t.Fatalf("la persistencia debe recibir la version normalizada: %+v", escritor.decisiones)
	}
}

// TestE017SinSujetoEsNoAutorizado: la identidad nunca se inventa; un `sub` vacío es 401.
func TestE017SinSujetoEsNoAutorizado(t *testing.T) {
	escritor := &e017Escritor{}
	comando := e017Comando("2026-11", appPrivacy.ConsentPurposeNotifications, e017Bool(true))
	comando.UserID = "   "
	_, err := e017Servicio(escritor).Record(context.Background(), comando)
	if !errors.Is(err, appPrivacy.ErrConsentUnauthorized) {
		t.Fatalf("sin sub debe ser 401, se obtuvo %v", err)
	}
	if len(escritor.decisiones) != 0 {
		t.Fatalf("sin sujeto no debe escribirse nada: %+v", escritor.decisiones)
	}
}

// TestE017FallaCerradoSinPersistencia: sin almacén, con fallo genérico o con conflicto de
// clave, la aplicación nunca devuelve un éxito ficticio.
func TestE017FallaCerradoSinPersistencia(t *testing.T) {
	ctx := context.Background()
	comando := e017Comando("2026-11", appPrivacy.ConsentPurposeNotifications, e017Bool(true))

	if _, err := appPrivacy.NewConsentsWriteService(nil).Record(ctx, comando); !errors.Is(err, appPrivacy.ErrConsentUnavailable) {
		t.Fatalf("sin almacen debe ser 503, se obtuvo %v", err)
	}

	caido := &e017Escritor{fallo: errors.New("dynamodb inaccesible")}
	if _, err := e017Servicio(caido).Record(ctx, comando); !errors.Is(err, appPrivacy.ErrConsentUnavailable) {
		t.Fatalf("un fallo de persistencia debe ser 503, se obtuvo %v", err)
	}

	conflicto := &e017Escritor{fallo: appPrivacy.ErrConsentConflict}
	if _, err := e017Servicio(conflicto).Record(ctx, comando); !errors.Is(err, appPrivacy.ErrConsentConflict) {
		t.Fatalf("una clave ocupada debe ser 409, se obtuvo %v", err)
	}

	sinCredenciales := &e017Escritor{fallo: appPrivacy.ErrConsentUnauthorized}
	if _, err := e017Servicio(sinCredenciales).Record(ctx, comando); !errors.Is(err, appPrivacy.ErrConsentUnauthorized) {
		t.Fatalf("una persistencia que rechaza el sujeto debe ser 401, se obtuvo %v", err)
	}
}

// TestE017IdentificadorNoDisponibleFallaCerrado: sin identificador válido no puede emitirse un
// `Consent` ni escribirse un ítem con clave incompleta.
func TestE017IdentificadorNoDisponibleFallaCerrado(t *testing.T) {
	ctx := context.Background()
	comando := e017Comando("2026-11", appPrivacy.ConsentPurposeNotifications, e017Bool(true))

	casos := []struct {
		nombre    string
		generador func() (string, error)
	}{
		{"error del generador", func() (string, error) { return "", errors.New("entropia no disponible") }},
		{"identificador vacio", func() (string, error) { return "   ", nil }},
		{"identificador fuera del contrato", func() (string, error) { return "no-es-un-uuid", nil }},
		{"identificador con separador de clave", func() (string, error) { return "018f90ef-08e5-7a68-b09f-0000000000#7", nil }},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			escritor := &e017Escritor{}
			servicio := appPrivacy.NewConsentsWriteService(escritor).
				WithClock(func() time.Time { return e017Ahora }).
				WithIDGenerator(caso.generador)
			if _, err := servicio.Record(ctx, comando); !errors.Is(err, appPrivacy.ErrConsentUnavailable) {
				t.Fatalf("debe fallar cerrado con 503, se obtuvo %v", err)
			}
			if len(escritor.decisiones) != 0 {
				t.Fatalf("sin identificador no debe escribirse nada: %+v", escritor.decisiones)
			}
		})
	}
}
