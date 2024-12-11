package mails

import (
	"context"
	"fmt"
	"strings"
	"woody-wood-portail/cmd/config"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"

	"github.com/a-h/templ"
	"github.com/mailjet/mailjet-apiv3-go/v4"
)

var (
	mailjetClient = mailjet.NewMailjetClient(config.Config.Mail.APIKey, config.Config.Mail.SecretKey)
)

func SendMail(c context.Context, recipient db.User, subject string, body templ.Component) error {
	renderedBody := &strings.Builder{}
	if err := body.Render(c, renderedBody); err != nil {
		return fmt.Errorf("failed to render email: %w", err)
	}

	res, err := mailjetClient.SendMailV31(&mailjet.MessagesV31{
		Info: []mailjet.InfoMessagesV31{
			{
				From: &config.Config.Mail.Sender,
				To: &mailjet.RecipientsV31{
					mailjet.RecipientV31{
						Email: recipient.Email,
						Name:  recipient.FullName,
					},
				},
				Subject:  subject,
				HTMLPart: renderedBody.String(),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	logger.Log.Info().
		Str("mailjet_msg_href", res.ResultsV31[0].To[0].MessageHref).
		Str("subject", subject).
		Str("recipient", recipient.Email).
		Msg("Email sent")

	return nil
}
