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
		names := make([]string, 0, len(f.Steps))
		for name := range f.Steps {
			names = append(names, name)
		}
		sort.Strings(names)

		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "STEP\tLAST SUCCESS\tSTATE")
		for _, name := range names {
			s := f.Steps[name]
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

		separated := false
		for _, name := range names {
			if s := f.Steps[name]; s.Error != "" {
				if !separated {
					b.WriteString("\n") // one blank line between the table and the errors
					separated = true
				}
				fmt.Fprintf(&b, "%s:\n", name)
				for _, line := range strings.Split(s.Error, "\n") {
					fmt.Fprintf(&b, "    %s\n", line)
				}
			}
		}
	}
	fmt.Fprintf(&b, "\nLog: %s\n", logPath)
	return b.String()
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
