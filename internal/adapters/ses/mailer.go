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
type Mailer struct {
	client  *sesv2.Client
	from    string
	baseURL string
}

// NewMailer construye el adaptador. Sin cliente o sin remitente queda inerte.
func NewMailer(client *sesv2.Client, from, baseURL string) *Mailer {
	return &Mailer{client: client, from: from, baseURL: baseURL}
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
	enlace := fmt.Sprintf("%s/auth/password/reset?token=%s", m.baseURL, token)
	texto := fmt.Sprintf(
		"Recibimos una solicitud para restablecer tu contrasena de Nexum.\n\n"+
			"Abre este enlace antes de %s (hora UTC):\n%s\n\n"+
			"Si no solicitaste el cambio, ignora este mensaje: tu contrasena no se modificara.\n",
		expiraEn.UTC().Format("2006-01-02 15:04"), enlace,
	)
	html := fmt.Sprintf(
		"<p>Recibimos una solicitud para restablecer tu contrase&ntilde;a de Nexum.</p>"+
			"<p><a href=\"%s\">Restablecer contrase&ntilde;a</a></p>"+
			"<p>El enlace caduca el %s (UTC) y solo puede usarse una vez.</p>"+
			"<p>Si no lo solicitaste, ignora este mensaje.</p>",
		enlace, expiraEn.UTC().Format("2006-01-02 15:04"),
	)

	out, err := m.client.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(m.from),
		Destination:      &types.Destination{ToAddresses: []string{destino}},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String("Recupera tu contrasena de Nexum")},
				Body: &types.Body{
					Text: &types.Content{Data: aws.String(texto)},
					Html: &types.Content{Data: aws.String(html)},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("ses send-email: %w", err)
	}
	return aws.ToString(out.MessageId), nil
}
