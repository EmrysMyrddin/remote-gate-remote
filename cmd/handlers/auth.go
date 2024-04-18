package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"
	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/auth"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/cmd/services/mails"
	"woody-wood-portail/views"
	"woody-wood-portail/views/emails"

	"github.com/a-h/templ"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

const (
	MAX_AGE = int(time.Hour * 24 * 7 * 4) // 1 month
)

type RequireAuth struct {
	*echo.Group
}

func RequireAuthGroup(e *echo.Echo) RequireAuth {
	requireAuth := e.Group("")
	requireAuth.Use(auth.JWTMiddleware(queries, func(c echo.Context, err error) error {
		if errors.Is(err, auth.ErrEmailNotVerified) {
			logger.Log.Debug().Err(err).Msg("not verified, redirecting to /verify")
			RedirectWitQuery(c, "/verify")
		} else if errors.Is(err, auth.ErrJWTMissing) {
			logger.Log.Debug().Err(err).Msg("not logged in, redirecting to /login")
			RedirectWitQuery(c, "/login?redirect="+url.QueryEscape(c.Path()))
		} else {
			logger.Log.Debug().Err(err).Msg("invalid JWT, logging out")
			Redirect(c, "/logout")
		}
		return err
	}))

	return RequireAuth{requireAuth}
}

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
		return Render(c, 200, views.RegisterPage(c.QueryParam("code")))
	})

	authGroup.GET("/login", func(c echo.Context) error {
		if ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/user/")
		}
		return Render(c, 200, views.LoginPage())
	})

	authGroup.POST("/register", func(c echo.Context) error {
		values, rawValues, err := Bind[views.RegisterFormValues](c)
		if err != nil {
			return Render(c, 422, views.RegisterForm(views.NewFormError("Erreur inatendue")))
		}

		model := views.NewFormModel(rawValues, Validate(c, values))

		if len(model.Errors.Fields) > 0 {
			logger.Log.Info().Any("errors", model.Errors).Msg("Invalid form")
			return Render(c, 422, views.RegisterForm(model))
		}

		createUserParams := db.CreateUserParams{
			Email:     values.Email,
			FullName:  values.FullName,
			Apartment: values.Apartment,
		}

		err = auth.CreateHash(values.Password, &createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to hash password")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		// TODO: use a transaction
		newUser, err := queries.CreateUser(c.Request().Context(), createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create user")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		logger.Log.Info().Stringer("user", newUser.ID).Msg("User created")

		if err := addAuthenticationCookie(c, newUser.ID); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to add authentication cookie")
			// Redirect to login page to allow user to login, since we failed to set its auth cookie
			return Redirect(c, "/login")
		}

		err = sendVerificationMail(c, newUser)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send verification email")
			// Redirect to the verification page to allow user to resend the email
			return Redirect(c, "/verify")
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
			return Render(c, 200, views.VerifyPage(c.QueryParam("error")))
		}

		user, err := auth.ParseToken(queries, c, verificationToken,
			auth.WithAudience(auth.EmailVerificationAudience),
			auth.IssuedAfterLastUserUpdate(2*time.Second),
		)
		if err != nil {
			logger.Log.Error().Str("code", verificationToken).Err(err).Msg("Unable to verify user email")
			return Render(c, 200, views.VerifyPage("Le code de vérification est invalide, veuillez réssayer."))
		} else if user.ID != currentUser.ID {
			logger.Log.Error().
				Str("code", verificationToken).
				Stringer("current_user", currentUser.ID).
				Stringer("userID", user.ID).
				Msg("User ID does not match during email verification")
			return RedirectWitQuery(c, "/logout")
		}

		if currentUser.EmailVerified {
			return Redirect(c, "/user/")
		}

		if err = queries.EmailVerified(c.Request().Context(), user.ID); err != nil {
			logger.Log.Error().Str("code", verificationToken).Err(err).Msg("Unable to verify user email")
			return Render(c, 200, views.VerifyPage("Une erreur est survenue, veuillez réssayer."))
		}

		logger.Log.Info().Stringer("user", user.ID).Msg("Email verified")

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
			return Render(c, 422, views.VerifyForm(views.VerifyModel{Err: "Une erreur est survenue, veuillez réssayer."}))
		}

		<-time.After(2 * time.Second)

		return Render(c, 200, views.VerifyForm(views.VerifyModel{EmailSent: true}))
	})

	authGroup.POST("/login", func(c echo.Context) error {
		values, rawValues, err := Bind[views.LoginFormValues](c)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.LoginForm(views.NewFormError("Erreur inatendue")))
		}
		model := views.NewFormModel(rawValues, Validate(c, values))

		user, err := queries.GetUserByEmail(c.Request().Context(), values.Email)
		if err != nil {
			model.Errors.Fields["Email"] = "Cet email n'existe pas"
			return Render(c, 422, views.LoginForm(model))
		}

		ok, err := auth.CompareHashAgainstPassword(user, values.Password)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to compare password")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.LoginForm(model))
		}
		if !ok {
			model.Errors.Fields["Password"] = "Mot de passe invalide"
			return Render(c, 422, views.LoginForm(model))
		}

		if err := addAuthenticationCookie(c, user.ID); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to add authentication cookie")
			model.Errors.Global = "Erreur inatendue"
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
		values, rawValues, err := Bind[views.PasswordForgottenFormValues](c)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.PasswordForgottenForm(views.PasswordForgottenModel{
				FormModel: views.NewFormError("Erreur inatendue"),
			}))
		}

		model := views.PasswordForgottenModel{FormModel: views.NewFormModel(rawValues, Validate(c, values))}

		if len(model.Errors.Fields) > 0 {
			logger.Log.Info().Any("errors", model.Errors).Msg("Invalid form")
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		user, err := queries.GetUserByEmail(c.Request().Context(), values.Email)
		if err != nil {
			model.Errors.Fields["Email"] = "Cet email n'existe pas"
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		resetToken, err := auth.CreateToken(user.ID, auth.ResetPasswordAudience)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create reset token")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		err = mails.SendMail(c,
			user,
			"Réinitialisation de votre mot de passe Woody Wood Gate",
			emails.PasswordReset(user, templ.SafeURL(fmt.Sprintf("%s/reset-password?code=%s", BASE_URL, resetToken))),
		)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send password reset email")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		model.EmailSent = true

		return Render(c, 200, views.PasswordForgottenForm(model))
	})

	authGroup.GET("/reset-password", func(c echo.Context) error {
		code := c.QueryParam("code")
		if code == "" {
			return RedirectWitQuery(c, "/password-forgotten")
		}

		if _, err := auth.ParseToken(queries, c, code,
			auth.WithAudience(auth.ResetPasswordAudience),
			auth.IssuedAfterLastUserUpdate(0),
		); err != nil {
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

		user, err := auth.ParseToken(queries, c, code,
			auth.WithAudience(auth.ResetPasswordAudience),
			auth.IssuedAfterLastUserUpdate(0),
		)
		if err != nil {
			logger.Log.Error().Str("code", code).Err(err).Msg("Unable to reset password")
			return Redirect(c, "/password-forgotten?error="+url.QueryEscape("Code de réinitialisation invalide"))
		}

		values, rawValues, err := Bind[views.ResetPasswordFormValues](c)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.ResetPasswordForm(views.NewFormError("Erreur inatendue")))
		}

		model := views.NewFormModel(rawValues, Validate(c, values))
		if len(model.Errors.Fields) > 0 {
			logger.Log.Info().Any("errors", model.Errors).Msg("Invalid form")
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		updatePasswordParams := db.UpdatePasswordParams{
			ID: user.ID,
		}

		if err := auth.CreateHash(values.Password, &updatePasswordParams); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to hash password")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		if err := queries.UpdatePassword(c.Request().Context(), updatePasswordParams); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to update password")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.ResetPasswordForm(model))
		}

		logger.Log.Info().Stringer("user", user.ID).Msg("Password reset")

		if err := addAuthenticationCookie(c, user.ID); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to add authentication cookie")
			model.Errors.Global = "Erreur inatendue"
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

func init() {
	customValidations["uniq_email"] = CustomValidation{
		Message: "Un compte utilisant cet email existe déjà",
		ValidateCtx: func(c context.Context, fl validator.FieldLevel) bool {
			_, err := queries.GetUserByEmail(c, fl.Field().String())
			if errors.Is(err, pgx.ErrNoRows) {
				return true
			} else {
				logger.Log.Error().Err(err).Msg("Unable to get user by email")
				return false
			}
		},
	}

	apartmentRegex := regexp.MustCompile(`^[AB][0-4][0-1][0-9]$`)
	customValidations["apartment"] = CustomValidation{
		Message: "Numéro d'appartement incorrect. (ex: A001)",
		Validate: func(fl validator.FieldLevel) bool {
			return apartmentRegex.MatchString(fl.Field().String())
		},
	}

	customValidations["invitation_code"] = CustomValidation{
		Message: "Code d'invitation invalide",
		ValidateCtx: func(c context.Context, fl validator.FieldLevel) bool {
			code, err := queries.GetRegistrationCode(c)
			if err != nil {
				logger.Log.Error().Err(err).Msg("Unable to get registration code")
				return false
			}
			return fl.Field().String() == code
		},
	}
}
