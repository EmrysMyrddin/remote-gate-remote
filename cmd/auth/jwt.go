package auth

import (
	"errors"
	"fmt"
	"log"
	"os"
	"woody-wood-portail/cmd/db"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
)

var (
	JWT_SECRET = []byte(os.Getenv("JWT_SECRET"))
)

func init() {
	if len(JWT_SECRET) == 0 {
		log.Fatal("JWT_SECRET is not set in the environment variables")
	}
}

func CreateToken(user db.User) (string, error) {
	userIDValue, err := user.ID.Value()
	if err != nil {
		return "", fmt.Errorf("unable to get user id: %w", err)
	}

	userID, ok := userIDValue.(string)
	if !ok {
		return "", errors.New("invalid user id")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		Subject: userID,
	})

	return token.SignedString([]byte(JWT_SECRET))
}

func JWTMiddleware(queries *db.Queries, errorHandler func(c echo.Context, err error) error) echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		TokenLookup: "cookie:authorization",
		ParseTokenFunc: func(c echo.Context, tokenString string) (interface{}, error) {
			token, err := jwt.Parse(tokenString, getJwtKey)
			if err != nil {
				return nil, &echojwt.TokenError{Token: token, Err: err}
			}
			if !token.Valid {
				return nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid token")}
			}
			userID := &pgtype.UUID{}
			subject, err := token.Claims.GetSubject()
			if err != nil {
				return nil, &echojwt.TokenError{Token: token, Err: errors.New("missing token subject")}
			}
			if err := userID.Scan(subject); err != nil {
				return nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid user id")}
			}
			user, err := queries.GetUser(c.Request().Context(), *userID)
			if err != nil {
				return nil, &echojwt.TokenError{Token: token, Err: fmt.Errorf("user not found: %w", err)}
			}
			return user, nil
		},
		ErrorHandler: errorHandler,
	})
}

func getJwtKey(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}

	return JWT_SECRET, nil
}
