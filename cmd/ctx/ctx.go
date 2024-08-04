package ctx

import (
	"context"

	"github.com/labstack/echo/v4"
)

type echoContextKeyType string

var echoContextKey echoContextKeyType = "echo"

func WithEchoContext(c context.Context, ec echo.Context) context.Context {
	return context.WithValue(c, echoContextKey, ec)
}

func GetEchoFromTempl(c context.Context) echo.Context {
	ec, ok := c.Value(echoContextKey).(echo.Context)
	if !ok {
		panic("Echo context not found in context")
	}
	return ec
}
