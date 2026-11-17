package ses

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// Mailer implementa ports.Correo con Amazon SES v2 (API HTTPS, no SMTP).
//
// Nexum es solo Android: no existe ni se crea frontend web. El correo incluye el
// token para que la app lo introduzca en el flujo de E006; no hay URL de recuperacion.
type Mailer struct {
	client *sesv2.Client
	from   string
}

// NewMailer construye el adaptador. Sin cliente o sin remitente queda inerte.
func NewMailer(client *sesv2.Client, from string) *Mailer {
	return &Mailer{client: client, from: from}
}

// Disponible indica si el adaptador puede enviar.
func (m *Mailer) Disponible() bool {
	return m != nil && m.client != nil && m.from != ""
}

// EnviarRecuperacion envia el enlace de un solo uso.
//
// Devuelve el MessageId de SES (identificador de entrega, no confirma recepcion).
func (m *Mailer) EnviarRecuperacion(ctx context.Context, destino, token string, expiraEn time.Time) (string, error) {
	if !m.Disponible() {
		return "", fmt.Errorf("correo no configurado")
	}
	texto := fmt.Sprintf(
		"Recibimos una solicitud para restablecer tu contrasena de Nexum.\n\n"+
			"Introduce este codigo en la aplicacion Android antes de %s (hora UTC):\n\n"+
			"%s\n\n"+
			"El codigo es de un solo uso. Si no solicitaste el cambio, ignora este mensaje: "+
			"tu contrasena no se modificara.\n",
		expiraEn.UTC().Format("2006-01-02 15:04"), token,
	)

	out, err := m.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.from),
		Destination:      &types.Destination{ToAddresses: []string{destino}},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String("Recupera tu contrasena de Nexum")},
				Body: &types.Body{
					Text: &types.Content{Data: aws.String(texto)},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ses send-email: %w", err)
	}
	return aws.ToString(out.MessageId), nil
}

// EnviarVerificacion envia el codigo de verificacion de correo de E010.
//
// Nexum es solo Android: no existe ni se crea frontend web. El correo incluye el codigo para
// que la app lo introduzca en E011; no hay URL de verificacion. Devuelve el MessageId de SES
// (identificador de entrega, no confirma recepcion) y nunca registra el token.
func (m *Mailer) EnviarVerificacion(ctx context.Context, destino, token string, expiraEn time.Time) (string, error) {
	if !m.Disponible() {
		return "", fmt.Errorf("correo no configurado")
	}
	texto := fmt.Sprintf(
		"Este es el codigo de verificacion de correo de tu cuenta de Nexum.\n\n"+
			"Introduce este codigo en la aplicacion Android antes de %s (hora UTC):\n\n"+
			"%s\n\n"+
			"El codigo es de un solo uso y caduca. Si no solicitaste la verificacion, "+
			"ignora este mensaje: tu correo no se marcara como verificado.\n",
		expiraEn.UTC().Format("2006-01-02 15:04"), token,
	)

	out, err := m.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.from),
		Destination:      &types.Destination{ToAddresses: []string{destino}},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String("Verifica tu correo de Nexum")},
				Body: &types.Body{
					Text: &types.Content{Data: aws.String(texto)},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ses send-email verificacion: %w", err)
	}
	return aws.ToString(out.MessageId), nil
}
