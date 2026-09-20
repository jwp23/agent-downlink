package rclone

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Outcome is how a copy ended.
type Outcome int

const (
	Success Outcome = iota
	// Warning means the only files left behind were being written while rclone read them.
	// The next run copies them.
	Warning
	Failure
)

func (o Outcome) String() string {
	switch o {
	case Success:
		return "ok"
	case Warning:
		return "warning"
	default:
		return "FAILED"
	}
}

// Result is the outcome of one rclone copy.
type Result struct {
	Outcome  Outcome
	ExitCode int
	Output   string
}

// sourceChanging is how rclone's local backend reports a file that changed while being read.
const sourceChanging = "source file is being updated"

type logLine struct {
	Level  string `json:"level"`
	Msg    string `json:"msg"`
	Object string `json:"object"`
}

// classify turns rclone's exit status and --use-json-log stderr into a Result. rclone has no
// exit status of its own for "a source file changed", so the error lines decide: if every one
// of them is about a changing source file, the copy only needs repeating.
func classify(exitCode int, stderr []byte) Result {
	out, errorLines, changingLines := scanRcloneLog(stderr)

	result := Result{ExitCode: exitCode, Output: strings.Join(out, "\n")}
	switch {
	case exitCode == 0:
		result.Outcome = Success
	case errorLines > 0 && errorLines == changingLines:
		result.Outcome = Warning
	default:
		result.Outcome = Failure
		if result.Output == "" {
			result.Output = fmt.Sprintf("rclone exited with status %d", exitCode)
		}
	}
	return result
}

// scanRcloneLog reads rclone's --use-json-log stderr, one line per rendered message, and
// counts how many were error-level and how many of those were about a changing source file.
func scanRcloneLog(stderr []byte) (out []string, errorLines, changingLines int) {
	scanner := bufio.NewScanner(bytes.NewReader(stderr))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var line logLine
		if err := json.Unmarshal([]byte(raw), &line); err != nil || line.Msg == "" {
			out = append(out, raw)
			continue
		}
		if line.Level == "error" || line.Level == "critical" {
			errorLines++
			if strings.Contains(line.Msg, sourceChanging) {
				changingLines++
			}
		}
		rendered := strings.ToUpper(line.Level) + ": "
		if line.Object != "" {
			rendered += line.Object + ": "
		}
		out = append(out, rendered+line.Msg)
	}
	return out, errorLines, changingLines
}
