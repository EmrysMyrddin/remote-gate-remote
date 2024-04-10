package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog"
)

func LoggerMiddleware() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:     true,
		LogStatus:  true,
		LogLatency: true,
		LogMethod:  true,
		LogError:   true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			requestLogger.Info().
				Str("method", fmt.Sprintf("%-6s", v.Method)).
				Str("URI", v.URI).
				Int("status", v.Status).
				Err(v.Error).
				Dur("latency", v.Latency).
				Send()

			return nil
		},
	})
}

var requestLogger = zerolog.New(zerolog.ConsoleWriter{
	Out:        os.Stderr,
	TimeFormat: time.TimeOnly,
	FormatFieldName: func(i interface{}) string {
		return ""
	},

	PartsOrder:    []string{"time", "level", "message", "status", "method", "latency", "URI"},
	FieldsExclude: []string{"method", "status", "latency", "URI"},
}).With().Timestamp().Logger()

var Log = zerolog.New(zerolog.ConsoleWriter{
	Out:        os.Stderr,
	TimeFormat: time.TimeOnly,
	FormatMessage: func(i interface{}) string {
		return fmt.Sprintf("---        %s", i)
	},
}).With().Timestamp().Stack().Logger()
