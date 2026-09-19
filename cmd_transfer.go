package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/rclone"
	"github.com/jwp23/agent-downlink/internal/runlog"
	"github.com/jwp23/agent-downlink/internal/status"
	"github.com/jwp23/agent-downlink/internal/transfer"
)

func cmdRun(e env, args []string) int {
	return runCycle(e, "run", true, (*transfer.Cycle).Run)
}

func cmdPush(e env, args []string) int {
	return runCycle(e, "push", false, (*transfer.Cycle).Push)
}

func cmdPull(e env, args []string) int {
	return runCycle(e, "pull", false, (*transfer.Cycle).Pull)
}

// runCycle is everything the three transfer commands share. quietWhenBusy is for the timer:
// an hourly run that overlaps a long one is normal and says nothing.
func runCycle(e env, name string, quietWhenBusy bool, steps func(*transfer.Cycle, context.Context) bool) int {
	paths := config.PathsFor(e.home)
	errors := io.Discard
	if e.interactive {
		errors = e.stderr
	}

	cycle, err := loadCycle(e.home, paths, errors)
	if err != nil {
		_, _ = fmt.Fprintf(errors, "agent-downlink %s: %v\n", name, err)
		recordStartupFailure(paths, err)
		return 1
	}

	release, acquired, err := transfer.Lock(paths.LockFile)
	if err != nil {
		_, _ = fmt.Fprintf(errors, "agent-downlink %s: %v\n", name, err)
		recordStartupFailure(paths, err)
		return 1
	}
	if !acquired {
		if quietWhenBusy {
			return 0
		}
		_, _ = fmt.Fprintf(errors, "agent-downlink %s: another run is in progress; try again when it finishes\n", name)
		return 1
	}
	defer release()

	if !steps(cycle, context.Background()) {
		return 1
	}
	return 0
}

// recordStartupFailure notes a failure that happened before there was a Cycle to record it,
// so it reaches both the log and status.json: a person running `agent-downlink status` must
// see a broken config or a missing rclone, not steps that quietly stopped running.
func recordStartupFailure(paths config.Paths, err error) {
	now := time.Now()
	_ = runlog.New(paths.LogFile).Append(now, "startup", "FAILED", err.Error())
	if st, loadErr := status.Load(paths.StatusFile); loadErr == nil {
		st.Record("startup", now, false, err.Error())
		_ = st.Save(paths.StatusFile)
	}
}

func loadCycle(home string, paths config.Paths, errors io.Writer) (*transfer.Cycle, error) {
	if err := config.CheckPermissions(paths); err != nil {
		return nil, err
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return nil, err
	}
	secrets, err := config.LoadRcloneConf(paths.RcloneConf)
	if err != nil {
		return nil, err
	}
	sources, err := cfg.Sources(home)
	if err != nil {
		return nil, err
	}
	runner, err := rclone.New(cfg.Rclone, paths.RcloneConf)
	if err != nil {
		return nil, err
	}
	var readable []string
	for _, machine := range secrets.Machines() {
		if machine != cfg.Machine {
			readable = append(readable, machine)
		}
	}
	return &transfer.Cycle{
		Machine:    cfg.Machine,
		Mirror:     cfg.Mirror,
		Sources:    sources,
		Readable:   readable,
		Runner:     runner,
		StatusPath: paths.StatusFile,
		Log:        runlog.New(paths.LogFile),
		Now:        time.Now,
		Errors:     errors,
	}, nil
}
