package db

import (
	"database/sql"
	"embed"
	"woody-wood-portail/cmd/config"
	"woody-wood-portail/cmd/logger"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func Migrate() {
	if !config.Config.Database.MigrateOnStart {
		logger.Log.Info().Msg("goose: database migration disabled")
		return
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("goose: failed to open database")
	}
	defer db.Close()

	goose.SetLogger(logger.StdLog)
	goose.SetBaseFS(embedMigrations)

	currentVersion, err := goose.EnsureDBVersion(db)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("goose: failed to get current version")
	}

	logger.Log.Info().Int64("currentVersion", currentVersion).Msg("goose: starting database migration")

	if err := goose.SetDialect("postgres"); err != nil {
		logger.Log.Fatal().Err(err).Msg("goose: failed to set dialect")
	}

	if err := goose.Status(db, "migrations"); err != nil {
		logger.Log.Fatal().Err(err).Msg("goose: failed to get migration status")
	}

	if err := goose.Up(db, "migrations"); err != nil {
		logger.Log.Fatal().Err(err).Msg("goose: failed to migrate")
	}

	if err != nil {
		logger.Log.Fatal().Err(err).Msg("goose: failed to get migrated version")
	}
}
