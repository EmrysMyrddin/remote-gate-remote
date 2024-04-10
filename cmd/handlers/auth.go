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
			return Render(c, 422, views.RegisterForm(views.FormModel{
				Errors: views.Errors{"form": "Erreur inatendue"},
			}))
		}
		model := views.FormModel{
			Values: values,
			Errors: views.Errors{},
		}

		if values.Get("email") == "" {
			model.Errors["email"] = "Email obligatoire"
			return Render(c, 422, views.RegisterForm(model))
		}
		if !emailRegex.MatchString(values.Get("email")) {
			model.Errors["email"] = "Email invalide"
			return Render(c, 422, views.RegisterForm(model))
		}

		_, err = queries.GetUserByEmail(c.Request().Context(), values.Get("email"))
		if err == nil {
			logger.Log.Info().Err(err).Msg("Email already used")
			model.Errors["email"] = "Email déjà utilisé"
			return Render(c, 422, views.RegisterForm(model))
		}
		if err != pgx.ErrNoRows {
			logger.Log.Error().Err(err).Msg("Unable to get user by email")
			model.Errors["email"] = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		if values.Get("password") == "" {
			model.Errors["password"] = "Mot de passe obligatoire"
			return Render(c, 422, views.RegisterForm(model))
		}

		if values.Get("password") != values.Get("confirm") {
			model.Errors["confirm"] = "Les mots de passes ne correspondent pas"
			return Render(c, 422, views.RegisterForm(model))
		}

		if values.Get("fullName") == "" {
			model.Errors["fullName"] = "Nom complet obligatoire"
			return Render(c, 422, views.RegisterForm(model))
		}

		apartment := strings.ToUpper(values.Get("apartment"))
		if apartment == "" {
			model.Errors["apartment"] = "Appartement obligatoire"
			return Render(c, 422, views.RegisterForm(model))
		}
		if (!strings.HasPrefix(apartment, "A") && !strings.HasPrefix(apartment, "B")) || len(apartment) != 4 {
			model.Errors["apartment"] = "Appartement invalide. Exemple: A001"
			return Render(c, 422, views.RegisterForm(model))
		}

		createUserParams, err := auth.CreateHash(values.Get("password"))
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to hash password")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		createUserParams.Email = values.Get("email")
		createUserParams.FullName = values.Get("fullName")
		createUserParams.Apartment = values.Get("apartment")

		// TODO: use a transaction
		newUser, err := queries.CreateUser(c.Request().Context(), createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create user")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		token, err := auth.CreateToken(newUser, auth.AuthAudience)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create token")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.LoginForm(model))
		}

		c.SetCookie(&http.Cookie{Name: "authorization", Value: token, HttpOnly: true})

		err = sendVerificationMail(c, newUser)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send verification email")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
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
			return Render(c, 422, views.VerifyForm(views.VerifyModel{Err: err}))
		}

		<-time.After(2 * time.Second)

		return Render(c, 200, views.VerifyForm(views.VerifyModel{EmailSent: true}))
	})

	authGroup.POST("/login", func(c echo.Context) error {
		values, err := c.FormParams()
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.LoginForm(views.FormModel{Errors: views.Errors{"form": "Erreur inatendue"}}))
		}
		model := views.FormModel{
			Values: values,
			Errors: views.Errors{},
		}

		user, err := queries.GetUserByEmail(c.Request().Context(), values.Get("email"))
		if err != nil {
			model.Errors["email"] = "Email invalide"
			return Render(c, 422, views.LoginForm(model))
		}

		ok, err := auth.CompareHashAgainstPassword(user, values.Get("password"))
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to compare password")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.LoginForm(model))
		}
		if !ok {
			model.Errors["password"] = "Mot de passe invalide"
			return Render(c, 422, views.LoginForm(model))
		}

		token, err := auth.CreateToken(user, auth.AuthAudience)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create token")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.LoginForm(model))
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
