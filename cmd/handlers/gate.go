package handlers

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"woody-wood-portail/cmd/config"
	"woody-wood-portail/cmd/logger"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func RegisterGateHandlers(e *echo.Echo, model *Model, openChannel chan struct{}) {

	gateRoutes := e.Group("/gate")

	middleware.DefaultKeyAuthConfig.AuthScheme = ""
	gateRoutes.Use(middleware.KeyAuth(func(key string, c echo.Context) (bool, error) {
		return key == config.Config.Gate.Secret, nil
	}))

	gateHandler := func(c echo.Context) error {
		model.gateConnected()
		defer model.gateDisconnected()

		currentVersion, ok := c.Request().Header[http.CanonicalHeaderKey("x-version")]
		if ok || len(currentVersion) == 1 {
			model.RunningVersion = currentVersion[0]
		}

		select {
		case <-openChannel:
			return c.NoContent(200)
		case <-time.After(time.Duration(config.Config.Gate.Timeout) * time.Second):
			return c.NoContent(408)
		case <-c.Request().Context().Done():
			return nil
		}
	}

	gateRoutes.GET("", gateHandler)
	gateRoutes.GET("/", gateHandler)

	// Don't use the gate group to skip authentication, the firmware can be public
	e.GET("/gate/firmware", func(c echo.Context) error {
		firmwareVersion, err := getCurrentFirmwareVersion()
		if err != nil {
			logger.Log.Error().Err(err).Msg("Failed to get firmware version")
			return c.NoContent(500)
		}

		if runningVersion, exists := c.Request().Header["X-Esp32-Version"]; exists {
			if len(runningVersion) != 1 {
				logger.Log.Error().Msg("Version Header (X-Esp32-Version) is present more than once")
				return c.NoContent(500)
			}

			if runningVersion[0] == firmwareVersion {
				return c.NoContent(304)
			}
		}

		firmwarePath := path.Join(config.Config.Gate.FirmwareDirectory, firmwareVersion+".bin")
		firmwareFile, err := os.Open(firmwarePath)

		if err != nil {
			logger.Log.Error().Err(err).Str("firmware path", firmwarePath).Msg("Failed to open firmware file")
			return c.NoContent(500)
		}

		md5Hasher := md5.New()
		if _, err := io.Copy(md5Hasher, firmwareFile); err != nil {
			logger.Log.Error().Err(err).Msg("failed to compute firmware MD5 hash")
			return c.NoContent(500)
		}

		md5Sum := hex.EncodeToString(md5Hasher.Sum(nil))
		logger.Log.Info().Str("md5", md5Sum).Str("version", firmwareVersion).Msg("sending firmware")

		c.Response().Header().Add("x-MD5", md5Sum)
		firmwareFile.Seek(0, 0) // reset the read offset, because the hashing moved it to the end.
		return c.Stream(200, "", firmwareFile)
	})
}

func getCurrentFirmwareVersion() (string, error) {
	if err := os.MkdirAll(config.Config.Gate.FirmwareDirectory, 0755); err != nil {
		return "", fmt.Errorf("failed to open firmware directory: %w", err)
	}

	dirEntries, err := os.ReadDir(config.Config.Gate.FirmwareDirectory)
	if err != nil {
		return "", fmt.Errorf("failed to list firmware directory files: %w", err)
	}

	if len(dirEntries) > 1 {
		return "", fmt.Errorf("failed to determine firmware version, more than 1 file present int the firmware directory")
	}

	if len(dirEntries) == 0 {
		return "none", nil
	}

	return strings.Split(dirEntries[0].Name(), ".bin")[0], nil
}
