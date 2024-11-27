package handlers

import (
	ctx "woody-wood-portail/cmd/ctx/auth"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/views"

	"github.com/labstack/echo/v4"
)

func RegisterUserHandlers(e RequireAuth, model *Model, openChannel chan struct{}) {
	userRoutes := e.Group.Group("/user")

	userHandler := func(c echo.Context) error {
		return Render(c, 200, views.UserPage(len(model.Gates) > 0))
	}
	userRoutes.GET("", userHandler)
	userRoutes.GET("/", userHandler)

	userRoutes.PUT("/open", func(c echo.Context) error {
		if len(openChannel) != 0 {
			return Render(c, 200, views.OpenResult("La porte est déjà en train de s'ouvrir", true))
		}

		user := ctx.GetUserFromEcho(c)
		if _, err := db.Q(c).CreateLog(c.Request().Context(), user.ID); err != nil {
			logger.Log.Error().Err(err).Msg("Failed to create log")
			return Render(c, 422, views.OpenResult("Une erreur est survenue", false))
		}

		if err := db.Commit(c); err != nil {
			logger.Log.Error().Err(err).Msg("Failed to commit transaction")
			return Render(c, 422, views.OpenResult("Une érreur est survenue", false))
		}

		openChannel <- struct{}{}
		return Render(c, 200, views.OpenResult("La porte s'ouvre", true))
	})
}
