package cache_test

import (
	"strings"
	"testing"

	domainCache "github.com/nexumpr/Nexum-Backend/internal/domain/cache"
)

func TestHashIdentifierEsEstableNormalizaYNoExponePII(t *testing.T) {
	base := domainCache.HashIdentifier("usuario-123")
	if len(base) != 16 {
		t.Fatalf("hash esperado de 16 hex, obtenido %d", len(base))
	}
	if base != domainCache.HashIdentifier("  usuario-123  ") {
		t.Errorf("el hash debe normalizar espacios: %q != %q", base, domainCache.HashIdentifier("  usuario-123  "))
	}
	if base == domainCache.HashIdentifier("usuario-124") {
		t.Errorf("identificadores distintos no deben colisionar")
	}
	if strings.Contains(base, "usuario") {
		t.Errorf("el hash no debe contener el identificador original")
	}
}

func TestUserCacheKeyCanonica(t *testing.T) {
	key := domainCache.UserCacheKey("prod", "user-1", "dashboard", `{"mes":"2026-11"}`, 7)
	partes := strings.Split(key, ":")
	if len(partes) != 7 {
		t.Fatalf("formato inesperado: %q", key)
	}
	if partes[0] != "nexum" || partes[1] != "v1" || partes[2] != "prod" {
		t.Errorf("prefijo/entorno incorrectos: %q", key)
	}
	if partes[3] != domainCache.HashIdentifier("user-1") {
		t.Errorf("hash de usuario incorrecto en %q", key)
	}
	if partes[4] != "dashboard" || partes[6] != "v7" {
		t.Errorf("recurso/version incorrectos en %q", key)
	}
	if len(partes[5]) != 8 {
		t.Errorf("hash de parámetros debe tener 8 hex: %q", partes[5])
	}
	sinParams := domainCache.UserCacheKey("", "user-1", "dashboard", "", 0)
	if !strings.Contains(sinParams, ":local:") || !strings.Contains(sinParams, ":default:v0") {
		t.Errorf("entorno por defecto/params vacíos mal resueltos: %q", sinParams)
	}
	distinta := domainCache.UserCacheKey("prod", "user-1", "dashboard", `{"mes":"2026-12"}`, 7)
	if distinta == key {
		t.Errorf("parámetros distintos deben producir claves distintas")
	}
}

func TestUserCachePrefix(t *testing.T) {
	prefix := domainCache.UserCachePrefix("prod", "user-1")
	esperado := "nexum:v1:prod:" + domainCache.HashIdentifier("user-1") + ":"
	if prefix != esperado {
		t.Errorf("prefijo esperado %q, obtenido %q", esperado, prefix)
	}
	if !strings.HasSuffix(domainCache.UserCachePrefix("", "user-1"), ":") {
		t.Errorf("el prefijo debe terminar en ':'")
	}
	if !strings.Contains(domainCache.UserCachePrefix("", "user-1"), ":local:") {
		t.Errorf("entorno por defecto no aplicado")
	}
}

func TestPublicCatalogCacheKey(t *testing.T) {
	if got := domainCache.PublicCatalogCacheKey("prod", "divisas", 3); got != "nexum:v1:prod:public:catalog:divisas:v3" {
		t.Errorf("clave de catálogo inesperada: %q", got)
	}
	if got := domainCache.PublicCatalogCacheKey("", "categorias", 1); got != "nexum:v1:local:public:catalog:categorias:v1" {
		t.Errorf("entorno por defecto no aplicado: %q", got)
	}
}

func TestRateLimitKey(t *testing.T) {
	key := domainCache.RateLimitKey("auth", "10.0.0.1")
	if !strings.HasPrefix(key, "nexum:rl:auth:") {
		t.Errorf("prefijo inesperado: %q", key)
	}
	if strings.Contains(key, "10.0.0.1") {
		t.Errorf("el sujeto no debe aparecer en claro: %q", key)
	}
	if key != domainCache.RateLimitKey("auth", "10.0.0.1") {
		t.Errorf("la clave debe ser determinista")
	}
	if key == domainCache.RateLimitKey("auth", "10.0.0.2") {
		t.Errorf("sujetos distintos deben producir claves distintas")
	}
}
