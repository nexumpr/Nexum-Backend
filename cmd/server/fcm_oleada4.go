package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/push"
)

const (
	variableProjectIDNueva      = "NEXUM_FCM_PROJECT_ID"
	variableProjectIDAlias      = "FIREBASE_PROJECT_ID"
	variableCuentaServicioNueva = "NEXUM_FCM_CREDENTIALS_JSON"
	variableCuentaServicioAlias = "FCM_SERVICE_ACCOUNT_JSON"
	variableArchivoNueva        = "NEXUM_FCM_CREDENTIALS_FILE"
	variableArchivoAlias        = "GOOGLE_APPLICATION_CREDENTIALS"

	timeoutFCMPorDefecto = 5 * time.Second
	timeoutFCMAximo      = 60 * time.Second

	// directorioConfiguracionFCM es el único árbol desde el que se acepta una
	// credencial en archivo. El valor de la variable de entorno no puede
	// convertirse en una lectura arbitraria del sistema (G304).
	directorioConfiguracionFCM = "/etc/nexum"
)

// configuracionFCMOleada4DesdeEntorno construye la configuración del emisor
// real. Prioridad: nombres NEXUM_* nuevos, después los alias del runbook.
// Nunca imprime ni devuelve el contenido de la credencial; los errores son
// claros y no incluyen secretos.
func configuracionFCMOleada4DesdeEntorno() (push.Config, error) {
	proyecto := strings.TrimSpace(primerValorEnv(variableProjectIDNueva, variableProjectIDAlias))
	if proyecto == "" {
		return push.Config{}, fmt.Errorf("FCM Oleada 4: se requiere %s (o su alias %s)",
			variableProjectIDNueva, variableProjectIDAlias)
	}

	contenido := primerValorEnv(variableCuentaServicioNueva, variableCuentaServicioAlias)
	ruta := strings.TrimSpace(primerValorEnv(variableArchivoNueva, variableArchivoAlias))
	var credencial []byte
	switch {
	case contenido != "":
		credencial = []byte(contenido)
	case ruta != "":
		leido, err := leerCredencialFCM(ruta, directorioConfiguracionFCM)
		if err != nil {
			return push.Config{}, errors.New("FCM Oleada 4: no se pudo leer la credencial de servicio configurada")
		}
		credencial = leido
	default:
		return push.Config{}, fmt.Errorf("FCM Oleada 4: se requiere %s o %s (o sus alias %s/%s)",
			variableCuentaServicioNueva, variableArchivoNueva, variableCuentaServicioAlias, variableArchivoAlias)
	}

	timeout := timeoutFCMPorDefecto
	if crudo := strings.TrimSpace(os.Getenv("NEXUM_FCM_TIMEOUT_MS")); crudo != "" {
		milisegundos, err := strconv.Atoi(crudo)
		if err != nil || milisegundos <= 0 {
			return push.Config{}, errors.New("FCM Oleada 4: NEXUM_FCM_TIMEOUT_MS debe ser un entero positivo")
		}
		timeout = time.Duration(milisegundos) * time.Millisecond
		if timeout > timeoutFCMAximo {
			return push.Config{}, fmt.Errorf("FCM Oleada 4: NEXUM_FCM_TIMEOUT_MS no puede exceder %d", timeoutFCMAximo.Milliseconds())
		}
	}

	cfg := push.Config{
		ProjectID:       proyecto,
		CredentialsJSON: credencial,
		Endpoint:        strings.TrimSpace(os.Getenv("NEXUM_FCM_ENDPOINT")),
		HTTPClient:      &http.Client{Timeout: timeout},
		Timeout:         timeout,
	}
	if err := cfg.Validar(); err != nil {
		return push.Config{}, err
	}
	return cfg, nil
}

func primerValorEnv(nombres ...string) string {
	for _, nombre := range nombres {
		if valor := strings.TrimSpace(os.Getenv(nombre)); valor != "" {
			return valor
		}
	}
	return ""
}

// leerCredencialFCM lee una credencial de archivo con dos límites: la ruta debe ser
// absoluta y debe estar dentro de `base`. `os.OpenRoot` impide además escapar del
// árbol mediante symlinks. El contenido se acota a 1 MiB. Sin esta validación, un
// valor de entorno podría convertirse en una lectura arbitraria (G304).
func leerCredencialFCM(ruta, base string) ([]byte, error) {
	ruta = strings.TrimSpace(ruta)
	if ruta == "" || !filepath.IsAbs(ruta) {
		return nil, errors.New("la ruta de credencial debe ser absoluta")
	}
	raizDir := filepath.Clean(base)
	if !filepath.IsAbs(raizDir) {
		return nil, errors.New("directorio de credenciales no configurado")
	}
	relativo, err := filepath.Rel(raizDir, filepath.Clean(ruta))
	if err != nil || relativo == "." || relativo == ".." ||
		strings.HasPrefix(relativo, ".."+string(filepath.Separator)) {
		return nil, errors.New("la credencial debe estar dentro del directorio permitido")
	}
	raiz, err := os.OpenRoot(raizDir)
	if err != nil {
		return nil, errors.New("no se pudo abrir el directorio de credenciales")
	}
	defer raiz.Close()
	archivo, err := raiz.Open(relativo)
	if err != nil {
		return nil, errors.New("no se pudo abrir la credencial")
	}
	defer archivo.Close()
	contenido, err := io.ReadAll(io.LimitReader(archivo, 1<<20))
	if err != nil {
		return nil, errors.New("no se pudo leer la credencial")
	}
	return contenido, nil
}

// iniciarFCMWorkerOleada4 construye el cliente real, el outbox durable, el
// servicio y arranca el worker con ticker. No registra rutas ni usa init().
// Devuelve una función de cierre idempotente.
func iniciarFCMWorkerOleada4(ctx context.Context, d *dependenciasOleada4, cfg push.Config) (func(), error) {
	return iniciarFCMWorkerOleada4Con(ctx, d, cfg, nil, nil)
}

// iniciarFCMWorkerSinEmisorOleada4 arranca el outbox sin emisor FCM. El worker
// sigue procesando: cierra SIN_DISPOSITIVOS los trabajos sin destino y
// reprograma CONFIG_FCM los que si lo tienen. Es el modo degradado que evita
// que un fallo de configuracion deje trabajos pendientes para siempre.
func iniciarFCMWorkerSinEmisorOleada4(ctx context.Context, d *dependenciasOleada4) (func(), error) {
	return iniciarWorkerOutboxOleada4(ctx, d, nil, nil, nil)
}

// iniciarOutboxPushOleada4 arranca el outbox con emisor FCM si la configuracion
// es valida y, si no lo es, en modo degradado sin emisor. En ambos casos el
// outbox se procesa; el modo se devuelve para registrarlo sin secretos.
func iniciarOutboxPushOleada4(ctx context.Context, d *dependenciasOleada4, logger *slog.Logger) (func(), string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	cfg, errCfg := configuracionFCMOleada4DesdeEntorno()
	if errCfg == nil {
		cerrar, errInicio := iniciarFCMWorkerOleada4(ctx, d, cfg)
		if errInicio == nil {
			return cerrar, "http-v1-oauth2-jwt-rs256", nil
		}
		logger.WarnContext(ctx, "FCM Oleada 4: emisor no disponible; worker en modo degradado",
			slog.String("clase", "FCM_WORKER_NO_INICIADO"))
	} else {
		logger.WarnContext(ctx, "FCM Oleada 4: credencial no inyectada; worker en modo degradado",
			slog.String("clase", "FCM_CREDENCIAL_AUSENTE"))
	}
	cerrar, errDegradado := iniciarFCMWorkerSinEmisorOleada4(ctx, d)
	if errDegradado != nil {
		return nil, "", errDegradado
	}
	return cerrar, "sin-emisor", nil
}

// iniciarFCMWorkerOleada4Con permite al responsable inyectar la fuente de
// tokens y/u otra bandeja E061 sin modificar este archivo.
func iniciarFCMWorkerOleada4Con(
	ctx context.Context,
	d *dependenciasOleada4,
	cfg push.Config,
	fuente pushoutbox.TokenSource,
	bandeja pushoutbox.NotificationSink,
) (func(), error) {
	return iniciarWorkerOutboxOleada4(ctx, d, &cfg, fuente, bandeja)
}

// iniciarWorkerOutboxOleada4 es la construccion comun. cfg nil significa modo
// degradado sin emisor: no se intenta ningun envio y tampoco se provoca un panic.
func iniciarWorkerOutboxOleada4(
	ctx context.Context,
	d *dependenciasOleada4,
	cfg *push.Config,
	fuente pushoutbox.TokenSource,
	bandeja pushoutbox.NotificationSink,
) (func(), error) {
	if ctx == nil {
		return nil, errors.New("FCM Oleada 4: contexto nulo")
	}
	var emisor push.Emisor
	modo := "sin-emisor"
	if cfg != nil {
		if err := cfg.Validar(); err != nil {
			return nil, err
		}
		clienteFCM, err := push.NewClient(*cfg)
		if err != nil {
			return nil, err
		}
		emisor = clienteFCM
		modo = "http-v1-oauth2-jwt-rs256"
	}
	if d == nil || d.Dynamo == nil || strings.TrimSpace(d.Tabla) == "" {
		return nil, errors.New("FCM Oleada 4: DynamoDB o tabla no configurados; worker no iniciado")
	}
	cliente, ok := d.Dynamo.(dynamoAdapter.PushOutboxAPI)
	if !ok || cliente == nil {
		return nil, errors.New("FCM Oleada 4: el cliente DynamoDB no expone Query/GetItem/PutItem/UpdateItem")
	}

	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	store := dynamoAdapter.NewPushOutboxStore(cliente, d.Tabla)

	// Preferencias ya es un campo real de dependenciasOleada4. Si no está
	// conectado, el servicio falla cerrado: no envía nada.
	var preferencias pushoutbox.PreferenceReader
	if d.Preferencias != nil {
		preferencias = d.Preferencias
	} else {
		logger.WarnContext(ctx, "FCM Oleada 4: Preferencias no conectada; el worker no enviará nada",
			slog.String("clase", "PREFERENCIAS_NO_CONECTADAS"))
	}

	// PushSealer ya es un campo real de dependenciasOleada4: acceso directo, sin
	// reflexión. El tipo estático expone Sellar; el sellador concreto también
	// implementa Abrir, así que se verifica por aserción de interfaz.
	abridor, okAbrir := d.PushSealer.(pushoutbox.TokenAbridor)
	if fuente == nil {
		if !okAbrir || abridor == nil {
			if cfg != nil {
				return nil, errors.New("FCM Oleada 4: PushSealer debe exponer Abrir([]byte,string)(string,error) o inyecte una fuente de tokens")
			}
			// Modo degradado sin emisor y sin sellador: el worker sigue procesando
			// para que los trabajos no queden pendientes para siempre.
			logger.WarnContext(ctx, "FCM Oleada 4: sin sellador de tokens; los trabajos quedaran TOKENS_NO_DISPONIBLES",
				slog.String("clase", "PUSH_SELLER_NO_DISPONIBLE"))
		} else {
			fuente = nuevaFuenteTokensDynamo(cliente, d.Tabla, abridor, logger)
		}
	}
	if bandeja == nil {
		bandeja = store
	}

	servicio := pushoutbox.NewService(store, emisor, preferencias, fuente, bandeja, pushoutbox.Opciones{
		Logger: logger,
	})
	ctxWorker, cancelar := context.WithCancel(ctx)
	go servicio.Run(ctxWorker)

	logger.InfoContext(ctx, "FCM Oleada 4: worker iniciado",
		slog.String("modo", modo),
		slog.String("outbox", "OUTBOX#GLOBAL/DUE#"),
	)
	var cerrado bool
	return func() {
		if cerrado {
			return
		}
		cerrado = true
		cancelar()
	}, nil
}

// fuenteTokensDynamo lee USER#<sub>/DEVICE#<id> tal como lo escribe E059 y
// abre el token cifrado en reposo. El atributo primario es tokenCipher (string
// base64); se conservan alias binarios sólo para despliegues previos. El AAD es
// exactamente "<userID>|<deviceID>". Sólo se observa el ID del dispositivo: el
// valor del token nunca se registra.
type fuenteTokensDynamo struct {
	cliente dynamoAdapter.PushOutboxAPI
	tabla   string
	abridor pushoutbox.TokenAbridor
	logger  *slog.Logger
}

func nuevaFuenteTokensDynamo(cliente dynamoAdapter.PushOutboxAPI, tabla string, abridor pushoutbox.TokenAbridor, logger *slog.Logger) *fuenteTokensDynamo {
	return &fuenteTokensDynamo{cliente: cliente, tabla: strings.TrimSpace(tabla), abridor: abridor, logger: logger}
}

func (f *fuenteTokensDynamo) TokensActivos(ctx context.Context, userID string) ([]pushoutbox.TokenDestino, error) {
	if f == nil || f.cliente == nil || f.tabla == "" || f.abridor == nil {
		return nil, errors.New("fuente de tokens push no configurada")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("usuario push ausente")
	}
	salida, err := f.cliente.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(f.tabla),
		KeyConditionExpression: aws.String("#pk = :pk AND begins_with(#sk, :prefijo)"),
		ExpressionAttributeNames: map[string]string{
			"#pk": "PK", "#sk": "SK",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":      &types.AttributeValueMemberS{Value: "USER#" + userID},
			":prefijo": &types.AttributeValueMemberS{Value: "DEVICE#"},
		},
	})
	if err != nil {
		return nil, errors.New("no se pudieron leer los dispositivos push")
	}
	tokens := make([]pushoutbox.TokenDestino, 0, len(salida.Items))
	for _, item := range salida.Items {
		sk := atributoTexto(item["SK"])
		deviceID := strings.TrimPrefix(sk, "DEVICE#")
		if deviceID == "" || deviceID == sk {
			continue
		}
		// E059/E060: entityType DEVICE, active booleano y tokenCipher en base64.
		if atributoTexto(item["entityType"]) != "DEVICE" {
			continue
		}
		if activo, presente := item["active"].(*types.AttributeValueMemberBOOL); !presente || !activo.Value {
			continue
		}
		if proveedor := atributoTexto(item["provider"]); proveedor != "" && proveedor != "fcm" {
			continue
		}
		sobre := atributoBinario(item["tokenCipher"])
		if len(sobre) == 0 {
			sobre = atributoBinario(item["tokenSello"])
		}
		if len(sobre) == 0 {
			sobre = atributoBinario(item["tokenSealed"])
		}
		if len(sobre) == 0 {
			sobre = atributoBinario(item["sealedToken"])
		}
		if len(sobre) == 0 {
			continue
		}
		token, errAbrir := f.abridor.Abrir(sobre, userID+"|"+deviceID)
		if errAbrir != nil || strings.TrimSpace(token) == "" {
			if f.logger != nil {
				f.logger.WarnContext(ctx, "dispositivo push con token ilegible omitido",
					slog.String("clase", "TOKEN_CIPHER_ILEGIBLE"))
			}
			continue
		}
		tokens = append(tokens, pushoutbox.TokenDestino{ID: deviceID, Valor: token})
	}
	return tokens, nil
}

// RevocarTokenPush replica la baja de E060 al recibir UNREGISTERED: active=false,
// revokedAt, version+1 y REMOVE de tokenCipher/tokenFingerprint. Es condicional
// a active=true para no reactivar ni sobrescribir una baja ya registrada.
func (f *fuenteTokensDynamo) RevocarTokenPush(ctx context.Context, userID, tokenID string) error {
	if f == nil || f.cliente == nil || f.tabla == "" {
		return errors.New("fuente de tokens push no configurada")
	}
	userID = strings.TrimSpace(userID)
	tokenID = strings.TrimSpace(tokenID)
	if userID == "" || tokenID == "" {
		return errors.New("dispositivo push no identificable")
	}
	_, err := f.cliente.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(f.tabla),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#" + userID},
			"SK": &types.AttributeValueMemberS{Value: "DEVICE#" + tokenID},
		},
		UpdateExpression: aws.String("SET #active = :inactive, #revokedAt = :revokedAt, " +
			"#version = #version + :one REMOVE #tokenCipher, #tokenFingerprint"),
		ConditionExpression: aws.String("#active = :active"),
		ExpressionAttributeNames: map[string]string{
			"#active":           "active",
			"#revokedAt":        "revokedAt",
			"#version":          "version",
			"#tokenCipher":      "tokenCipher",
			"#tokenFingerprint": "tokenFingerprint",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":active":    &types.AttributeValueMemberBOOL{Value: true},
			":inactive":  &types.AttributeValueMemberBOOL{Value: false},
			":revokedAt": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)},
			":version":   &types.AttributeValueMemberN{Value: "1"},
			":one":       &types.AttributeValueMemberN{Value: "1"},
		},
	})
	return err
}

func atributoTexto(valor types.AttributeValue) string {
	switch v := valor.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	default:
		return ""
	}
}

func atributoBinario(valor types.AttributeValue) []byte {
	switch v := valor.(type) {
	case *types.AttributeValueMemberB:
		return v.Value
	case *types.AttributeValueMemberS:
		decodificado, err := base64.StdEncoding.DecodeString(v.Value)
		if err == nil {
			return decodificado
		}
	}
	return nil
}
