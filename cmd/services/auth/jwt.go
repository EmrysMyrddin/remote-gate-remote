package auth

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"
	"woody-wood-portail/cmd/services/db"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
)

var (
	JWT_SECRET          = []byte(os.Getenv("JWT_SECRET"))
	ErrEmailNotVerified = errors.New("email not verified")
	ErrJWTMissing       = echojwt.ErrJWTMissing

	AuthAudience              = audience("auth")
	EmailVerificationAudience = audience("email_verification")
	ResetPasswordAudience     = audience("reset_password")
)

func init() {
	if len(JWT_SECRET) == 0 {
		log.Fatal("JWT_SECRET is not set in the environment variables")
	}
}

func CreateToken(userID uuid.UUID, audience audience) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		Subject:  userID.String(),
		Audience: jwt.ClaimStrings{string(audience)},
		IssuedAt: &jwt.NumericDate{Time: time.Now()},
	})

	return token.SignedString([]byte(JWT_SECRET))
}

func JWTMiddleware(queries *db.Queries, errorHandler func(c echo.Context, err error) error) echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		TokenLookup:            "cookie:authorization",
		ContinueOnIgnoredError: true,
		ParseTokenFunc: func(c echo.Context, tokenString string) (interface{}, error) {
			userID, _, err := ParseToken(tokenString, AuthAudience)
			if err != nil {
				return nil, err
			}

			user, err := queries.GetUser(c.Request().Context(), *userID)
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve user from jwt: %w", err)
			}

			if !user.EmailVerified {
				// Manually set the user in the context to allow using unverified users in auth handlers
				c.Set("user", user)
				return nil, ErrEmailNotVerified
			}

			return user, nil
		},
		ErrorHandler: errorHandler,
	})
}

func ParseToken(tokenString string, audience audience) (*uuid.UUID, *jwt.Token, error) {
	token, err := jwt.Parse(tokenString, getJwtKey)
	if err != nil {
		return nil, nil, err
	}
	if !token.Valid {
		return nil, nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid token")}
	}

	audienceClaim, err := token.Claims.GetAudience()
	if err != nil {
		return nil, nil, &echojwt.TokenError{Token: token, Err: errors.New("missing token audience")}
	} else if len(audienceClaim) != 1 {
		return nil, nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid token audience")}
	} else if audienceClaim[0] != string(audience) {
		return nil, nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid token audience")}
	}

	subject, err := token.Claims.GetSubject()
	if err != nil {
		return nil, nil, &echojwt.TokenError{Token: token, Err: errors.New("missing token subject")}
	}

	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid token subject uuid")}
	}
	return &userID, token, nil
}

func getJwtKey(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}

	return JWT_SECRET, nil
}

type audience string
