package config

import (
	"strings"
	"woody-wood-portail/cmd/logger"

	"github.com/go-playground/validator/v10"
	"github.com/mailjet/mailjet-apiv3-go/v4"
	"github.com/spf13/viper"

	_ "github.com/joho/godotenv/autoload"
)

var Config config

type config struct {
	Http struct {
		Port    string
		BaseURL string `mapstructure:"base_url"`
		JWT     struct {
			Secret string `validate:"required"`
			// MaxAge of the JWT token in days
			MaxAge int `mapstructure:"max_age"`
		}
	}

	Users struct {
		RenewalInterval        string `mapstructure:"renewal_interval"`
		ReminderDays           string `mapstructure:"reminder_days"`
		AddressProofsDirectory string `mapstructure:"address_proof_directory"`
	}

	Gate struct {
		Secret string `validate:"required"`
		// Timeout of the polling request in seconds
		Timeout           int
		FirmwareDirectory string `mapstructure:"firmware_directory"`
	}

	Database struct {
		URL            string
		MigrateOnStart bool `mapstructure:"migrate_on_start"`
	}

	Mail struct {
		APIKey    string `validate:"required" mapstructure:"api_key"`
		SecretKey string `validate:"required" mapstructure:"secret_key"`
		Sender    mailjet.RecipientV31
	}
}

func init() {
	Config.Http.Port = "80"
	Config.Http.BaseURL = "http://localhost"
	Config.Http.JWT.MaxAge = 30

	Config.Database.URL = "user=postgres dbname=gate password=postgres host=localhost"
	Config.Database.MigrateOnStart = true

	Config.Mail.Sender.Email = "woody-wood-gate@cocaud.dev"
	Config.Mail.Sender.Name = "Woody Wood Gate"

	Config.Gate.Timeout = 60
	Config.Gate.FirmwareDirectory = "/usr/src/app/firmwares"

	Config.Users.ReminderDays = "7, 3, 1"
	Config.Users.RenewalInterval = "2 months"
	Config.Users.AddressProofsDirectory = "/usr/src/app/address_proofs"

	v := viper.New()
	v.AutomaticEnv()
	err := v.Unmarshal(&Config)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("unable to unmarshal config")
	}

	if err := validator.New().Struct(Config); err != nil {
		validationErr, ok := err.(validator.ValidationErrors)
		if !ok {
			logger.Log.Fatal().Err(err).Msg("Failed to validate config")
		}

		for _, e := range validationErr {
			logger.Log.Warn().Str("field", strings.TrimPrefix(e.Namespace(), "config.")).Str("reason", e.Tag()).Msg("invalid config field")
		}

		logger.Log.Fatal().Msg("configuration is not valid")
	}

	logger.Log.Debug().Interface("config", Config).Msg("config loaded")
}
