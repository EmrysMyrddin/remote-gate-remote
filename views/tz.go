package views

import (
	"fmt"
	"time"
)

var tz *time.Location

func init() {
	var err error
	tz, err = time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(fmt.Errorf("failed to load timezone: %w", err))
	}
}
