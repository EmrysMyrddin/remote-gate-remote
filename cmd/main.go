package main

import (
	"html/template"
	"io"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Model struct {
	Gates chan struct{}
}

func main() {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.Renderer = newTemplate()

	openChannel := make(chan struct{}, 1)
	model := newModel()

	e.GET("/", func(c echo.Context) error {
		return c.Render(200, "index", model)
	})

	e.GET("/open", func(c echo.Context) error {
		if len(openChannel) == 0 {
			openChannel <- struct{}{}
			return c.String(200, "Opening the gate")
		}

		return c.String(200, "The gate is already opening")
	})

	e.GET("/gate", func(c echo.Context) error {
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
	})

	port, ok := os.LookupEnv("PORT")
	if !ok {
		port = "80"
	}
	e.Logger.Fatal(e.Start(":" + port))
}

type Templates struct {
	templates *template.Template
}

func (t *Templates) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func newTemplate() *Templates {
	println(template.ParseGlob("views/*.html"))
	return &Templates{
		templates: template.Must(template.ParseGlob("views/*.html")),
	}
}

func newModel() Model {
	return Model{
		Gates: make(chan struct{}, 10),
	}
}

func (model Model) gateConnected() {
	model.Gates <- struct{}{}
}

func (model Model) gateDisconnected() {
	<-model.Gates
}
