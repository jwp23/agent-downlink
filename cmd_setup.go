package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jwp23/agent-downlink/internal/setup"
)

func cmdSetup(e env, args []string) int {
	flags := flag.NewFlagSet("agent-downlink setup", flag.ContinueOnError)
	flags.SetOutput(e.stderr)
	noTimer := flags.Bool("no-timer", false, "do not install the hourly schedule (for a machine making a single push)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(e.stderr, "usage: agent-downlink setup [--no-timer]")
		return 2
	}
	hostname, _ := os.Hostname() // no hostname just means no default machine name
	opts := setup.Options{
		Home:     e.home,
		Hostname: hostname,
		NoTimer:  *noTimer,
		InstallTimer: func() error {
			binary, err := ownBinaryFn()
			if err != nil {
				return err
			}
			return newScheduler(e.home, binary).Install()
		},
	}
	if err := setup.Run(context.Background(), setup.NewPrompter(e.stdin, e.stdout), opts); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink setup: %v\n", err)
		return 1
	}
	return 0
}
