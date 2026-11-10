package identity

import "time"

// attrString lee el primer atributo string no vacío. Permite convivir con las
// claves canónicas de E001 y con las históricas de A-050 sin duplicar datos.
func attrString(item map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := item[name].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

// attrTime lee el primer atributo temporal RFC3339 válido.
func attrTime(item map[string]any, names ...string) time.Time {
	for _, name := range names {
		if value, ok := item[name].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
