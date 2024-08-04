package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	ctx "woody-wood-portail/cmd/ctx/auth"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/cmd/services/mails"
	"woody-wood-portail/views"
	"woody-wood-portail/views/emails"

	"github.com/a-h/templ"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/skip2/go-qrcode"
)

func RegisterAdminHandlers(e RequireAuth) {
	adminGroup := e.Group.Group("/admin")
	adminGroup.Use(RequireAdminRoleMiddleware)

	adminGroup.GET("", func(c echo.Context) error {
		return Redirect(c, "/admin/users")
	})
	adminGroup.GET("/", func(c echo.Context) error {
		return Redirect(c, "/admin/users")
	})

	adminGroup.GET("/invitation", adminInvitationPageHandler)
	adminGroup.POST("/invitation", adminInvitationFormHandler)

	adminGroup.GET("/users", adminUsersPage)
	adminGroup.PUT("/registrations/:id/:action", adminRegistrationFormHandler)
}

func adminUsersPage(c echo.Context) error {
	users, err := db.Q(c).ListUsers(c.Request().Context())
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	model := &views.AdminUsersPageModel{
		PendingRegistrations: make([]db.User, 0, len(users)),
		Users:                make([]db.User, 0, len(users)),
	}

	for _, user := range users {
		if user.RegistrationState == "accepted" {
			model.Users = append(model.Users, user)
		} else {
			model.PendingRegistrations = append(model.PendingRegistrations, user)
		}
	}

	return Render(c, 200, views.AdminUsersPage(model))
}

func adminRegistrationFormHandler(c echo.Context) error {
	model := &views.AdminUserRowModel{}

	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		model.Err = errors.New("id d'utilisateur invalide")
		return Render(c, 422, views.AdminRegistrationRow(model))
	}

	action := c.Param("action")

	switch action {
	case "accept":
		model.User, err = db.Q(c).RegistrationAccepted(c.Request().Context(), userID)
		if err != nil {
			model.Err = errors.New("utilisateur introuvable")
			logger.Log.Error().Err(err).Msg("Failed to accept registration")
			return Render(c, 422, views.AdminRegistrationRow(model))
		}

		if err := sendRegistrationAcceptedMail(c, model.User); err != nil {
			model.Err = errors.New("échec de l'envoi de l'e-mail")
			return Render(c, 422, views.AdminRegistrationRow(model))
		}

		if err := db.Commit(c); err != nil {
			model.Err = errors.New("échec de l'enregistrement")
			logger.Log.Error().Err(err).Msg("Failed to commit transaction")
			return Render(c, 422, views.AdminRegistrationRow(model))
		}

		model.Attrs = templ.Attributes{"hx-swap-oob": "beforeend:#users-list"}
		return Render(c, 200, views.AdminUserRow(model))
	case "reject":
		model.User, err = db.Q(c).RegistrationRejected(c.Request().Context(), userID)
		if err != nil {
			model.Err = errors.New("utilisateur introuvable")
			logger.Log.Error().Err(err).Msg("Failed to reject registration")
			return Render(c, 422, views.AdminRegistrationRow(model))
		}

		if err := sendRegistrationRejectedMail(c, model.User); err != nil {
			model.Err = errors.New("échec de l'envoi de l'e-mail")
			return Render(c, 422, views.AdminRegistrationRow(model))
		}

		if err := db.Commit(c); err != nil {
			model.Err = errors.New("échec de l'enregistrement")
			logger.Log.Error().Err(err).Msg("Failed to commit transaction")
			return Render(c, 422, views.AdminRegistrationRow(model))
		}

		return Render(c, 200, views.AdminRegistrationRow(model))
	default:
		model.Err = errors.New("action inconnue")
		logger.Log.Error().Str("action", action).Msg("Unknown action")
		return Render(c, 422, views.AdminRegistrationRow(model))
	}
}

func adminInvitationPageHandler(c echo.Context) error {
	var err error
	model := &views.AdminInvitationFormModel{}
	model.Code, err = db.Q(c).GetRegistrationCode(c.Request().Context())

	if err != nil && err != pgx.ErrNoRows {
		model.Err = err.Error()
		return Render(c, 200, views.AdminInvitationPage(model))
	}

	model.QrCode, err = invitationQrCodeHandler(model.Code)
	if err != nil {
		model.Err = err.Error()
	}

	return Render(c, 200, views.AdminInvitationPage(model))
}

func adminInvitationFormHandler(c echo.Context) error {
	var err error
	model := &views.AdminInvitationFormModel{}
	model.Code, err = db.Q(c).GetRegistrationCode(c.Request().Context())
	if err != nil && err != pgx.ErrNoRows {
		model.Err = err.Error()
		return Render(c, 422, views.AdminInvitationForm(model))
	}

	newCode := fmt.Sprintf("%06d", rand.Int31n(899_999)+100_000)

	if err = db.Q(c).SetRegistrationCode(c.Request().Context(), newCode); err != nil {
		logger.Log.Error().Err(err).Msg("Failed to set registration code")
		model.Err = err.Error()
		return Render(c, 422, views.AdminInvitationForm(model))
	}

	model.Code = newCode
	model.QrCode, err = invitationQrCodeHandler(newCode)
	if err != nil {
		logger.Log.Error().Err(err).Msg("Failed to render QR code")
		model.Err = err.Error()
		return Render(c, 422, views.AdminInvitationForm(model))
	}

	if err := db.Commit(c); err != nil {
		logger.Log.Error().Err(err).Msg("Failed to commit transaction")
		model.Err = err.Error()
		return Render(c, 422, views.AdminInvitationForm(model))
	}

	return Render(c, 200, views.AdminInvitationForm(model))
}

func invitationQrCodeHandler(code string) (string, error) {
	qrPNG, err := qrcode.Encode(BASE_URL+"/register?code="+code, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG), nil
}

func RequireAdminRoleMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := ctx.GetUserFromEcho(c)
		if user.Role != "admin" {
			logger.Log.Debug().Stringer("user", user.ID).Str("role", user.Role).Msg("unauthorized access to admin page")
			return Redirect(c, "/user")
		}
		return next(c)
	}
}

func sendRegistrationAcceptedMail(c echo.Context, user db.User) error {
	if err := mails.SendMail(c,
		user,
		"Inscription sur Woody Wood Gate acceptée",
		emails.RegistrationRequestAccepted(user),
	); err != nil {
		logger.Log.Error().Err(err).Msg("Unable to send accepted registration email")
	}

	return nil
}

func sendRegistrationRejectedMail(c echo.Context, user db.User) error {
	if err := mails.SendMail(c,
		user,
		"Inscription sur Woody Wood Gate refusée",
		emails.RegistrationRequestRejected(user),
	); err != nil {
		logger.Log.Error().Err(err).Msg("Unable to send rejected registration email")
	}

	return nil
}
