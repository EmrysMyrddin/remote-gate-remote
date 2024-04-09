package handlers

import (
	"errors"
	"net/url"
	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/cmd/services/auth"
	"woody-wood-portail/views"

	"github.com/labstack/echo/v4"
)

func RegisterUserHandlers(e *echo.Echo, model *Model, openChannel chan struct{}) {
	userRoutes := e.Group("/user")
	userRoutes.Use(auth.JWTMiddleware(queries, func(c echo.Context, err error) error {
		if errors.Is(err, auth.ErrEmailNotVerified) {
			RedirectWitQuery(c, "/verify")
		} else if errors.Is(err, auth.ErrJWTMissing) {
			RedirectWitQuery(c, "/login?redirect="+url.QueryEscape(c.Path()))
		} else {
			Redirect(c, "/logout")
		}
		return err
	}))

	userHandler := func(c echo.Context) error {
		return Render(c, 200, views.UserPage(len(model.Gates) > 0))
	}
	userRoutes.GET("", userHandler)
	userRoutes.GET("/", userHandler)

	userRoutes.GET("/open", func(c echo.Context) error {
		user := ctx.GetUserFromEcho(c)
		queries.CreateLog(c.Request().Context(), user.ID)
		if len(openChannel) == 0 {
			openChannel <- struct{}{}
			return c.String(200, "La porte s'ouvre")
		}

		return c.String(200, "La porte est déjà en train de s'ouvrir")
	})
}
