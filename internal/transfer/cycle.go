// Package transfer is the run cycle: copy this machine's records into the mirror, push that
// folder to the bucket, and pull every other readable machine. Every copy is "rclone copy",
// so nothing is ever deleted anywhere.
package transfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/plugintimeline"
	"github.com/jwp23/agent-downlink/internal/rclone"
	"github.com/jwp23/agent-downlink/internal/runlog"
	"github.com/jwp23/agent-downlink/internal/status"
)

// Cycle is one machine's view of the archive.
type Cycle struct {
	Machine    string          // this machine's name
	Mirror     string          // mirror root
	Sources    []config.Source // this machine's tools
	Readable   []string        // other machines with a crypt remote on this machine
	Runner     *rclone.Runner
	StatusPath string
	Log        *runlog.Log
	Now        func() time.Time
	Errors     io.Writer // failures are echoed here for a person; io.Discard under the timer
}

// Run is the full cycle. Steps fail independently, so every step runs whatever came before.
func (c *Cycle) Run(ctx context.Context) bool {
	plugins := c.storePlugins()
	pushed := c.Push(ctx)
	pulled := c.Pull(ctx)
	return plugins && pushed && pulled
}

// Push copies this machine's records into the mirror and uploads that copy. The upload never
// reads a file an agent is writing; only the local copy can meet one.
func (c *Cycle) Push(ctx context.Context) bool {
	copied := c.localCopy(ctx)
	pushed := c.push(ctx)
	return copied && pushed
}

// Pull brings every other readable machine into the mirror.
func (c *Cycle) Pull(ctx context.Context) bool {
	ok := true
	for _, machine := range c.Readable {
		// The mirror root is created here as well as in the local copy: a pull can be the
		// first thing a machine ever does, and the mirror holds decrypted transcripts.
		if err := os.MkdirAll(c.Mirror, 0o700); err != nil {
			ok = c.record("pull:"+machine, rclone.Failure, err.Error()) && ok
			continue
		}
		res := c.Runner.Copy(ctx, config.CryptRemote(machine), filepath.Join(c.Mirror, machine))
		ok = c.record("pull:"+machine, res.Outcome, res.Output) && ok
	}
	return ok
}

func (c *Cycle) machineDir() string { return filepath.Join(c.Mirror, c.Machine) }

func (c *Cycle) storePlugins() bool {
	var failures []string
	for _, s := range c.Sources {
		if s.PluginManifest == "" {
			continue
		}
		manifest := filepath.Join(s.Root, filepath.FromSlash(s.PluginManifest))
		dest := plugintimeline.DestDir(filepath.Join(c.machineDir(), s.Tool), s.PluginManifest)
		if _, err := plugintimeline.Store(manifest, dest, c.Now()); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", s.Tool, err))
		}
	}
	if len(failures) > 0 {
		return c.record("plugins", rclone.Failure, strings.Join(failures, "\n"))
	}
	return c.record("plugins", rclone.Success, "")
}

func (c *Cycle) localCopy(ctx context.Context) bool {
	if err := os.MkdirAll(c.machineDir(), 0o700); err != nil {
		return c.record("local-copy", rclone.Failure, err.Error())
	}
	worst := rclone.Success
	var details []string
	for _, s := range c.Sources {
		for _, p := range s.Paths {
			rel := filepath.FromSlash(p)
			res := c.Runner.Copy(ctx, filepath.Join(s.Root, rel), filepath.Join(c.machineDir(), s.Tool, rel))
			if res.Outcome > worst {
				worst = res.Outcome
			}
			if res.Output != "" {
				details = append(details, fmt.Sprintf("%s/%s:\n%s", s.Tool, p, res.Output))
			}
		}
	}
	return c.record("local-copy", worst, strings.Join(details, "\n"))
}

func (c *Cycle) push(ctx context.Context) bool {
	// The folder must exist even when the local copy failed, so that push reports its own
	// outcome rather than "directory not found".
	if err := os.MkdirAll(c.machineDir(), 0o700); err != nil {
		return c.record("push", rclone.Failure, err.Error())
	}
	res := c.Runner.Copy(ctx, c.machineDir(), config.CryptRemote(c.Machine))
	return c.record("push", res.Outcome, res.Output)
}

// record writes a step's outcome to the log and to status.json, echoes a failure for a person
// watching, and reports whether the step succeeded. A warning is a success: the files it left
// behind are copied by the next run.
func (c *Cycle) record(step string, outcome rclone.Outcome, detail string) bool {
	now := c.Now()
	succeeded := outcome != rclone.Failure
	ok := succeeded

	if err := c.Log.Append(now, step, outcome.String(), detail); err != nil {
		_, _ = fmt.Fprintf(c.Errors, "cannot write the log: %v\n", err)
		ok = false
	}
	st, err := status.Load(c.StatusPath)
	if err == nil {
		st.Record(step, now, succeeded, detail)
		err = st.Save(c.StatusPath)
	}
	if err != nil {
		_, _ = fmt.Fprintf(c.Errors, "cannot update the status file: %v\n", err)
		ok = false
	}
	if !succeeded {
		_, _ = fmt.Fprintf(c.Errors, "%s FAILED\n", step)
		for _, line := range strings.Split(strings.TrimRight(detail, "\n"), "\n") {
			_, _ = fmt.Fprintf(c.Errors, "    %s\n", line)
		}
	}
	return ok
}
