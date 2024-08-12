package views

import (
	"fmt"
	"time"

	_ "time/tzdata" // Include TZ data into the binary
)

var tz *time.Location

func init() {
	var err error
	tz, err = time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(fmt.Errorf("failed to load timezone: %w", err))
	}
}
