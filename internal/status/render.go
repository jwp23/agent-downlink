package status

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Render is the report printed by "agent-downlink status". It shows ages and leaves the
// judgment to the operator: a machine that is off looks like a machine that is broken.
func Render(f File, now time.Time, logPath string) string {
	var b strings.Builder
	if len(f.Steps) == 0 {
		b.WriteString("No runs recorded yet.\n")
	} else {
		names := sortedStepNames(f.Steps)
		writeStepTable(&b, f.Steps, names, now)
		writeStepErrors(&b, f.Steps, names)
	}
	fmt.Fprintf(&b, "\nLog: %s\n", logPath)
	return b.String()
}

func sortedStepNames(steps map[string]Step) []string {
	names := make([]string, 0, len(steps))
	for name := range steps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// writeStepTable writes the STEP/LAST SUCCESS/STATE table for the named steps, in order.
func writeStepTable(b *strings.Builder, steps map[string]Step, names []string, now time.Time) {
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "STEP\tLAST SUCCESS\tSTATE")
	for _, name := range names {
		s := steps[name]
		last, state := "never", "ok"
		if !s.LastSuccess.IsZero() {
			last = age(now.Sub(s.LastSuccess))
		}
		if s.Error != "" {
			state = "FAILING"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", name, last, state)
	}
	_ = tw.Flush()
}

// writeStepErrors writes each failing step's stored error, indented, separated from the table
// by one blank line.
func writeStepErrors(b *strings.Builder, steps map[string]Step, names []string) {
	separated := false
	for _, name := range names {
		s := steps[name]
		if s.Error == "" {
			continue
		}
		if !separated {
			b.WriteString("\n") // one blank line between the table and the errors
			separated = true
		}
		fmt.Fprintf(b, "%s:\n", name)
		for _, line := range strings.Split(s.Error, "\n") {
			fmt.Fprintf(b, "    %s\n", line)
		}
	}
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
