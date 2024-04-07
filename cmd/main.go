package main

import (
	"context"
	"log"
	"os"
	"woody-wood-portail/cmd/db"
	"woody-wood-portail/cmd/handlers"
	"woody-wood-portail/cmd/logger"
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

	e.Logger.Fatal(e.Start(":" + PORT))
}
