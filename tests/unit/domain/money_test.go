package domain_test

import (
	"errors"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
)

func TestMoneyCreation(t *testing.T) {
	t.Run("Creacion valida de Money", func(t *testing.T) {
		m, err := money.New(15000, "MXN")
		if err != nil {
			t.Fatalf("error inesperado al crear Money: %v", err)
		}
		if m.Amount() != 15000 {
			t.Errorf("monto esperado 15000, obtenido %d", m.Amount())
		}
		if m.Currency() != "MXN" {
			t.Errorf("divisa esperada MXN, obtenida %s", m.Currency())
		}
	})

	t.Run("Rechazo de codigo de divisa invalido", func(t *testing.T) {
		invalidCurrencies := []string{"", "M", "MX", "PESOS", "123"}
		for _, curr := range invalidCurrencies {
			_, err := money.New(100, curr)
			if err == nil {
				t.Errorf("se esperaba error para divisa invalida '%s'", curr)
			}
			if !errors.Is(err, money.ErrInvalidCurrency) {
				t.Errorf("se esperaba ErrInvalidCurrency para '%s', obtenido: %v", curr, err)
			}
		}
	})

	t.Run("MustNew no entra en panico con divisa valida", func(t *testing.T) {
		m := money.MustNew(500, "USD")
		if m.Amount() != 500 || m.Currency() != "USD" {
			t.Errorf("MustNew creo Money incorrecto: %+v", m)
		}
	})
}

func TestMoneyArithmetic(t *testing.T) {
	m100 := money.MustNew(10000, "MXN") // 100.00 MXN
	m50 := money.MustNew(5000, "MXN")   // 50.00 MXN
	mUSD := money.MustNew(10000, "USD") // 100.00 USD

	t.Run("Suma de importes en misma divisa", func(t *testing.T) {
		sum, err := m100.Add(m50)
		if err != nil {
			t.Fatalf("error inesperado en suma: %v", err)
		}
		if sum.Amount() != 15000 {
			t.Errorf("suma esperada 15000, obtenida %d", sum.Amount())
		}
		if sum.Currency() != "MXN" {
			t.Errorf("divisa esperada MXN, obtenida %s", sum.Currency())
		}
	})

	t.Run("Rechazo de suma con diferente divisa", func(t *testing.T) {
		_, err := m100.Add(mUSD)
		if err == nil {
			t.Fatal("se esperaba error al sumar MXN y USD")
		}
		if !errors.Is(err, money.ErrCurrencyMismatch) {
			t.Errorf("se esperaba ErrCurrencyMismatch, obtenido: %v", err)
		}
	})

	t.Run("Resta de importes en misma divisa", func(t *testing.T) {
		diff, err := m100.Subtract(m50)
		if err != nil {
			t.Fatalf("error inesperado en resta: %v", err)
		}
		if diff.Amount() != 5000 {
			t.Errorf("diferencia esperada 5000, obtenida %d", diff.Amount())
		}
	})

	t.Run("Rechazo de resta con diferente divisa", func(t *testing.T) {
		_, err := m100.Subtract(mUSD)
		if err == nil {
			t.Fatal("se esperaba error al restar MXN y USD")
		}
		if !errors.Is(err, money.ErrCurrencyMismatch) {
			t.Errorf("se esperaba ErrCurrencyMismatch, obtenido: %v", err)
		}
	})

	t.Run("Multiplicacion escalar", func(t *testing.T) {
		prod := m50.Multiply(3)
		if prod.Amount() != 15000 {
			t.Errorf("producto esperado 15000, obtenido %d", prod.Amount())
		}
	})
}

func TestMoneyPredicatesAndFormatting(t *testing.T) {
	mPos := money.MustNew(10050, "MXN")
	mNeg := money.MustNew(-5000, "USD")
	mZero := money.MustNew(0, "EUR")

	if !mPos.IsPositive() || mPos.IsNegative() || mPos.IsZero() {
		t.Errorf("predicados invalidos para importe positivo: %+v", mPos)
	}
	if !mNeg.IsNegative() || mNeg.IsPositive() || mNeg.IsZero() {
		t.Errorf("predicados invalidos para importe negativo: %+v", mNeg)
	}
	if !mZero.IsZero() || mZero.IsPositive() || mZero.IsNegative() {
		t.Errorf("predicados invalidos para importe cero: %+v", mZero)
	}

	if mPos.Formatted() != "100.50 MXN" {
		t.Errorf("formato esperado '100.50 MXN', obtenido '%s'", mPos.Formatted())
	}
	if mNeg.Formatted() != "-50.00 USD" {
		t.Errorf("formato esperado '-50.00 USD', obtenido '%s'", mNeg.Formatted())
	}
	if mZero.Formatted() != "0.00 EUR" {
		t.Errorf("formato esperado '0.00 EUR', obtenido '%s'", mZero.Formatted())
	}
}

