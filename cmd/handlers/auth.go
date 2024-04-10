package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/auth"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/cmd/services/mails"
	"woody-wood-portail/views"
	"woody-wood-portail/views/emails"

	"github.com/a-h/templ"
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
	authGroup := e.Group("")

	authGroup.Use(auth.JWTMiddleware(queries, func(c echo.Context, err error) error {
		if !errors.Is(err, auth.ErrJWTMissing) && !errors.Is(errors.Unwrap(err), auth.ErrEmailNotVerified) {
			RedirectWitQuery(c, "/logout")
		}
		return nil
	}))

	authGroup.GET("/register", func(c echo.Context) error {
		if ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/user/")
		}
		return Render(c, 200, views.RegisterPage())
	})

	authGroup.GET("/login", func(c echo.Context) error {
		if ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/user/")
		}
		return Render(c, 200, views.LoginPage())
	})

	authGroup.POST("/register", func(c echo.Context) error {
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

		// TODO: use a transaction
		newUser, err := queries.CreateUser(c.Request().Context(), createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create user")
			return Render(c, 422, views.RegisterForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		token, err := auth.CreateToken(newUser, auth.AuthAudience)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create token")
			return Render(c, 422, views.LoginForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		c.SetCookie(&http.Cookie{Name: "authorization", Value: token, HttpOnly: true})

		err = sendVerificationMail(c, newUser)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send verification email")
			return Render(c, 422, views.RegisterForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		return Redirect(c, "/verify")
	})

	authGroup.GET("/verify", func(c echo.Context) error {
		if !ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/login")
		}
		currentUser := ctx.GetUserFromEcho(c)

		verificationToken := c.QueryParam("code")
		if verificationToken == "" {
			if currentUser.EmailVerified {
				return Redirect(c, "/user/")
			}
			return Render(c, 200, views.VerifyPage(nil))
		}

		userID, token, err := auth.ParseToken(verificationToken, auth.EmailVerificationAudience)
		if err != nil {
			logger.Log.Error().Str("code", verificationToken).Err(err).Msg("Unable to verify user email")
			return Render(c, 200, views.VerifyPage(err))
		} else if *userID != currentUser.ID {
			logger.Log.Error().
				Str("code", verificationToken).
				Stringer("current_user", currentUser.ID).
				Stringer("userID", userID).
				Msg("User ID does not match during email verification")
			return RedirectWitQuery(c, "/logout")
		}

		if currentUser.EmailVerified {
			return Redirect(c, "/user/")
		}

		issuedAt, err := token.Claims.GetIssuedAt()
		if err != nil {
			logger.Log.Error().Str("code", verificationToken).Err(err).Msg("Unable to verify user email")
			return Render(c, 200, views.VerifyPage(err))
		} else if currentUser.UpdatedAt.Time.Add(-2 * time.Second).After(issuedAt.Time) {
			err = errors.New("token expired")
			logger.Log.Error().Str("code", verificationToken).Err(err).Msg("Unable to verify user email")
			return Render(c, 200, views.VerifyPage(err))
		}

		if _, err = queries.EmailVerified(c.Request().Context(), *userID); err != nil {
			logger.Log.Error().Str("code", verificationToken).Err(err).Msg("Unable to verify user email")
			return Render(c, 200, views.VerifyPage(err))
		}

		return Redirect(c, "/user/")
	})

	authGroup.POST("/reset-verification", func(c echo.Context) error {
		if !ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/login")
		}
		currentUser := ctx.GetUserFromEcho(c)

		if currentUser.EmailVerified {
			return RedirectWitQuery(c, "/user/")
		}

		err := sendVerificationMail(c, currentUser)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send verification email")
			return Render(c, 422, views.VerifyForm(err, ""))
		}

		<-time.After(2 * time.Second)

		return Render(c, 200, views.VerifyForm(nil, "Un nouveau lien de vérification vous a été envoyé"))
	})

	authGroup.POST("/login", func(c echo.Context) error {
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

		token, err := auth.CreateToken(user, auth.AuthAudience)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create token")
			return Render(c, 422, views.LoginForm(values, map[string]string{"form": "Erreur inatendue"}))
		}

		c.SetCookie(createCookie(token, MAX_AGE))

		redirect := c.QueryParam("redirect")
		if redirect != "" {
			Redirect(c, redirect)
		}
		return RedirectWitQuery(c, "/user/")
	})

	e.GET("/logout", func(c echo.Context) error {
		c.SetCookie(createCookie("", -1))
		return RedirectWitQuery(c, "/login")
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

func sendVerificationMail(c echo.Context, user db.User) error {
	mailVerifToken, err := auth.CreateToken(user, auth.EmailVerificationAudience)
	if err != nil {
		return fmt.Errorf("unable to create email verification token: %w", err)
	}

	err = mails.SendMail(c,
		user,
		"Votre lien de vérification de compte Woody Wood Gate",
		emails.EmailVerification(user, templ.SafeURL(fmt.Sprintf("%s/verify?code=%s", BASE_URL, mailVerifToken))),
	)
	if err != nil {
		return fmt.Errorf("unable to send verification email: %w", err)
	}

	return nil
}
