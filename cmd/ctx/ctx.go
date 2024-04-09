package ctx

import (
	"context"
	"log"
	"woody-wood-portail/cmd/services/db"

	"github.com/labstack/echo/v4"
)

type userContextKeyType string

var userContextKey userContextKeyType = "user"

func GetUserFromEcho(c echo.Context) db.User {
	user, ok := c.Get("user").(db.User)
	if !ok {
		log.Fatal("User not found in context")
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
		log.Fatal("User not found in context")
	}
	return user

}

func EchoToTemplContext(c echo.Context) context.Context {
	user, ok := c.Get("user").(db.User)
	if !ok {
		return c.Request().Context()
	}
	return context.WithValue(context.Background(), userContextKey, user)
}
