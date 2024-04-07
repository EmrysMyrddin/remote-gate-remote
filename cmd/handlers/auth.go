package handlers

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"woody-wood-portail/cmd/auth"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/views"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

const (
	MAX_AGE = int(time.Hour * 24 * 7 * 4) // 1 month
)

var (
	emailRegex = regexp.MustCompile(`^.*@.*\..{2,}$`)
)

func RegisterAuthHandlers(e *echo.Echo) {
	e.GET("/register", func(c echo.Context) error {
		return Render(c, 200, views.RegisterPage())
	})

	e.GET("/login", func(c echo.Context) error {
		return Render(c, 200, views.LoginPage())
	})

	e.POST("/register", func(c echo.Context) error {
		values, err := c.FormParams()
		if err != nil {
			return Render(c, 422, views.RegisterForm(nil, map[string]string{"form": "Erreur inatendue"}))
		}

		if values.Get("email") == "" {
			return Render(c, 422, views.RegisterForm(values, map[string]string{"email": "Email obligatoire"}))
		}
		if !emailRegex.MatchString(values.Get("email")) {
			return Render(c, 422, views.RegisterForm(values, map[string]string{"email": "Email invalide"}))
		}

		_, err = queries.GetUserByEmail(c.Request().Context(), values.Get("email"))
		if err == nil {
			logger.Log.Info().Err(err).Msg("Email already used")
			return Render(c, 422, views.RegisterForm(values, map[string]string{"email": "Email déjà utilisé"}))
		}
		if err != pgx.ErrNoRows {
			logger.Log.Error().Err(err).Msg("Unable to get user by email")
			return Render(c, 422, views.RegisterForm(values, map[string]string{"email": "Erreur inatendue"}))
		}

		if values.Get("password") == "" {
			return Render(c, 422, views.RegisterForm(values, map[string]string{"password": "Mot de passe obligatoire"}))
		}

		if values.Get("password") != values.Get("confirm") {
			return Render(c, 422, views.RegisterForm(values, map[string]string{"confirm": "Les mots de passes ne correspondent pas"}))
		}

		if values.Get("fullName") == "" {
			return Render(c, 422, views.RegisterForm(values, map[string]string{"fullName": "Nom complet obligatoire"}))
		}

		apartment := strings.ToUpper(values.Get("apartment"))
		if apartment == "" {
			return Render(c, 422, views.RegisterForm(values, map[string]string{"apartment": "Appartement obligatoire"}))
		}
		if (!strings.HasPrefix(apartment, "A") && !strings.HasPrefix(apartment, "B")) || len(apartment) != 4 {
			return Render(c, 422, views.RegisterForm(values, map[string]string{"apartment": "Appartement invalide. Exemple: A001"}))
		}

		createUserParams, err := auth.CreateHash(values.Get("password"))
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to hash password")
			return Render(c, 422, views.RegisterForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		createUserParams.Email = values.Get("email")
		createUserParams.FullName = values.Get("fullName")
		createUserParams.Apartment = values.Get("apartment")

		newUser, err := queries.CreateUser(c.Request().Context(), createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create user")
			return Render(c, 422, views.RegisterForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		token, err := auth.CreateToken(newUser)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create token")
			return Render(c, 422, views.LoginForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		c.SetCookie(&http.Cookie{Name: "authorization", Value: token, HttpOnly: true})

		return Redirect(c, "/user/")
	})

	e.POST("/login", func(c echo.Context) error {
		values, err := c.FormParams()
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.LoginForm(nil, map[string]string{"form": "Erreur inatendue"}))
		}

		user, err := queries.GetUserByEmail(c.Request().Context(), values.Get("email"))
		if err != nil {
			return Render(c, 422, views.LoginForm(values, map[string]string{"email": "Email invalide"}))
		}

		ok, err := auth.CompareHashAgainstPassword(user, values.Get("password"))
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to compare password")
			return Render(c, 422, views.LoginForm(values, map[string]string{"form": "Erreur inatendue"}))
		}
		if !ok {
			return Render(c, 422, views.LoginForm(values, map[string]string{"password": "Mot de passe invalide"}))
		}

		token, err := auth.CreateToken(user)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create token")
			return Render(c, 422, views.LoginForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		c.SetCookie(createCookie(token, MAX_AGE))
		return Redirect(c, "/user/")
	})

	e.GET("/logout", func(c echo.Context) error {
		c.SetCookie(createCookie("", -1))
		return Redirect(c, "/login")
	})
}

func createCookie(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     "authorization",
		Value:    token,
		HttpOnly: true,
		MaxAge:   maxAge,
	}
}
