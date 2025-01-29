package handlers

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
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

		runningVersion, ok := c.Request().Header[http.CanonicalHeaderKey("x-version")]
		if ok || len(runningVersion) == 1 {
			model.RunningVersion = runningVersion[0]
		}

		if currentVersion, err := getCurrentFirmwareVersion(); err != nil {
			if !os.IsNotExist(err) {
				logger.Log.Error().Err(err).Str("running version", model.RunningVersion).Msg("failed to get current firmware version, client will not be updated")
			}
		} else if model.RunningVersion != "" && currentVersion != "none" && currentVersion != model.RunningVersion {
			logger.Log.Info().Str("running version", model.RunningVersion).Str("current version", currentVersion).Msg("running version mismatch, instruct client to upgrade")
			return c.NoContent(http.StatusUpgradeRequired)
		}

		select {
		case <-openChannel:
			return c.NoContent(http.StatusOK)
		case <-time.After(time.Duration(config.Config.Gate.Timeout) * time.Second):
			return c.NoContent(http.StatusRequestTimeout)
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
		firmwareFile, err := os.ReadFile(firmwarePath)

		if err != nil {
			if os.IsNotExist(err) {
				logger.Log.Warn().Str("firmware path", firmwarePath).Msg("No firmware uploaded yet")
				return c.NoContent(304)
			}
			logger.Log.Error().Err(err).Str("firmware path", firmwarePath).Msg("Failed to open firmware file")
			return c.NoContent(500)
		}

		md5Hasher := md5.New()
		if _, err := io.Copy(md5Hasher, bytes.NewReader(firmwareFile)); err != nil {
			logger.Log.Error().Err(err).Msg("failed to compute firmware MD5 hash")
			return c.NoContent(500)
		}

		md5Sum := hex.EncodeToString(md5Hasher.Sum(nil))
		logger.Log.Info().Str("md5", md5Sum).Str("version", firmwareVersion).Msg("sending firmware")
		w := c.Response().Writer
		w.Header().Set("x-MD5", md5Sum)
		w.Header().Set("Content-Length", strconv.Itoa(len(firmwareFile)))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Transfer-Encoding", "identity")
		return c.Blob(200, "application/octet-stream", firmwareFile)
	})
}

func getCurrentFirmwareVersion() (string, error) {
	if err := os.MkdirAll(config.Config.Gate.FirmwareDirectory, 0755); err != nil {
		return "none", fmt.Errorf("failed to open firmware directory: %w", err)
	}

	dirEntries, err := os.ReadDir(config.Config.Gate.FirmwareDirectory)
	if err != nil {
		return "none", fmt.Errorf("failed to list firmware directory files: %w", err)
	}

	if len(dirEntries) > 1 {
		return "none", fmt.Errorf("failed to determine firmware version, more than 1 file present int the firmware directory: %s", config.Config.Gate.FirmwareDirectory)
	}

	if len(dirEntries) == 0 {
		return "none", nil
	}

	return strings.Split(dirEntries[0].Name(), ".bin")[0], nil
}
