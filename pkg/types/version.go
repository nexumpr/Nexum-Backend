package types

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	semVerRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)
)

// SemVer representa una versión semántica estructurada (A-110 §2.1, AC-A-110-01).
type SemVer struct {
	Major      int    `json:"major"`
	Minor      int    `json:"minor"`
	Patch      int    `json:"patch"`
	PreRelease string `json:"pre_release,omitempty"`
	BuildMeta  string `json:"build_meta,omitempty"`
}

// Constantes vigentes del sistema Nexum
const (
	CurrentBackendVersion = "0.8.0"
	CurrentAPIPrefix      = "/api/v1"
	CurrentSDDVersion     = "0.2.0"
	TotalCanonicalSpecs   = 103
)

// ParseSemVer analiza una cadena de versión y devuelve una estructura SemVer.
func ParseSemVer(v string) (*SemVer, error) {
	v = strings.TrimSpace(v)
	matches := semVerRegex.FindStringSubmatch(v)
	if len(matches) == 0 {
		return nil, errors.New("formato de version SemVer invalido (debe cumplir MAJOR.MINOR.PATCH)")
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("major version invalida: %w", err)
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("minor version invalida: %w", err)
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("patch version invalida: %w", err)
	}

	return &SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		PreRelease: matches[4],
		BuildMeta:  matches[5],
	}, nil
}

// String retorna la representación canónica de la versión semántica.
func (s *SemVer) String() string {
	res := fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
	if s.PreRelease != "" {
		res += "-" + s.PreRelease
	}
	if s.BuildMeta != "" {
		res += "+" + s.BuildMeta
	}
	return res
}

// ValidateAPIRoute verifica que una ruta comience con el prefijo canónico /api/v1.
func ValidateAPIRoute(path string) error {
	if !strings.HasPrefix(path, CurrentAPIPrefix) {
		return fmt.Errorf("ruta no versionada: debe iniciar con el prefijo canónico %s", CurrentAPIPrefix)
	}
	return nil
}
