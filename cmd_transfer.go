package main

import (
	"context"
	"flag"
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
	if !noArgs(e, "run", args) {
		return 2
	}
	return runCycle(e, "run", true, 0, (*transfer.Cycle).Run)
}

func cmdPush(e env, args []string) int {
	if !noArgs(e, "push", args) {
		return 2
	}
	return runCycle(e, "push", false, 0, (*transfer.Cycle).Push)
}

// cmdPull takes --transfers=N to tune one pull without editing config.toml.
func cmdPull(e env, args []string) int {
	flags := flag.NewFlagSet("agent-downlink pull", flag.ContinueOnError)
	flags.SetOutput(e.stderr)
	transfers := flags.Int("transfers", 0, "files to copy at the same time; 0 keeps the value from config.toml")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(e.stderr, "usage: agent-downlink pull [--transfers=N]")
		return 2
	}
	if *transfers < 0 {
		_, _ = fmt.Fprintf(e.stderr, "agent-downlink pull: --transfers must be 1 or more; 0 keeps the value from config.toml, got %d\n", *transfers)
		return 2
	}
	return runCycle(e, "pull", false, *transfers, (*transfer.Cycle).Pull)
}

// noArgs reports whether a command that takes no arguments was given none, printing its usage
// line when it was given some.
func noArgs(e env, name string, args []string) bool {
	if len(args) != 0 {
		_, _ = fmt.Fprintf(e.stderr, "usage: agent-downlink %s\n", name)
		return false
	}
	return true
}

// runCycle is everything the three transfer commands share. quietWhenBusy is for the timer:
// an hourly run that overlaps a long one is normal and says nothing. A non-zero transfers
// overrides config.toml's copy parallelism for this run alone.
func runCycle(e env, name string, quietWhenBusy bool, transfers int, steps func(*transfer.Cycle, context.Context) bool) int {
	paths := config.PathsFor(e.home)
	errors := io.Discard
	if e.interactive {
		errors = e.stderr
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

	cycle, err := loadCycle(e.home, paths, errors, transfers)
	if err != nil {
		_, _ = fmt.Fprintf(errors, "agent-downlink %s: %v\n", name, err)
		recordStartupFailure(paths, err)
		return 1
	}

	recordStartupSuccess(paths)
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

// recordStartupSuccess clears an earlier startup failure once loading the configuration and
// taking the lock has succeeded, so a person running `agent-downlink status` after fixing the
// problem sees it resolved rather than stuck on the last failure.
func recordStartupSuccess(paths config.Paths) {
	st, err := status.Load(paths.StatusFile)
	if err != nil {
		return
	}
	if _, recorded := st.Steps["startup"]; !recorded {
		return
	}
	st.Record("startup", time.Now(), true, "")
	_ = st.Save(paths.StatusFile)
}

func loadCycle(home string, paths config.Paths, errors io.Writer, transfers int) (*transfer.Cycle, error) {
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
	concurrency := rclone.Concurrency{Transfers: cfg.Transfers, Checkers: cfg.Checkers}
	if transfers != 0 {
		concurrency.Transfers = transfers
	}
	runner, err := rclone.New(cfg.Rclone, paths.RcloneConf, concurrency)
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
