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
			user, err := ParseToken(queries, c, tokenString, WithAudience(AuthAudience))
			if err != nil {
				return nil, err
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

func ParseToken(queries *db.Queries, c echo.Context, tokenString string, rules ...TokenRule) (*db.User, error) {
	token, err := jwt.Parse(tokenString, getJwtKey)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid token")}
	}

	subject, err := token.Claims.GetSubject()
	if err != nil {
		return nil, &echojwt.TokenError{Token: token, Err: errors.New("missing token subject")}
	}

	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, &echojwt.TokenError{Token: token, Err: errors.New("invalid token subject uuid")}
	}

	user, err := queries.GetUser(c.Request().Context(), userID)
	if err != nil {
		return nil, &echojwt.TokenError{Token: token, Err: errors.New("user not found")}
	}

	for _, rule := range rules {
		if err := rule(&user, token); err != nil {
			return nil, &echojwt.TokenError{Token: token, Err: err}
		}
	}
	return &user, nil
}

func getJwtKey(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}

	return JWT_SECRET, nil
}

type audience string

type TokenRule func(user *db.User, token *jwt.Token) error

func WithAudience(audience audience) TokenRule {
	return func(user *db.User, token *jwt.Token) error {
		audienceClaim, err := token.Claims.GetAudience()
		if err != nil {
			return &echojwt.TokenError{Token: token, Err: errors.New("missing token audience")}
		} else if len(audienceClaim) != 1 {
			return &echojwt.TokenError{Token: token, Err: errors.New("invalid token audience")}
		} else if audienceClaim[0] != string(audience) {
			return &echojwt.TokenError{Token: token, Err: errors.New("invalid token audience")}
		}

		return nil
	}
}

func IssuedAfterLastUserUpdate(allowedOverlapping time.Duration) TokenRule {
	return func(user *db.User, token *jwt.Token) error {
		issuedAt, err := token.Claims.GetIssuedAt()
		if err != nil {
			return &echojwt.TokenError{Token: token, Err: errors.New("missing token issued at")}
		}

		if issuedAt.Time.Before(user.UpdatedAt.Time.Add(-allowedOverlapping)) {
			return &echojwt.TokenError{Token: token, Err: errors.New("missing token issued at")}
		}

		return nil
	}
}
