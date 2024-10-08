package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"woody-wood-portail/cmd/config"
	"woody-wood-portail/cmd/handlers"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/cmd/timezone"

	"github.com/go-playground/validator/v10"
	"github.com/robfig/cron"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	db.Migrate()

	pool, err := db.Connect()
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	// Cron task to delete old logs after 1 year. We do it every day at midnight.
	c := cron.NewWithLocation(timezone.TZ)
	err = c.AddFunc("@daily", func() {
		db.DeleteOldLogs()
	})
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to add cron job")
	}
	c.Start()

	e := echo.New()

	e.Use(logger.LoggerMiddleware())
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			logger.Log.Error().Err(err).Msg("request failed with panic\n" + string(stack))
			return err
		},
	}))
	e.Use(db.TransactionMiddleware())

	e.Static("/static", "static")

	e.GET("/", func(c echo.Context) error {
		return handlers.Redirect(c, "/login")
	})

	openChannel := make(chan struct{}, 1)
	model := handlers.NewModel()

	handlers.RegisterAuthHandlers(e)
	handlers.RegisterGateHandlers(e, &model, openChannel)

	requireAuth := handlers.RequireAuthGroup(e)
	handlers.RegisterUserHandlers(requireAuth, &model, openChannel)
	handlers.RegisterAdminHandlers(requireAuth)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		if err := e.Start(":" + config.Config.Http.Port); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal().Err(err).Msg("HTTP server crashed")
		}
	}()

	<-sigCtx.Done()
	logger.Log.Info().Msg("Shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Log.Fatal().Err(err).Msg("Failed to gracefully shutdown")
	}
	logger.Log.Info().Msg("Server stopped")
}

type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}
