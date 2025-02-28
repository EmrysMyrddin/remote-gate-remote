package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path"
	"woody-wood-portail/cmd/config"
	ctx "woody-wood-portail/cmd/ctx/auth"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/cmd/services/mails"
	"woody-wood-portail/views"
	"woody-wood-portail/views/components"
	"woody-wood-portail/views/emails"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/skip2/go-qrcode"
)

func RegisterAdminHandlers(e RequireAuth, gateModel *Model) {
	adminGroup := e.Group.Group("/admin")
	adminGroup.Use(RequireAdminRoleMiddleware)

	adminGroup.GET("", func(c echo.Context) error {
		return Redirect(c, "/admin/users")
	})
	adminGroup.GET("/", func(c echo.Context) error {
		return Redirect(c, "/admin/users")
	})

	adminGroup.GET("/invitation", func(c echo.Context) error {
		var err error
		model := &views.AdminInvitationFormModel{}
		model.Code, err = db.Q(c).GetRegistrationCode(c.Request().Context())

		if err != nil && err != pgx.ErrNoRows {
			model.Err = err.Error()
			return Render(c, 200, views.AdminInvitationPage(model))
		}

		model.QrCode, err = invitationQrCodeHandler(model.Code)
		if err != nil {
			model.Err = err.Error()
		}

		return Render(c, 200, views.AdminInvitationPage(model))
	})

	adminGroup.POST("/invitation", func(c echo.Context) error {
		var err error
		model := &views.AdminInvitationFormModel{}
		model.Code, err = db.Q(c).GetRegistrationCode(c.Request().Context())
		if err != nil && err != pgx.ErrNoRows {
			model.Err = err.Error()
			return Render(c, 422, views.AdminInvitationForm(model))
		}

		newCode := fmt.Sprintf("%06d", rand.Int31n(899_999)+100_000)

		if err = db.Q(c).SetRegistrationCode(c.Request().Context(), newCode); err != nil {
			logger.Log.Error().Err(err).Msg("Failed to set registration code")
			model.Err = err.Error()
			return Render(c, 422, views.AdminInvitationForm(model))
		}

		model.Code = newCode
		model.QrCode, err = invitationQrCodeHandler(newCode)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to render QR code")
			model.Err = err.Error()
			return Render(c, 422, views.AdminInvitationForm(model))
		}

		if err := db.Commit(c); err != nil {
			logger.Log.Error().Err(err).Msg("Failed to commit transaction")
			model.Err = err.Error()
			return Render(c, 422, views.AdminInvitationForm(model))
		}

		return Render(c, 200, views.AdminInvitationForm(model))
	})

	adminGroup.GET("/users", func(c echo.Context) error {
		users, err := db.Q(c).ListUsers(c.Request().Context())
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}
		model := &views.AdminUsersPageModel{
			Users:    make([]db.User, 0, len(users)),
			Pending:  make([]db.User, 0),
			Rejected: make([]db.User, 0),
		}

		for _, user := range users {
			switch user.RegistrationState {
			case "pending", "new":
				model.Pending = append(model.Pending, user)
			case "rejected":
				model.Rejected = append(model.Rejected, user)
			case "accepted":
				model.Users = append(model.Users, user)
			default:
				logger.Log.Error().Str("state", user.RegistrationState).Msg("Unknown registration state")
			}
		}

		return Render(c, 200, views.AdminUsersPage(model))
	})

	adminGroup.GET("/users/:id", func(c echo.Context) error {
		userID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.String(404, "Failed to parse user ID: "+err.Error())
		}

		user, err := db.Q(c).GetUser(c.Request().Context(), userID)
		if err != nil {
			return c.NoContent(404)
		}

		model := &views.AdminUserPageModel{
			Form: views.AdminUserFormModel{
				User: user,
			},
		}

		logs, err := db.Q(c).ListLogsByUser(c.Request().Context(), userID)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to list logs")
			model.Form.Errors.Global = "Une erreur inatendue est survenue lors du chargement des demandes d'ouvertures"
		} else {
			model.Logs = logs
		}

		return Render(c, 200, views.AdminUserPage(model))
	})

	adminGroup.PUT("/users/:id", func(c echo.Context) error {
		userID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.String(404, "Failed to parse user ID: "+err.Error())
		}

		values, rawValues, err := Bind[views.AdminUserValues](c)
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to bind values")
			return Render(c, 422, views.AdminUserForm(&views.AdminUserFormModel{FormModel: components.NewFormError("Erreur inatendue", rawValues)}))
		}

		model := &views.AdminUserFormModel{
			FormModel: components.NewFormModel(rawValues, Validate(c, values)),
		}

		if model.HasError() {
			return Render(c, 422, views.AdminUserForm(model))
		}

		model.User, err = db.Q(c).UpdateUserInfo(c.Request().Context(), db.UpdateUserInfoParams{
			ID:        userID,
			Email:     values.Email,
			Role:      values.Role,
			FullName:  values.FullName,
			Apartment: values.Apartment,
		})
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to update user info")
			model.Errors.Global = "Une erreur inatendue lors de la sauvegarde"
			return Render(c, 422, views.AdminUserForm(model))
		}

		if err = db.Commit(c); err != nil {
			logger.Log.Error().Err(err).Msg("Failed to commit transaction")
			model.Errors.Global = "Une erreur inatendue lors de la sauvegarde"
			return Render(c, 422, views.AdminUserForm(model))
		}

		return Render(c, 200, views.AdminUserForm(model))
	})

	adminGroup.GET("/registrations/:id/address_proof", func(c echo.Context) error {
		userID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.String(400, "missing id param")
		}

		userAddressProofDir := addressProofDir(userID)
		entries, err := os.ReadDir(userAddressProofDir)
		if os.IsNotExist(err) || len(entries) == 0 {
			return c.NoContent(404)
		} else if err != nil {
			logger.Log.Err(err).Stringer("user id", userID).Str("dir", userAddressProofDir).Msg("failed to list user address proof files")
			return c.NoContent(500)
		}

		return c.File(path.Join(userAddressProofDir, entries[0].Name()))
	})

	adminGroup.PUT("/registrations/:id/:action", func(c echo.Context) error {
		model := &views.AdminUserRowModel{}

		userID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			model.Err = errors.New("id d'utilisateur invalide")
			return Render(c, 422, views.AdminPendingRow(model))
		}

		action := c.Param("action")

		switch action {
		case "accept":
			model.User, err = db.Q(c).RegistrationAccepted(c.Request().Context(), userID)
			if err != nil {
				model.Err = errors.New("utilisateur introuvable")
				logger.Log.Error().Err(err).Msg("Failed to accept registration")
				return Render(c, 422, views.AdminPendingRow(model))
			}

			if err := sendRegistrationAcceptedMail(c, model.User); err != nil {
				model.Err = errors.New("échec de l'envoi de l'e-mail")
				return Render(c, 422, views.AdminPendingRow(model))
			}

			if err := os.RemoveAll(addressProofDir(userID)); err != nil {
				logger.Log.Err(err).Stringer("user", userID).Str("dir", addressProofDir(userID)).Msg("failed to clean up address proof")
				model.Err = errors.New("échec de la suppression du justificatif de domicile")
				return Render(c, 422, views.AdminPendingRow(model))
			}
			logger.Log.Info().Stringer("user", userID).Msg("Successfully cleaned up address proof")

			if err := db.Commit(c); err != nil {
				model.Err = errors.New("échec de l'enregistrement")
				logger.Log.Error().Err(err).Msg("Failed to commit transaction")
				return Render(c, 422, views.AdminPendingRow(model))
			}

			return Render(c, 200, components.OOB("beforeend:#accepted-list", views.AdminAcceptedRow(model)))
		case "reject":
			model.User, err = db.Q(c).RegistrationRejected(c.Request().Context(), userID)
			if err != nil {
				model.Err = errors.New("utilisateur introuvable")
				logger.Log.Error().Err(err).Msg("Failed to reject registration")
				return Render(c, 422, views.AdminPendingRow(model))
			}

			if err := sendRegistrationRejectedMail(c, model.User); err != nil {
				model.Err = errors.New("échec de l'envoi de l'e-mail")
				return Render(c, 422, views.AdminPendingRow(model))
			}

			if err := os.RemoveAll(addressProofDir(userID)); err != nil {
				logger.Log.Err(err).Stringer("user", userID).Str("dir", addressProofDir(userID)).Msg("failed to clean up address proof")
				model.Err = errors.New("échec de la suppression du justificatif de domicile")
				return Render(c, 422, views.AdminPendingRow(model))
			}
			logger.Log.Info().Stringer("user", userID).Msg("Successfully cleaned up address proof")

			if err := db.Commit(c); err != nil {
				model.Err = errors.New("échec de l'enregistrement")
				logger.Log.Error().Err(err).Msg("Failed to commit transaction")
				return Render(c, 422, views.AdminPendingRow(model))
			}

			return Render(c, 200, components.OOB("beforeend:#rejected-list", views.AdminRejectedRow(model)))
		case "reset":
			model.User, err = db.Q(c).RegistrationPending(c.Request().Context(), userID)
			if err != nil {
				model.Err = errors.New("utilisateur introuvable")
				logger.Log.Error().Err(err).Msg("Failed to reset registration")
				return Render(c, 422, views.AdminRejectedRow(model))
			}

			if err := db.Commit(c); err != nil {
				model.Err = errors.New("échec de l'enregistrement")
				logger.Log.Error().Err(err).Msg("Failed to commit transaction")
				return Render(c, 422, views.AdminRejectedRow(model))
			}

			return Render(c, 200, components.OOB("beforeend:#pending-list", views.AdminPendingRow(model)))
		default:
			model.Err = errors.New("action inconnue")
			logger.Log.Error().Str("action", action).Msg("Unknown action")
			return Render(c, 422, views.AdminPendingRow(model))
		}
	})

	adminGroup.DELETE("/registrations/:id", func(c echo.Context) error {
		model := &views.AdminUserRowModel{}

		userID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			model.Err = errors.New("id d'utilisateur invalide")
			logger.Log.Error().Err(err).Msg("Failed to parse user ID")
			return Render(c, 422, views.AdminRejectedRow(model))
		}

		model.User, err = db.Q(c).DeleteUser(c.Request().Context(), userID)
		if err != nil {
			model.Err = errors.New("utilisateur introuvable")
			logger.Log.Error().Err(err).Msg("Failed to delete user")
			return Render(c, 422, views.AdminRejectedRow(model))
		}

		if err := db.Commit(c); err != nil {
			model.Err = errors.New("échec de l'enregistrement")
			logger.Log.Error().Err(err).Msg("Failed to commit transaction")
			return Render(c, 422, views.AdminRejectedRow(model))
		}

		return c.NoContent(200)
	})

	adminGroup.GET("/firmware", func(c echo.Context) error {
		model := views.FirmwarePageModel{}

		currentVersion, err := getCurrentFirmwareVersion()
		if err != nil && !os.IsNotExist(err) {
			logger.Log.Error().Err(err).Msg("failed to get current version")
			model.ErrorMsg = fmt.Sprintf("failed to get current version: %s", err)
			model.CurrentVersion = "none"
		}
		model.CurrentVersion = currentVersion

		if gateModel.RunningVersion == "" {
			model.RunningVersion = "none"
		} else {
			model.RunningVersion = gateModel.RunningVersion
		}

		return Render(c, 200, views.FirmwarePage(model))
	})

	adminGroup.PUT("/firmware", func(c echo.Context) (err error) {
		currentVersion, _ := getCurrentFirmwareVersion()

		firmware, err := c.FormFile("firmware")
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to get firmware file")
			return Render(c, 422, views.FirmwareUpdateResult(currentVersion, fmt.Sprintf("Impossible de récupérer le fichier uploadé : %s", err)))
		}

		src, err := firmware.Open()
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to open uploaded firmware file")
			return Render(c, 422, views.FirmwareUpdateResult(currentVersion, fmt.Sprintf("Impossible d'ouvrir le fichier envoyé : %s", err)))
		}
		defer src.Close()

		dst, err := os.Create(path.Join(config.Config.Gate.FirmwareDirectory, firmware.Filename))
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to create new firmware file")
			return Render(c, 422, views.FirmwareUpdateResult(currentVersion, fmt.Sprintf("Échec de la création du fichier : %s", err)))
		}
		defer func() {
			if err != nil {
				logger.Log.Info().Msg("error while uploading file, deleting it from disk")
				if err := os.Remove(path.Join(config.Config.Gate.FirmwareDirectory, firmware.Filename)); err != nil {
					logger.Log.Error().Err(err).Msg("failed to delete firmware file")
				}
			}
		}()
		defer dst.Close()

		if _, err = io.Copy(dst, src); err != nil {
			logger.Log.Error().Err(err).Msg("failed to save firmware")
			return Render(c, 422, views.FirmwareUpdateResult(currentVersion, fmt.Sprintf("Échec de l'enregirstement du firmware : %s", err)))
		}

		if currentVersion != "none" {
			logger.Log.Info().Str("current version", currentVersion).Msg("deleting previous firmware")
			if err = os.Remove(path.Join(config.Config.Gate.FirmwareDirectory, currentVersion+".bin")); err != nil {
				logger.Log.Error().Err(err).Msg("failed to delete firmware file")
				return Render(c, 422, views.FirmwareUpdateResult(currentVersion, fmt.Sprintf("Échec de la suppression du fichier précédent : %s", err)))
			}
		}

		logger.Log.Info().Str("firmware", firmware.Filename).Str("save path", config.Config.Gate.FirmwareDirectory).Msg("New firmware uploaded")

		currentVersion, _ = getCurrentFirmwareVersion()
		return Render(c, 200, views.FirmwareUpdateResult(currentVersion, ""))
	})
}

func invitationQrCodeHandler(code string) (string, error) {
	qrPNG, err := qrcode.Encode(config.Config.Http.BaseURL+"/register?code="+code, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG), nil
}

func RequireAdminRoleMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := ctx.GetUserFromEcho(c)
		if user.Role != "admin" {
			logger.Log.Debug().Stringer("user", user.ID).Str("role", user.Role).Msg("unauthorized access to admin page")
			return Redirect(c, "/user")
		}
		return next(c)
	}
}

func sendRegistrationAcceptedMail(c echo.Context, user db.User) error {
	if err := mails.SendMail(c.Request().Context(),
		user,
		"Inscription sur Woody Wood Gate acceptée",
		emails.RegistrationRequestAccepted(user),
	); err != nil {
		logger.Log.Error().Err(err).Msg("Unable to send accepted registration email")
	}

	return nil
}

func sendRegistrationRejectedMail(c echo.Context, user db.User) error {
	if err := mails.SendMail(c.Request().Context(),
		user,
		"Inscription sur Woody Wood Gate refusée",
		emails.RegistrationRequestRejected(user),
	); err != nil {
		logger.Log.Error().Err(err).Msg("Unable to send rejected registration email")
	}

	return nil
}

func addressProofDir(userID uuid.UUID) string {
	return path.Join(config.Config.Users.AddressProofsDirectory, userID.String())
}
