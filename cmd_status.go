package main

import (
	"fmt"
	"time"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/status"
)

// cmdStatus reads only non-secret state, so it does not need the configuration.
func cmdStatus(e env, args []string) int {
	paths := config.PathsFor(e.home)
	f, err := status.Load(paths.StatusFile)
	if err != nil {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink status: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprint(e.stdout, status.Render(f, time.Now(), paths.LogFile))
	return 0
}
