package auth

import (
	"context"
	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/cmd/services/db"

	"github.com/labstack/echo/v4"
)

type userContextKeyType string

var userContextKey userContextKeyType = "user"

func GetUserFromEcho(c echo.Context) db.User {
	user, ok := c.Get("user").(db.User)
	if !ok {
		panic("User not found in context")
	}
	return user
}

func IsAuthenticated(c echo.Context) bool {
	_, ok := c.Get("user").(db.User)
	return ok
}

func GetUserFromTempl(c context.Context) db.User {
	user, ok := c.Value(userContextKey).(db.User)
	if !ok {
		panic("User not found in context")
	}
	return user

}

func EchoToTemplContext(c echo.Context) context.Context {
	templCtx := c.Request().Context()
	templCtx = ctx.WithEchoContext(templCtx, c)
	if user, ok := c.Get("user").(db.User); ok {
		templCtx = context.WithValue(templCtx, userContextKey, user)
	}
	return templCtx
}
