package handlers

import (
	"os"
	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/cmd/db"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

var (
	GATE_SECRET = os.Getenv("GATE_SECRET")
)

var queries *db.Queries

func Render(c echo.Context, statusCode int, t templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
	c.Response().Writer.WriteHeader(statusCode)
	return t.Render(ctx.EchoToTemplContext(c), c.Response().Writer)
}

func Redirect(c echo.Context, url string) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", url)
		return c.NoContent(204)
	}
	return c.Redirect(302, url)
}

type Model struct {
	Gates chan struct{}
}

func NewModel() Model {
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

func SetQueries(q *db.Queries) {
	queries = q
}

func init() {
	if GATE_SECRET == "" {
		GATE_SECRET = "dev_gate_secret"
	}
}
