// Package budgetset implementa E048 (`GET /api/v1/budgets`) sobre el ítem agregado
// `USER#<id>/BUDGETSET#<YYYY-MM>` aprobado en RFC-022 v1.2.1.
package budgetset

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	CurrencyMXN     = "MXN"
	DefaultLimit    = httputil.DefaultPageLimit
	MaxLimit        = httputil.MaxPageLimit
	CursorMinLength = 8
	CursorMaxLength = 2048
	MaxCategories   = 50
)

var (
	ErrInvalidRequest = errors.New("presupuestos: solicitud invalida")
	ErrUnauthorized   = errors.New("presupuestos: identidad no autorizada")
	ErrNotFound       = errors.New("presupuestos: recurso no encontrado")
	ErrUnavailable    = errors.New("presupuestos: dependencia no disponible")
	ErrInternal       = errors.New("presupuestos: fallo interno")
	ErrInconsistent   = errors.New("presupuestos: registro inconsistente")
)

// Budget es una categoría del conjunto.
type Budget struct {
	CategoryID       string
	LimitAmountCents int64
}

// Set es el conjunto completo de un periodo.
type Set struct {
	Period    string
	Currency  string
	Version   int64
	Items     []Budget
	UpdatedAt time.Time
}

// Item es una fila de `Budget` con el periodo y la versión del conjunto del que procede.
type Item struct {
	Period           string
	Currency         string
	Version          int64
	CategoryID       string
	LimitAmountCents int64
}

// Page es una página de `Budget`.
type Page struct {
	Items      []Item
	NextCursor string
	ETag       string
}

// WireBudget es el objeto `Budget` del contrato.
type WireBudget struct {
	CategoryID  string `json:"categoryId"`
	Period      string `json:"period"`
	LimitAmount string `json:"limitAmount"`
	Currency    string `json:"currency"`
	Version     int64  `json:"version"`
}

// Store lee conjuntos. Nunca Scan.
type Store interface {
	LoadSet(ctx context.Context, userID, period string) (*Set, error)
	ListAllSets(ctx context.Context, userID string) ([]Set, error)
}

// Service sirve E048.
type Service struct {
	store     Store
	cursorKey []byte
	now       func() time.Time
}

// NewService construye el servicio. Sin almacén falla cerrado.
func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// WithCursorKey fija la clave HMAC del cursor. Sin clave el listado falla 503.
func (s *Service) WithCursorKey(key []byte) *Service {
	if len(key) == 0 {
		return s
	}
	copia := *s
	copia.cursorKey = append([]byte(nil), key...)
	return &copia
}

// WithClock inyecta reloj determinista.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now == nil {
		return s
	}
	copia := *s
	copia.now = now
	return &copia
}

// List ejecuta E048. Con `period` devuelve el conjunto paginado y su ETag; sin `period`
// devuelve todos los periodos paginados sin ETag.
func (s *Service) List(ctx context.Context, userID, period, cursor string, limit int) (*Page, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	if len(s.cursorKey) == 0 {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	normalizado, err := normalizeQuery(period, cursor, limit)
	if err != nil {
		return nil, err
	}

	if normalizado.Period != "" {
		set, err := s.store.LoadSet(ctx, titular, normalizado.Period)
		if err != nil {
			return nil, err
		}
		if set == nil {
			set = &Set{Period: normalizado.Period, Currency: CurrencyMXN, Version: 0}
		}
		return s.paginateSet(titular, set, normalizado)
	}

	sets, err := s.store.ListAllSets(ctx, titular)
	if err != nil {
		return nil, err
	}
	return s.paginateAll(titular, sets, normalizado)
}

type query struct {
	Period string
	Cursor string
	Limit  int
}

func normalizeQuery(period, cursor string, limit int) (query, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return query{}, fmt.Errorf("%w: limit", ErrInvalidRequest)
	}
	p := strings.TrimSpace(period)
	if p != "" {
		if len(p) != 7 || p[4] != '-' {
			return query{}, fmt.Errorf("%w: period", ErrInvalidRequest)
		}
		if _, err := time.Parse("2006-01", p); err != nil {
			return query{}, fmt.Errorf("%w: period", ErrInvalidRequest)
		}
	}
	c := strings.TrimSpace(cursor)
	if c != "" && (len(c) < CursorMinLength || len(c) > CursorMaxLength) {
		return query{}, fmt.Errorf("%w: cursor", ErrInvalidRequest)
	}
	return query{Period: p, Cursor: c, Limit: limit}, nil
}

func (s *Service) paginateSet(userID string, set *Set, q query) (*Page, error) {
	items := append([]Budget(nil), set.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].CategoryID < items[j].CategoryID })
	inicio := 0
	firma := fmt.Sprintf("period=%s;limit=%d", set.Period, q.Limit)
	if q.Cursor != "" {
		payload, err := httputil.ValidateCursor(s.cursorKey, q.Cursor, userID)
		if err != nil || payload.SortBy != firma || payload.LastID == "" {
			return nil, fmt.Errorf("%w: cursor", ErrInvalidRequest)
		}
		if _, err := fmt.Sscanf(payload.LastID, "%d", &inicio); err != nil || inicio < 0 || inicio > len(items) {
			return nil, fmt.Errorf("%w: cursor", ErrInvalidRequest)
		}
	}
	fin := inicio + q.Limit
	if fin > len(items) {
		fin = len(items)
	}
	page := &Page{ETag: FormatETag(set.Period, set.Version)}
	for _, b := range items[inicio:fin] {
		page.Items = append(page.Items, Item{Period: set.Period, Currency: set.Currency, Version: set.Version, CategoryID: b.CategoryID, LimitAmountCents: b.LimitAmountCents})
	}
	if fin < len(items) {
		cursor, err := httputil.GenerateCursor(s.cursorKey, userID, firma, fmt.Sprintf("%d", fin), s.now().UTC(), httputil.DefaultCursorTTL)
		if err != nil {
			return nil, ErrInternal
		}
		page.NextCursor = cursor
	}
	return page, nil
}

func (s *Service) paginateAll(userID string, sets []Set, q query) (*Page, error) {
	sort.Slice(sets, func(i, j int) bool { return sets[i].Period < sets[j].Period })
	type fila struct {
		period   string
		currency string
		version  int64
		budget   Budget
	}
	filas := make([]fila, 0)
	for _, set := range sets {
		items := append([]Budget(nil), set.Items...)
		sort.Slice(items, func(i, j int) bool { return items[i].CategoryID < items[j].CategoryID })
		for _, b := range items {
			filas = append(filas, fila{period: set.Period, currency: set.Currency, version: set.Version, budget: b})
		}
	}
	inicio := 0
	firma := fmt.Sprintf("all;limit=%d", q.Limit)
	if q.Cursor != "" {
		payload, err := httputil.ValidateCursor(s.cursorKey, q.Cursor, userID)
		if err != nil || payload.SortBy != firma || payload.LastID == "" {
			return nil, fmt.Errorf("%w: cursor", ErrInvalidRequest)
		}
		if _, err := fmt.Sscanf(payload.LastID, "%d", &inicio); err != nil || inicio < 0 || inicio > len(filas) {
			return nil, fmt.Errorf("%w: cursor", ErrInvalidRequest)
		}
	}
	fin := inicio + q.Limit
	if fin > len(filas) {
		fin = len(filas)
	}
	page := &Page{Items: make([]Item, 0, fin-inicio)}
	for _, f := range filas[inicio:fin] {
		page.Items = append(page.Items, Item{Period: f.period, Currency: f.currency, Version: f.version, CategoryID: f.budget.CategoryID, LimitAmountCents: f.budget.LimitAmountCents})
	}
	if fin < len(filas) {
		cursor, err := httputil.GenerateCursor(s.cursorKey, userID, firma, fmt.Sprintf("%d", fin), s.now().UTC(), httputil.DefaultCursorTTL)
		if err != nil {
			return nil, ErrInternal
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// FormatETag devuelve el validador fuerte del periodo.
func FormatETag(period string, version int64) string {
	return fmt.Sprintf("\"b%s-%d\"", period, version)
}

// Wire proyecta la página al contrato.
func (p *Page) Wire() []WireBudget {
	salida := make([]WireBudget, 0, len(p.Items))
	for _, item := range p.Items {
		salida = append(salida, WireBudget{
			CategoryID:  item.CategoryID,
			Period:      item.Period,
			LimitAmount: httputil.FormatCentsToDecimal(item.LimitAmountCents),
			Currency:    item.Currency,
			Version:     item.Version,
		})
	}
	return salida
}
