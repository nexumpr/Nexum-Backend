package dynamo

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
)

// TestE020ExportReminderRowFrecuencia cierra la frontera del adaptador: `frequency`
// ausente o vacía se normaliza a `nil` (sin recurrencia) y un valor fuera del enum se
// rechaza como item incoherente antes de llegar a la proyección del contrato.
func TestE020ExportReminderRowFrecuencia(t *testing.T) {
	casos := []struct {
		nombre     string
		frecuencia *string
		esperado   *string
		incoherent bool
	}{
		{nombre: "sin atributo", frecuencia: nil, esperado: nil},
		{nombre: "cadena vacía", frecuencia: cadenaPrueba(""), esperado: nil},
		{nombre: "sólo espacios", frecuencia: cadenaPrueba("   "), esperado: nil},
		{nombre: "daily", frecuencia: cadenaPrueba("daily"), esperado: cadenaPrueba("daily")},
		{nombre: "weekly", frecuencia: cadenaPrueba("weekly"), esperado: cadenaPrueba("weekly")},
		{nombre: "monthly", frecuencia: cadenaPrueba("monthly"), esperado: cadenaPrueba("monthly")},
		{nombre: "hourly fuera del enum", frecuencia: cadenaPrueba("hourly"), incoherent: true},
		{nombre: "mayúsculas fuera del enum", frecuencia: cadenaPrueba("DAILY"), incoherent: true},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			item := itemRecordatorioExport(caso.frecuencia)
			fila, err := exportReminderRow(item, "user-a")
			if caso.incoherent {
				if !errors.Is(err, appExport.ErrInconsistent) {
					t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("exportReminderRow: %v", err)
			}
			if caso.esperado == nil {
				if fila.Frequency != nil {
					t.Fatalf("frequency=%q, esperado nil", *fila.Frequency)
				}
				return
			}
			if fila.Frequency == nil || *fila.Frequency != *caso.esperado {
				t.Fatalf("frequency=%v, esperado %q", fila.Frequency, *caso.esperado)
			}
		})
	}

	// El adaptador no acepta ni siquiera el item de otro titular.
	if _, err := exportReminderRow(itemRecordatorioExport(cadenaPrueba("daily")), "user-b"); !errors.Is(err, appExport.ErrInconsistent) {
		t.Fatalf("item de otro titular: esperado ErrInconsistent, obtenido %v", err)
	}
}

func cadenaPrueba(valor string) *string { return &valor }

func itemRecordatorioExport(frecuencia *string) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"entityType": &types.AttributeValueMemberS{Value: "USER_REMINDER"},
		"userId":     &types.AttributeValueMemberS{Value: "user-a"},
		"reminderID": &types.AttributeValueMemberS{Value: "8f14e45f-ceea-4a3f-9c1e-2b3f6a0d7c11"},
		"title":      &types.AttributeValueMemberS{Value: "Pagar luz"},
		"dueAt":      &types.AttributeValueMemberS{Value: "2026-11-24T10:00:00Z"},
		"timezone":   &types.AttributeValueMemberS{Value: "America/Mexico_City"},
		"active":     &types.AttributeValueMemberBOOL{Value: true},
		"createdAt":  &types.AttributeValueMemberS{Value: "2026-11-23T09:00:00Z"},
		"updatedAt":  &types.AttributeValueMemberS{Value: "2026-11-23T09:30:00Z"},
	}
	if frecuencia != nil {
		item["frequency"] = &types.AttributeValueMemberS{Value: *frecuencia}
	}
	return item
}
