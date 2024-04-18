package handlers

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/views"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/skip2/go-qrcode"
)

func RegisterAdminHandlers(e RequireAuth) {
	adminGroup := e.Group.Group("/admin")
	adminGroup.Use(RequireAdminRoleMiddleware)

	adminGroup.GET("", adminPageHandler)
	adminGroup.GET("/", adminPageHandler)

	adminGroup.POST("", adminFormHandler)
	adminGroup.POST("/", adminFormHandler)
}

func adminPageHandler(c echo.Context) error {
	var err error
	model := views.AdminFormModel{}
	model.Code, err = queries.GetRegistrationCode(c.Request().Context())

	if err != nil && err != pgx.ErrNoRows {
		model.Err = err.Error()
		return Render(c, 200, views.AdminPage(model))
	}

	model.QrCode, err = invitationQrCodeHandler(model.Code)
	if err != nil {
		model.Err = err.Error()
	}

	return Render(c, 200, views.AdminPage(model))
}

func adminFormHandler(c echo.Context) error {
	var err error
	model := views.AdminFormModel{}
	model.Code, err = queries.GetRegistrationCode(c.Request().Context())
	if err != nil && err != pgx.ErrNoRows {
		model.Err = err.Error()
		return Render(c, 422, views.AdminForm(model))
	}

	newCode := fmt.Sprintf("%06d", rand.Int31n(899_999)+100_000)

	if err = queries.SetRegistrationCode(c.Request().Context(), newCode); err != nil {
		logger.Log.Error().Err(err).Msg("Failed to set registration code")
		model.Err = err.Error()
		return Render(c, 422, views.AdminForm(model))
	}

	model.Code = newCode
	model.QrCode, err = invitationQrCodeHandler(newCode)
	if err != nil {
		model.Err = err.Error()
	}
	return Render(c, 200, views.AdminForm(model))
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
