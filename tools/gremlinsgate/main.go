// Command gremlinsgate enforces the mutation-testing bar ADR-005 sets: zero unexcluded
// LIVED, TIMED OUT or NOT VIABLE mutants. gremlins itself only fails a run on percentage
// thresholds; ADR-005 sets none, since each of those statuses is a real gap to triage
// rather than a rate to average against. NOT COVERED mutants are reported as information:
// gremlins never runs them, so they say nothing about test strength, and statement
// coverage is measured elsewhere.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type mutation struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type fileReport struct {
	FileName  string     `json:"file_name"`
	Mutations []mutation `json:"mutations"`
}

type report struct {
	Files []fileReport `json:"files"`
}

type violation struct {
	File   string
	Type   string
	Status string
	Line   int
	Column int
}

// equivalent names one mutant reviewed and proven equivalent: no test can ever distinguish
// it from the original code, so it is not a gap. gremlins only excludes whole files
// (exclude-files in .gremlins.yaml); this allowlist covers a single mutant excluded on its
// own reviewed merits, in a file whose other mutants are real coverage.
type equivalent struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

const (
	statusKilled     = "KILLED"
	statusNotCovered = "NOT COVERED"
)

func main() {
	reportPath := flag.String("report", "", "path to gremlins JSON report (produced by unleash -o)")
	equivalentsPath := flag.String("equivalents", "", "optional path to a reviewed-equivalents JSON allowlist")
	flag.Parse()
	if *reportPath == "" {
		fmt.Fprintln(os.Stderr, "gremlinsgate: -report is required")
		os.Exit(2)
	}
	os.Exit(run(*reportPath, *equivalentsPath, os.Stdout, os.Stderr))
}

func run(reportPath, equivalentsPath string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	equivalents, err := loadEquivalents(equivalentsPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	violations, err := findViolations(data, equivalents)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if notCovered := countStatus(data, statusNotCovered); notCovered > 0 {
		_, _ = fmt.Fprintf(stdout, "gremlinsgate: %d mutant(s) not covered (information only; see ADR-005)\n", notCovered)
	}
	if excluded := countExcludedEquivalents(data, equivalents); excluded > 0 {
		_, _ = fmt.Fprintf(stdout, "gremlinsgate: %d known-equivalent mutant(s) excluded (reviewed, see -equivalents)\n", excluded)
	}
	if len(violations) == 0 {
		_, _ = fmt.Fprintln(stdout, "gremlinsgate: no unexcluded survivors")
		return 0
	}
	_, _ = fmt.Fprintf(stdout, "gremlinsgate: %d unexcluded mutant(s) not killed:\n", len(violations))
	for _, v := range violations {
		_, _ = fmt.Fprintf(stdout, "  %s:%d:%d %s %s\n", v.File, v.Line, v.Column, v.Type, v.Status)
	}
	return 1
}

// isGap is true for the statuses the gate fails on. KILLED is the goal and NOT COVERED is
// information; everything else (LIVED, TIMED OUT, NOT VIABLE) is a gap.
func isGap(m mutation) bool {
	return m.Status != statusKilled && m.Status != statusNotCovered
}

// findViolations reports every gap mutation, minus any reviewed equivalents. Files gremlins
// never mutated at all (.gremlins.yaml's exclude-files) do not appear in the report.
func findViolations(data []byte, equivalents []equivalent) ([]violation, error) {
	r, err := parseReport(data)
	if err != nil {
		return nil, err
	}
	var violations []violation
	for _, f := range r.Files {
		for _, m := range f.Mutations {
			if !isGap(m) || isReviewedEquivalent(f.FileName, m, equivalents) {
				continue
			}
			violations = append(violations, violation{
				File:   f.FileName,
				Type:   m.Type,
				Status: m.Status,
				Line:   m.Line,
				Column: m.Column,
			})
		}
	}
	return violations, nil
}

// countExcludedEquivalents reports how many gap mutants the allowlist actually matched, so
// a stale entry (its mutant got killed by an unrelated test change, or never existed) is
// visibly worth zero rather than silently claimed.
func countExcludedEquivalents(data []byte, equivalents []equivalent) int {
	r, err := parseReport(data)
	if err != nil {
		return 0
	}
	count := 0
	for _, f := range r.Files {
		for _, m := range f.Mutations {
			if isGap(m) && isReviewedEquivalent(f.FileName, m, equivalents) {
				count++
			}
		}
	}
	return count
}

func countStatus(data []byte, status string) int {
	r, err := parseReport(data)
	if err != nil {
		return 0
	}
	count := 0
	for _, f := range r.Files {
		for _, m := range f.Mutations {
			if m.Status == status {
				count++
			}
		}
	}
	return count
}

// isReviewedEquivalent matches an allowlist entry's file (recorded module-relative, e.g.
// "internal/transfer/cycle.go") against the report's file_name, which gremlins writes
// relative to the path it was invoked with: module-relative when scoped to ".", bare
// ("cycle.go") when scoped to one package directory. An exact match or a "/"-boundary
// suffix match covers both; the boundary keeps "my_cycle.go" from matching "cycle.go".
func isReviewedEquivalent(fileName string, m mutation, equivalents []equivalent) bool {
	for _, e := range equivalents {
		if fileMatches(e.File, fileName) && e.Line == m.Line && e.Column == m.Column && e.Type == m.Type {
			return true
		}
	}
	return false
}

func fileMatches(recorded, reported string) bool {
	return recorded == reported || strings.HasSuffix(recorded, "/"+reported)
}

func parseReport(data []byte) (report, error) {
	var r report
	if err := json.Unmarshal(data, &r); err != nil {
		return report{}, fmt.Errorf("parse gremlins report: %w", err)
	}
	if len(r.Files) == 0 {
		return report{}, fmt.Errorf("parse gremlins report: no files in report")
	}
	for _, f := range r.Files {
		if len(f.Mutations) > 0 {
			return r, nil
		}
	}
	return report{}, fmt.Errorf("parse gremlins report: no mutations in report")
}

// loadEquivalents reads the reviewed-equivalents allowlist. An empty path means no
// allowlist was configured, not an error.
func loadEquivalents(path string) ([]equivalent, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read equivalents file: %w", err)
	}
	var equivalents []equivalent
	if err := json.Unmarshal(data, &equivalents); err != nil {
		return nil, fmt.Errorf("parse equivalents file: %w", err)
	}
	for _, e := range equivalents {
		if strings.TrimSpace(e.Reason) == "" {
			return nil, fmt.Errorf("parse equivalents file: %s:%d:%d %s has an empty reason", e.File, e.Line, e.Column, e.Type)
		}
	}
	return equivalents, nil
}
