package mails

import (
	"fmt"
	"os"
	"strings"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/mailjet/mailjet-apiv3-go/v4"
)

const (
	API_KEY    = "a2cc6295a4a83aee7c5065585c65dc88"
	SECRET_KEY = "2233d2c971a933f6a51bb2cd638076a9"
)

var (
	mailjetClient = mailjet.NewMailjetClient(API_KEY, SECRET_KEY)
	SENDER        = &mailjet.RecipientV31{
		Email: os.Getenv("MAIL_SENDER_ADDRESS"),
		Name:  os.Getenv("MAIL_SENDER_NAME"),
	}
)

func init() {
	if SENDER.Email == "" {
		SENDER.Email = "woody-wood-gate@cocaud.dev"
	}
	if SENDER.Name == "" {
		SENDER.Name = "Woody Wood Gate"
	}
}

func SendMail(c echo.Context, recipient db.User, subject string, body templ.Component) error {
	renderedBody := &strings.Builder{}
	if err := body.Render(c.Request().Context(), renderedBody); err != nil {
		return fmt.Errorf("failed to render email: %w", err)
	}

	res, err := mailjetClient.SendMailV31(&mailjet.MessagesV31{
		Info: []mailjet.InfoMessagesV31{
			{
				From: SENDER,
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
