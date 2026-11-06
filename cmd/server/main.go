package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nexumpr/Nexum-Backend/internal/system"
	"github.com/nexumpr/Nexum-Backend/pkg/types"
)

func main() {
	envStr := os.Getenv("APP_ENV")
	var meta types.SystemMetadata

	if envStr == "PRODUCTION" {
		meta = system.DefaultProductionMetadata()
	} else {
		meta = system.DefaultLocalMetadata()
	}

	sysCtx, err := system.NewSystemContext(meta)
	if err != nil {
		log.Fatalf("Error crítico al inicializar contexto de sistema (A-000): %v", err)
	}

	fmt.Printf("=== %s v%s ===\n", sysCtx.Metadata.AppName, sysCtx.Metadata.Version)
	fmt.Printf("Especificación de Contexto: %s\n", sysCtx.Metadata.SpecVersion)
	fmt.Printf("Entorno: %s | Nodos Mínimos: %d | TLS: %t\n", sysCtx.Metadata.Environment, sysCtx.Metadata.MinNodeCount, sysCtx.Metadata.TLSRequired)
	fmt.Printf("Persistencia: %s | Caché: %s\n", sysCtx.Metadata.NoSQLProvider, sysCtx.Metadata.CacheProvider)
	fmt.Println("Fronteras del sistema validadas exitosamente conforme a A-000 y Constitución GOV-01..18.")
}

