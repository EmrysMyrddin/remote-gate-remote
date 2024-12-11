package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
	"woody-wood-portail/cmd/config"
	"woody-wood-portail/cmd/handlers"
	"woody-wood-portail/cmd/logger"
	"woody-wood-portail/cmd/services/db"
	"woody-wood-portail/cmd/services/mails"
	"woody-wood-portail/cmd/timezone"
	"woody-wood-portail/views/emails"

	"github.com/go-playground/validator/v10"
	"github.com/robfig/cron"
	"github.com/rs/zerolog"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

var dailyCronJobs = map[string]func(){
	"logs cleanup":                 db.DeleteOldLogs,
	"code cycle":                   db.CycleRegistrationCode,
	"registration expiration mail": sendExpiredRegistrationMails,
	"disable expired accounts":     disableExpiredAccounts,
	"delete old accounts":          deleteOldAccounts,
}

func main() {
	db.Migrate()

	pool, err := db.Connect()
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer pool.Close()

	c := cron.NewWithLocation(timezone.TZ)

	// Register all cron jobs as daily jobs.
	// This is to avoid missing deadlines if the server restarts.
	// TODO: Implement a proper Schedule to run jobs only when needed
	for job, handler := range dailyCronJobs {
		err = c.AddFunc("@daily", handler)
		if err != nil {
			logger.Log.Fatal().Err(err).Str("job", job).Msg("failed to add cron job")
		}
	}
	c.Start()

	db.CycleRegistrationCode()

	e := echo.New()

	e.Use(logger.LoggerMiddleware())
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			logger.Log.Error().Err(err).Msg("request failed with panic\n" + string(stack))
			return err
		},
	}))
	e.Use(db.TransactionMiddleware())

	e.Static("/static", "static")

	e.GET("/", func(c echo.Context) error {
		return handlers.Redirect(c, "/login")
	})

	openChannel := make(chan struct{}, 1)
	model := handlers.NewModel()

	handlers.RegisterAuthHandlers(e)
	handlers.RegisterGateHandlers(e, &model, openChannel)

	requireAuth := handlers.RequireAuthGroup(e)
	handlers.RegisterUserHandlers(requireAuth, &model, openChannel)
	handlers.RegisterAdminHandlers(requireAuth, &model)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		if err := e.Start(":" + config.Config.Http.Port); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal().Err(err).Msg("HTTP server crashed")
		}
	}()

	<-sigCtx.Done()
	logger.Log.Info().Msg("Shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		logger.Log.Fatal().Err(err).Msg("Failed to gracefully shutdown")
	}
	logger.Log.Info().Msg("Server stopped")
}

type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}

func sendExpiredRegistrationMails() {
	reminders := strings.Split(config.Config.Users.ReminderDays, ",")

	for _, days := range reminders {
		sendExpiredRegistrationMailsFor(strings.TrimSpace(days))
	}
}

func sendExpiredRegistrationMailsFor(days string) {
	logger.Log.Info().Msg("sending reminder mail to renew registrations")

	usersToRemind, err := db.QGlobal().ListUsersRegisteredSince(context.Background(), config.Config.Users.RenewalInterval+" -"+days+" days")
	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to list users expiring in " + days + " days")
		return
	}
	logger.Log.Debug().Str("days", days).Array("users", zerolog.Arr().Interface(usersToRemind)).Msg("sending reminder mail to renew registration")

	for _, user := range usersToRemind {
		err := mails.SendMail(context.Background(), user,
			"Votre inscription à Woody Wood gate va expirer dans "+days+" jours",
			emails.RegistrationWillExpire(days+" jours"),
		)
		if err != nil {
			logger.Log.Error().Err(err).Msg("failed to send " + days + " days registration expiration mail")
			continue
		}
		logger.Log.Info().Str("user", user.Email).Msg("sent " + days + " days registration expiration mail")
	}
}

func disableExpiredAccounts() {
	logger.Log.Info().Msg("checking for users to suspend")
	q := db.QGlobal()

	usersToDisable, err := q.ListUsersRegisteredSince(context.Background(), config.Config.Users.RenewalInterval)

	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to list expired users to disable")
		return
	}

	for _, user := range usersToDisable {
		if _, err := q.RegistrationSuspended(context.Background(), user.ID); err != nil {
			logger.Log.Error().Err(err).Stringer("user", user.ID).Msg("failed to suspend user")
			continue
		}

		err := mails.SendMail(context.Background(), user,
			"Votre compte Woody Wood Gate a été suspendu",
			emails.RegistrationSuspended(),
		)
		if err != nil {
			logger.Log.Error().Err(err).Msg("failed to send user expiration email")
			continue
		}

		logger.Log.Info().Stringer("user", user.ID).Msg("user account suspended")
	}
}

func deleteOldAccounts() {
	logger.Log.Info().Msg("checking for users to delete")
	q := db.QGlobal()

	usersToDelete, err := q.ListUsersRegisteredSince(context.Background(), "1 year "+config.Config.Users.RenewalInterval)

	if err != nil {
		logger.Log.Error().Err(err).Msg("failed to list old users to delete")
		return
	}

	if len(usersToDelete) == 0 {
		logger.Log.Info().Msg("no old users to delete detected")
		return
	}

	logger.Log.Debug().Array("found users to delete", zerolog.Arr().Interface(usersToDelete)).Msg("users")

	for _, user := range usersToDelete {
		deleted, err := q.DeleteUser(context.Background(), user.ID)
		if err != nil {
			logger.Log.Error().Err(err).Stringer("user", user.ID).Msg("failed to delete old user")
			continue
		}

		logger.Log.Info().Interface("deleted user", deleted).Msg("old user deleted")
		if err := mails.SendMail(context.Background(), deleted,
			"Votre compte Woody Wood Gate a été supprimé",
			emails.AccountDeleted(),
		); err != nil {
			logger.Log.Error().Str("email", deleted.Email).Err(err).Msg("failed to send deleted account notification")
			// We can ignore the error, it's just a notification
		}
	}

	logger.Log.Info().Array("deleted users", zerolog.Arr().Interface(usersToDelete)).Msg("old users deleted")
}
