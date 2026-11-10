package domain_test

import (
	"regexp"
	"testing"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

func TestNewUserIDFormatAndUniqueness(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := make(map[string]struct{})

	for i := 0; i < 128; i++ {
		id, err := identity.NewUserID()
		if err != nil {
			t.Fatalf("NewUserID fallo: %v", err)
		}
		if !pattern.MatchString(id) {
			t.Fatalf("UUID con formato/version/variante invalidos: %q", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("UUID duplicado: %q", id)
		}
		seen[id] = struct{}{}
	}
}
