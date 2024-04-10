package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	"github.com/google/uuid"
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

		createUserParams := db.CreateUserParams{
			Email:     values.Get("email"),
			FullName:  values.Get("fullName"),
			Apartment: apartment,
		}

		err = auth.CreateHash(values.Get("password"), &createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to hash password")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		// TODO: use a transaction
		newUser, err := queries.CreateUser(c.Request().Context(), createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create user")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		if err := addAuthenticationCookie(c, newUser.ID); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to add authentication cookie")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.LoginForm(model))
		}

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

		if err = queries.EmailVerified(c.Request().Context(), *userID); err != nil {
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

		if err := addAuthenticationCookie(c, user.ID); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to add authentication cookie")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.LoginForm(model))
		}

		redirect := c.QueryParam("redirect")
		if redirect != "" {
			Redirect(c, redirect)
		}
		return RedirectWitQuery(c, "/user/")
	})

	authGroup.GET("/password-forgotten", func(c echo.Context) error {
		resetError := c.QueryParam("error")
		logger.Log.Info().Str("error", resetError).Msg("Password forgotten error")
		if resetError != "" {
			logger.Log.Error().Str("error", resetError).Msg("Unable to reset password")
			return Render(c, 422, views.PasswordForgottenPage(resetError))
		}
		return Render(c, 200, views.PasswordForgottenPage(""))
	})

	authGroup.POST("/password-forgotten", func(c echo.Context) error {
		values, err := c.FormParams()
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.PasswordForgottenForm(views.PasswordForgottenModel{
				FormModel: views.FormModel{Errors: views.Errors{"form": "Erreur inatendue"}},
			}))
		}
		model := views.PasswordForgottenModel{
			FormModel: views.FormModel{
				Values: values,
				Errors: views.Errors{},
			},
		}

		user, err := queries.GetUserByEmail(c.Request().Context(), values.Get("email"))
		if err != nil {
			model.Errors["email"] = "Email invalide"
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		resetToken, err := auth.CreateToken(user.ID, auth.ResetPasswordAudience)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create reset token")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		err = mails.SendMail(c,
			user,
			"Réinitialisation de votre mot de passe Woody Wood Gate",
			emails.PasswordReset(user, templ.SafeURL(fmt.Sprintf("%s/reset-password?code=%s", BASE_URL, resetToken))),
		)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send password reset email")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		return Render(c, 200, views.PasswordForgottenForm(views.PasswordForgottenModel{EmailSent: true}))
	})

	authGroup.GET("/reset-password", func(c echo.Context) error {
		code := c.QueryParam("code")
		if code == "" {
			return RedirectWitQuery(c, "/password-forgotten")
		}

		userID, _, err := auth.ParseToken(code, auth.ResetPasswordAudience)
		if err != nil {
			logger.Log.Error().Str("code", code).Err(err).Msg("Unable to reset password")
			return Redirect(c, "/password-forgotten?error="+url.QueryEscape("Code de réinitialisation invalide"))
		}

		_, err = queries.GetUser(c.Request().Context(), *userID)
		if err != nil {
			logger.Log.Error().Str("code", code).Err(err).Msg("Unable to reset password")
			return Redirect(c, "/password-forgotten?error="+url.QueryEscape("Code de réinitialisation invalide"))
		}

		return Render(c, 200, views.ResetPasswordPage())
	})

	authGroup.POST("/reset-password", func(c echo.Context) error {
		code := c.QueryParam("code")
		if code == "" {
			return RedirectWitQuery(c, "/password-forgotten")
		}

		userID, _, err := auth.ParseToken(code, auth.ResetPasswordAudience)
		if err != nil {
			logger.Log.Error().Str("code", code).Err(err).Msg("Unable to reset password")
			return Redirect(c, "/password-forgotten?error="+url.QueryEscape("Code de réinitialisation invalide"))
		}

		_, err = queries.GetUser(c.Request().Context(), *userID)
		if err != nil {
			logger.Log.Error().Str("code", code).Err(err).Msg("Unable to reset password")
			return Redirect(c, "/password-forgotten?error="+url.QueryEscape("Code de réinitialisation invalide"))
		}

		values, err := c.FormParams()
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.ResetPasswordForm(views.FormModel{Errors: views.Errors{"form": "Erreur inatendue"}}))
		}

		model := views.FormModel{
			Values: values,
			Errors: views.Errors{},
		}

		if values.Get("password") != values.Get("confirm") {
			model.Errors["confirm"] = "Les mots de passes ne correspondent pas"
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		if values.Get("password") == "" {
			model.Errors["password"] = "Mot de passe obligatoire"
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		updatePassword := db.UpdatePasswordParams{
			ID: *userID,
		}

		if err := auth.CreateHash(c.FormValue("password"), &updatePassword); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to hash password")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		if err := queries.UpdatePassword(c.Request().Context(), updatePassword); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to update password")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		if err := addAuthenticationCookie(c, *userID); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to add authentication cookie")
			model.Errors["form"] = "Erreur inatendue"
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		return Redirect(c, "/user/")
	})

	e.GET("/logout", func(c echo.Context) error {
		c.SetCookie(createCookie("", -1))
		return RedirectWitQuery(c, "/login")
	})
}

func addAuthenticationCookie(c echo.Context, userID uuid.UUID) error {
	token, err := auth.CreateToken(userID, auth.AuthAudience)
	if err != nil {
		return fmt.Errorf("unable to create authentication token: %w", err)
	}

	c.SetCookie(createCookie(token, MAX_AGE))
	return nil
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
	mailVerifToken, err := auth.CreateToken(user.ID, auth.EmailVerificationAudience)
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
