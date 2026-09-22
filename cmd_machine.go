package main

import (
	"context"
	"fmt"

	"github.com/jwp23/agent-downlink/internal/setup"
)

const machineUsage = "usage: agent-downlink machine add <name> | remove <name>"

// cmdMachine manages which machines are readable here.
func cmdMachine(e env, args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(e.stderr, machineUsage)
		return 2
	}
	switch args[0] {
	case "add":
		return cmdMachineAdd(e, args[1:])
	case "remove":
		return cmdMachineRemove(e, args[1:])
	}
	_, _ = fmt.Fprintln(e.stderr, machineUsage)
	return 2
}

func cmdMachineAdd(e env, args []string) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(e.stderr, "usage: agent-downlink machine add <name>")
		return 2
	}
	if err := setup.AddMachine(context.Background(), setup.NewPrompter(e.stdin, e.stdout), e.home, args[0]); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink machine add: %v\n", err)
		return 1
	}
	return 0
}

func cmdMachineRemove(e env, args []string) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(e.stderr, "usage: agent-downlink machine remove <name>")
		return 2
	}
	if err := setup.RemoveMachine(setup.NewPrompter(e.stdin, e.stdout), e.home, args[0]); err != nil {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink machine remove: %v\n", err)
		return 1
	}
	return 0
}
