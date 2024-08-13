package timezone

import (
	"fmt"
	"time"

	_ "time/tzdata" // Include TZ data into the binary
)

var TZ *time.Location

func init() {
	var err error
	TZ, err = time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(fmt.Errorf("failed to load timezone: %w", err))
	}
}
