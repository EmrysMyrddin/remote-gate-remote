package handlers

import (
	"woody-wood-portail/cmd/auth"
	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/views"

	"github.com/labstack/echo/v4"
)

func RegisterUserHandlers(e *echo.Echo, model *Model, openChannel chan struct{}) {
	userRoutes := e.Group("/user")
	userRoutes.Use(auth.JWTMiddleware(queries, func(c echo.Context, err error) error {
		Redirect(c, "/login")
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
