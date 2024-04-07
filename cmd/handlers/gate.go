package handlers

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func RegisterGateHandlers(e *echo.Echo, model *Model, openChannel chan struct{}) {

	gateRoutes := e.Group("/gate")
	gateRoutes.Use(middleware.KeyAuth(func(key string, c echo.Context) (bool, error) {
		return key == GATE_SECRET, nil
	}))

	gateHandler := func(c echo.Context) error {
		model.gateConnected()
		defer model.gateDisconnected()

		select {
		case <-openChannel:
			return c.Render(200, "reloader", nil)
		case <-time.After(30 * time.Second):
			return c.Render(408, "reloader", nil)
		case <-c.Request().Context().Done():
			return nil
		}
	}

	gateRoutes.GET("", gateHandler)
	gateRoutes.GET("/", gateHandler)
}
