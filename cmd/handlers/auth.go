package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"time"
	"woody-wood-portail/cmd/config"
	ctx "woody-wood-portail/cmd/ctx/auth"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/auth"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/cmd/services/mails"
	"woody-wood-portail/views"
	components "woody-wood-portail/views/components"
	"woody-wood-portail/views/emails"

	"github.com/a-h/templ"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type RequireAuth struct {
	*echo.Group
}

func RequireAuthGroup(e *echo.Echo) RequireAuth {
	e.GET("/panic", func(c echo.Context) error {
		panic("Panic")
	})
	requireAuth := e.Group("")
	requireAuth.Use(auth.JWTMiddleware(func(c echo.Context, err error) error {
		if errors.Is(err, auth.ErrEmailNotVerified) {
			logger.Log.Debug().Err(err).Msg("not verified, redirecting to /verify")
			RedirectWitQuery(c, "/verify")
		} else if errors.Is(err, auth.ErrRegistrationNotAccepted) {
			if c.Get("user").(db.User).RegistrationState == "suspended" {
				logger.Log.Debug().Err(err).Msg("registration not accepted, redirecting to /renew-registration")
				RedirectWitQuery(c, "/renew-registration")
			} else {
				logger.Log.Debug().Err(err).Msg("registration not accepted, redirecting to /pending-registration")
				RedirectWitQuery(c, "/pending-registration")
			}
		} else if errors.Is(err, auth.ErrJWTMissing) {
			logger.Log.Debug().Err(err).Msg("not logged in, redirecting to /login")
			RedirectWitQuery(c, "/login?redirect="+url.QueryEscape(c.Request().URL.Path))
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

	authGroup.Use(auth.JWTMiddleware(func(c echo.Context, err error) error {
		if !errors.Is(err, auth.ErrJWTMissing) && !errors.Is(err, auth.ErrEmailNotVerified) && !errors.Is(err, auth.ErrRegistrationNotAccepted) {
			logger.Log.Debug().Err(err).Msg("invalid JWT, logging out")
			RedirectWitQuery(c, "/logout")
		}
		return nil
	}))

	authGroup.GET("/register", func(c echo.Context) error {
		if ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/renew-registration")
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
			return Render(c, 422, views.RegisterForm(components.NewFormError("Erreur inatendue")))
		}

		model := components.NewFormModel(rawValues, Validate(c, values))

		addressProofFile, err := c.FormFile("AddressProofFile")
		if err != nil {
			logger.Log.Err(err).Msg("failed to get address proof file from form")
			model.Errors.Fields["AddressProofFile"] = "Le justificatif de domicile est obligatoire"
		}

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

		newUser, err := db.Q(c).CreateUser(c.Request().Context(), createUserParams)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to create user")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		userAddressProofSrc, err := addressProofFile.Open()
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to upload user address proofs dir")
			model.Errors.Global = "Erreur inatendue durant l'upload du document"
			return Render(c, 422, views.RegisterForm(model))
		}
		defer userAddressProofSrc.Close()

		userAddressProofsDir := addressProofDir(newUser.ID)
		if err := os.MkdirAll(userAddressProofsDir, 0755); err != nil {
			logger.Log.Error().Err(err).Str("path", userAddressProofsDir).Msg("Failed to create user address proofs dir")
			model.Errors.Global = "Erreur inatendue durant l'upload du document"
			return Render(c, 422, views.RegisterForm(model))
		}

		userAddressProofPath := path.Join(userAddressProofsDir, addressProofFile.Filename)
		userAddressProofsDst, err := os.Create(userAddressProofPath)
		if err != nil {
			logger.Log.Error().Err(err).Str("path", userAddressProofPath).Msg("Failed to create user address proofs file")
			model.Errors.Global = "Erreur inatendue durant l'upload du document"
			return Render(c, 422, views.RegisterForm(model))
		}
		defer userAddressProofsDst.Close()

		if _, err := io.Copy(userAddressProofsDst, userAddressProofSrc); err != nil {
			logger.Log.Error().Err(err).Str("path", userAddressProofPath).Msg("Failed to copy proofs file")
			model.Errors.Global = "Erreur inatendue durant l'upload du document"
			if err := os.RemoveAll(userAddressProofsDir); err != nil {
				logger.Log.Err(err).Str("dir", userAddressProofsDir).Msg("failed to clean up user address proof dir")
			}
			return Render(c, 422, views.RegisterForm(model))
		}

		if err := db.Commit(c); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to commit transaction")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.RegisterForm(model))
		}

		logger.Log.Info().Stringer("user", newUser.ID).Msg("User created")

		if err := addAuthenticationCookie(c, newUser.ID); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to add authentication cookie")
			// Redirect to login page to allow user to login, since we failed to set its auth cookie
			return Redirect(c, "/login")
		}

		if err = sendVerificationMail(c, newUser); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send verification email")
			// Redirect to the verification page to allow user to resend the email
			return Redirect(c, "/verify?error="+url.QueryEscape("Une erreur est survenue, veuillez réssayer."))
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

		user, err := auth.ParseToken(c, verificationToken,
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

		if err = db.Q(c).EmailVerified(c.Request().Context(), user.ID); err != nil {
			logger.Log.Error().Str("code", verificationToken).Err(err).Msg("Unable to verify user email")
			return Render(c, 200, views.VerifyPage("Une erreur est survenue, veuillez réssayer."))
		}

		if err := db.Commit(c); err != nil {
			logger.Log.Error().Err(err).Msg("Unable to commit transaction")
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
			return Render(c, 422, views.LoginForm(components.NewFormError("Erreur inatendue")))
		}
		model := components.NewFormModel(rawValues, Validate(c, values))

		user, err := db.Q(c).GetUserByEmail(c.Request().Context(), values.Email)
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

	authGroup.GET("/pending-registration", func(c echo.Context) error {
		if !ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/login")
		}
		currentUser := ctx.GetUserFromEcho(c)

		if currentUser.RegistrationState == "accepted" {
			return Redirect(c, "/user/")
		}

		if currentUser.RegistrationState == "rejected" {
			return Render(c, 200, views.RejectedRegistrationPage())
		}

		if currentUser.RegistrationState == "new" {
			if err := sendRegistrationRequestMail(c, currentUser); err != nil {
				logger.Log.Error().Err(err).Msg("Unable to send registration request")
				return Render(c, 200, views.FailedRegistrationPage())
			}

			if _, err := db.Q(c).RegistrationPending(c.Request().Context(), currentUser.ID); err != nil {
				logger.Log.Error().Err(err).Msg("Unable to set registration state to pending")
				return Render(c, 200, views.FailedRegistrationPage())
			}
		}

		return Render(c, 200, views.PendingRegistrationPage())
	})

	authGroup.GET("/renew-registration", func(c echo.Context) error {
		if !ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/login")
		}

		return Render(c, 200, views.RegistrationRenewalPage(c.QueryParam("code")))
	})

	authGroup.PUT("/renew-registration", func(c echo.Context) error {
		if !ctx.IsAuthenticated(c) {
			return RedirectWitQuery(c, "/login")
		}

		currentUser := ctx.GetUserFromEcho(c)

		values, rawValues, err := Bind[views.RegistrationRenewalFormValues](c)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to get form params")
			return Render(c, 422, views.RegistrationRenewalForm(components.NewFormError("Erreur inatendue")))
		}

		model := components.NewFormModel(rawValues, Validate(c, values))
		if len(model.Errors.Fields) != 0 {
			return Render(c, 422, views.RegistrationRenewalForm(model))
		}

		if _, err := db.Q(c).RenewRegistration(c.Request().Context(), currentUser.ID); err != nil {
			logger.Log.Error().Err(err).Stringer("user", currentUser.ID).Msg("failed to save registration renewal")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.RegistrationRenewalForm(model))
		}

		if err := mails.SendMail(c.Request().Context(), currentUser,
			"Inscription à Woody Wood Gate renouvellée",
			emails.RegistrationRenewed(),
		); err != nil {
			logger.Log.Error().Err(err).Stringer("user", currentUser.ID).Msg("failed to send renewal confirmation email")
			// Don't report this error to user nor rollback, it's just a notification mail
		}

		if err := db.Commit(c); err != nil {
			logger.Log.Error().Err(err).Stringer("user", currentUser.ID).Msg("failed to commit while renewing registration")
			model.Errors.Global = "Erreur inatendue"
			return Render(c, 422, views.RegistrationRenewalForm(model))
		}

		return Redirect(c, "/user")
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
				FormModel: components.NewFormError("Erreur inatendue"),
			}))
		}

		model := views.PasswordForgottenModel{FormModel: components.NewFormModel(rawValues, Validate(c, values))}

		if len(model.Errors.Fields) > 0 {
			logger.Log.Info().Any("errors", model.Errors).Msg("Invalid form")
			return Render(c, 422, views.PasswordForgottenForm(model))
		}

		user, err := db.Q(c).GetUserByEmail(c.Request().Context(), values.Email)
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

		resetURL := fmt.Sprintf("%s/reset-password?code=%s", config.Config.Http.BaseURL, resetToken)

		err = mails.SendMail(c.Request().Context(),
			user,
			"Réinitialisation de votre mot de passe Woody Wood Gate",
			emails.PasswordReset(user, templ.SafeURL(resetURL)),
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

		if _, err := auth.ParseToken(c, code,
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

		user, err := auth.ParseToken(c, code,
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
			return Render(c, 422, views.ResetPasswordForm(components.NewFormError("Erreur inatendue")))
		}

		model := components.NewFormModel(rawValues, Validate(c, values))
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

		if err := db.Q(c).UpdatePassword(c.Request().Context(), updatePasswordParams); err != nil {
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

	c.SetCookie(createCookie(token, config.Config.Http.JWT.MaxAge))
	return nil
}

func createCookie(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     "authorization",
		Value:    token,
		HttpOnly: true,
		MaxAge:   maxAge * 24 * 60 * 60,
	}
}

func sendVerificationMail(c echo.Context, user db.User) error {
	mailVerifToken, err := auth.CreateToken(user.ID, auth.EmailVerificationAudience)
	if err != nil {
		return fmt.Errorf("unable to create email verification token: %w", err)
	}

	verificationURL := fmt.Sprintf("%s/verify?code=%s", config.Config.Http.BaseURL, mailVerifToken)

	err = mails.SendMail(c.Request().Context(),
		user,
		"Votre lien de vérification de compte Woody Wood Gate",
		emails.EmailVerification(user, templ.SafeURL(verificationURL)),
	)
	if err != nil {
		return fmt.Errorf("unable to send verification email: %w", err)
	}

	return nil
}

func sendRegistrationRequestMail(c echo.Context, user db.User) error {
	admins, err := db.Q(c).ListUsersByRole(c.Request().Context(), "admin")
	if err != nil {
		return fmt.Errorf("unable to list admins: %w", err)
	}

	adminURL := fmt.Sprintf("%s/admin", config.Config.Http.BaseURL)

	errs := make([]error, 0, len(admins))
	for _, admin := range admins {
		if err := mails.SendMail(c.Request().Context(),
			admin,
			"Nouvelle demande d'inscription sur Woody Wood Gate",
			emails.RegistrationRequest(user, templ.SafeURL(adminURL)),
		); err != nil {
			errs = append(errs, err)
		}
	}

	for _, err := range errs {
		if err != nil {
			logger.Log.Error().Err(err).Msg("Unable to send registration request")
		}
	}

	if len(errs) == len(admins) {
		return fmt.Errorf("unable to send registration request email: all mail tentative failed")
	}

	if err = mails.SendMail(c.Request().Context(),
		user,
		"Demande d'inscription sur Woody Wood Gate",
		emails.RegistrationRequestPending(user),
	); err != nil {
		logger.Log.Error().Err(err).Msg("Unable to send pending registration email")
	}

	return nil
}

func init() {
	customValidations["uniq_email"] = CustomValidation{
		Message: "Un compte utilisant cet email existe déjà",
		ValidateCtx: func(c context.Context, fl validator.FieldLevel) bool {
			_, err := db.Qtempl(c).GetUserByEmail(c, fl.Field().String())
			if errors.Is(err, pgx.ErrNoRows) {
				return true
			} else {
				logger.Log.Error().Err(err).Msg("Unable to get user by email")
				return false
			}
		},
	}

	apartmentRegex := regexp.MustCompile(`^(A[0-4]|B[0-5])[0-1][0-9]$`)
	customValidations["apartment"] = CustomValidation{
		Message: "Numéro d'appartement incorrect. (ex: A001)",
		Validate: func(fl validator.FieldLevel) bool {
			return apartmentRegex.MatchString(fl.Field().String())
		},
	}

	customValidations["invitation_code"] = CustomValidation{
		Message: "Code d'invitation invalide",
		ValidateCtx: func(c context.Context, fl validator.FieldLevel) bool {
			code, err := db.Qtempl(c).GetRegistrationCode(c)
			if err == pgx.ErrNoRows {
				logger.Log.Info().Msg("No registration code found, allowing registration")
				return true
			}
			if err != nil {
				logger.Log.Error().Err(err).Msg("Unable to get registration code")
				return false
			}
			return fl.Field().String() == code
		},
	}
}
