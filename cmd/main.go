package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"woody-wood-portail/cmd/handlers"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/views"

	"github.com/jackc/pgx/v5"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var (
	PORT = os.Getenv("PORT")
)

func init() {
	if PORT == "" {
		PORT = "80"
	}
}

func main() {
	conn, err := pgx.Connect(context.Background(), "user=postgres dbname=gate password=postgres host=localhost")
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer conn.Close(context.Background())
	handlers.SetQueries(db.New(conn))

	e := echo.New()

	e.Use(logger.LoggerMiddleware())
	e.Use(middleware.Recover())

	e.Static("/static", "static")

	e.GET("/", func(c echo.Context) error {
		return handlers.Render(c, 200, views.IndexPage())
	})

	openChannel := make(chan struct{}, 1)
	model := handlers.NewModel()

	handlers.RegisterAuthHandlers(e)
	handlers.RegisterUserHandlers(e, &model, openChannel)
	handlers.RegisterGateHandlers(e, &model, openChannel)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		if err := e.Start(":" + PORT); err != nil && err != http.ErrServerClosed {
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
