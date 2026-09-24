package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestFindViolationsAllKilledIsClean(t *testing.T) {
	report := `{"files":[{"file_name":"internal/x/y.go","mutations":[
		{"type":"CONDITIONALS_BOUNDARY","status":"KILLED","line":10,"column":5}
	]}]}`
	violations, err := findViolations([]byte(report), nil)
	if err != nil {
		t.Fatalf("findViolations: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %v, want none", violations)
	}
}

// TestFindViolationsReportsLivedTimedOutAndNotViable pins ADR-005's bar: those three
// statuses are gaps; NOT COVERED is not, because gremlins never ran that mutant.
func TestFindViolationsReportsLivedTimedOutAndNotViable(t *testing.T) {
	report := `{"files":[{"file_name":"internal/x/y.go","mutations":[
		{"type":"CONDITIONALS_BOUNDARY","status":"KILLED","line":10,"column":5},
		{"type":"CONDITIONALS_NEGATION","status":"LIVED","line":11,"column":6},
		{"type":"ARITHMETIC_BASE","status":"NOT COVERED","line":12,"column":7},
		{"type":"CONDITIONALS_NEGATION","status":"TIMED OUT","line":13,"column":8},
		{"type":"INVERT_NEGATIVES","status":"NOT VIABLE","line":14,"column":9}
	]}]}`
	violations, err := findViolations([]byte(report), nil)
	if err != nil {
		t.Fatalf("findViolations: %v", err)
	}
	if len(violations) != 3 {
		t.Fatalf("violations = %v, want 3 (LIVED, TIMED OUT, NOT VIABLE)", violations)
	}
	for _, want := range []string{"LIVED", "TIMED OUT", "NOT VIABLE"} {
		found := false
		for _, v := range violations {
			if v.Status == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing violation for status %q", want)
		}
	}
}

func TestFindViolationsRejectsMalformedJSON(t *testing.T) {
	if _, err := findViolations([]byte("not json"), nil); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

// A report with no files means gremlins never ran, not that it found nothing.
func TestFindViolationsRejectsReportWithNoFiles(t *testing.T) {
	if _, err := findViolations([]byte(`{"files":[]}`), nil); err == nil {
		t.Fatal("expected an error for a report with no files")
	}
}

// A mutation with no type, line, or column carries no identity: the gate cannot say what
// was tested, so a report containing one is rejected outright rather than silently counted
// as clean.
func TestFindViolationsRejectsMutationMissingIdentityFields(t *testing.T) {
	report := `{"files":[{"file_name":"a.go","mutations":[{"status":"KILLED"}]}]}`
	if _, err := findViolations([]byte(report), nil); err == nil {
		t.Fatal("expected an error for a mutation missing type, line, and column")
	}
}

func TestFindViolationsRejectsFileMissingFileName(t *testing.T) {
	report := `{"files":[{"mutations":[{"type":"CONDITIONALS_BOUNDARY","status":"KILLED","line":1,"column":1}]}]}`
	if _, err := findViolations([]byte(report), nil); err == nil {
		t.Fatal("expected an error for a file entry missing file_name")
	}
}

func TestFindViolationsRejectsReportWithNoMutations(t *testing.T) {
	report := `{"files":[{"file_name":"a.go","mutations":[]},{"file_name":"b.go","mutations":[]}]}`
	if _, err := findViolations([]byte(report), nil); err == nil {
		t.Fatal("expected an error for a report where every file has zero mutations")
	}
}

func TestFindViolationsSkipsReviewedEquivalents(t *testing.T) {
	report := `{"files":[{"file_name":"a.go","mutations":[
		{"type":"CONDITIONALS_BOUNDARY","status":"LIVED","line":10,"column":5},
		{"type":"CONDITIONALS_BOUNDARY","status":"LIVED","line":20,"column":9}
	]}]}`
	equivalents := []equivalent{
		{File: "a.go", Line: 10, Column: 5, Type: "CONDITIONALS_BOUNDARY", Reason: "proven equivalent"},
	}
	violations, err := findViolations([]byte(report), equivalents)
	if err != nil {
		t.Fatalf("findViolations: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 20 {
		t.Fatalf("violations = %v, want exactly the unreviewed one at line 20", violations)
	}
}

// gremlins scoped to one package directory reports a bare file name; the allowlist records
// module-relative paths. Both must match, on a path boundary.
func TestFindViolationsMatchesEquivalentScopedToASubPackage(t *testing.T) {
	report := `{"files":[{"file_name":"cycle.go","mutations":[
		{"type":"CONDITIONALS_BOUNDARY","status":"LIVED","line":124,"column":19}
	]}]}`
	equivalents := []equivalent{
		{File: "internal/transfer/cycle.go", Line: 124, Column: 19, Type: "CONDITIONALS_BOUNDARY", Reason: "proven equivalent"},
	}
	violations, err := findViolations([]byte(report), equivalents)
	if err != nil {
		t.Fatalf("findViolations: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("violations = %v, want none", violations)
	}
}

func TestFindViolationsEquivalentSuffixMatchRespectsPathBoundaries(t *testing.T) {
	report := `{"files":[{"file_name":"my_cycle.go","mutations":[
		{"type":"CONDITIONALS_BOUNDARY","status":"LIVED","line":124,"column":19}
	]}]}`
	equivalents := []equivalent{
		{File: "internal/transfer/cycle.go", Line: 124, Column: 19, Type: "CONDITIONALS_BOUNDARY", Reason: "proven equivalent"},
	}
	violations, err := findViolations([]byte(report), equivalents)
	if err != nil {
		t.Fatalf("findViolations: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations = %v, want the my_cycle.go survivor kept", violations)
	}
}

func TestFindViolationsEquivalentMustMatchTypeToo(t *testing.T) {
	report := `{"files":[{"file_name":"a.go","mutations":[
		{"type":"CONDITIONALS_NEGATION","status":"LIVED","line":10,"column":5}
	]}]}`
	equivalents := []equivalent{
		{File: "a.go", Line: 10, Column: 5, Type: "CONDITIONALS_BOUNDARY", Reason: "proven equivalent"},
	}
	violations, err := findViolations([]byte(report), equivalents)
	if err != nil {
		t.Fatalf("findViolations: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations = %v, want the CONDITIONALS_NEGATION survivor kept", violations)
	}
}

func TestLoadEquivalentsFromFile(t *testing.T) {
	path := t.TempDir() + "/equivalents.json"
	body := `[{"file":"a.go","line":10,"column":5,"type":"CONDITIONALS_BOUNDARY","reason":"proven equivalent"}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadEquivalents(path)
	if err != nil {
		t.Fatalf("loadEquivalents: %v", err)
	}
	want := equivalent{File: "a.go", Line: 10, Column: 5, Type: "CONDITIONALS_BOUNDARY", Reason: "proven equivalent"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("loadEquivalents = %v, want %v", got, want)
	}
}

func TestLoadEquivalentsRejectsEmptyReason(t *testing.T) {
	path := t.TempDir() + "/equivalents.json"
	body := `[{"file":"a.go","line":10,"column":5,"type":"CONDITIONALS_BOUNDARY","reason":"   "}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEquivalents(path); err == nil {
		t.Fatal("expected an error for a whitespace-only reason")
	}
}

func TestLoadEquivalentsEmptyPathIsNoEquivalents(t *testing.T) {
	got, err := loadEquivalents("")
	if err != nil || len(got) != 0 {
		t.Fatalf("loadEquivalents(\"\") = %v, %v; want none", got, err)
	}
}

func TestLoadEquivalentsRejectsDuplicateBasenameWithDifferentPaths(t *testing.T) {
	path := t.TempDir() + "/equivalents.json"
	body := `[
		{"file":"pkg1/x.go","line":10,"column":5,"type":"CONDITIONALS_BOUNDARY","reason":"first"},
		{"file":"pkg2/x.go","line":20,"column":6,"type":"CONDITIONALS_BOUNDARY","reason":"second"}
	]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadEquivalents(path)
	if err == nil {
		t.Fatal("expected an error for duplicate basename with different paths")
	}
	if !strings.Contains(err.Error(), "x.go") {
		t.Fatalf("error should mention the conflicting basename, got: %v", err)
	}
}

func TestLoadEquivalentsMultipleDistinctBasenames(t *testing.T) {
	path := t.TempDir() + "/equivalents.json"
	body := `[
		{"file":"pkg1/a.go","line":10,"column":5,"type":"CONDITIONALS_BOUNDARY","reason":"first"},
		{"file":"pkg2/b.go","line":20,"column":6,"type":"CONDITIONALS_BOUNDARY","reason":"second"},
		{"file":"pkg3/c.go","line":30,"column":7,"type":"CONDITIONALS_NEGATION","reason":"third"}
	]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadEquivalents(path)
	if err != nil {
		t.Fatalf("loadEquivalents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("loadEquivalents returned %d entries, want 3", len(got))
	}
}

func TestLoadEquivalentsRealFileStillLoads(t *testing.T) {
	// Verify the real .gremlins-equivalents.json (internal/rclone/classify.go and
	// internal/transfer/cycle.go) loads without error - basenames are distinct.
	got, err := loadEquivalents("../../.gremlins-equivalents.json")
	if err != nil {
		t.Fatalf("loadEquivalents real file: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("real .gremlins-equivalents.json has %d entries, want 2", len(got))
	}
	// Verify the entries have the expected files
	files := make(map[string]bool)
	for _, e := range got {
		files[e.File] = true
	}
	if !files["internal/rclone/classify.go"] {
		t.Fatal("expected internal/rclone/classify.go in real file")
	}
	if !files["internal/transfer/cycle.go"] {
		t.Fatal("expected internal/transfer/cycle.go in real file")
	}
}

func writeReport(t *testing.T, report string) string {
	t.Helper()
	path := t.TempDir() + "/report.json"
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunExitsNonZeroOnAnySurvivor(t *testing.T) {
	path := writeReport(t, `{"files":[{"file_name":"a.go","mutations":[{"type":"CONDITIONALS_BOUNDARY","status":"LIVED","line":1,"column":1}]}]}`)
	var stdout, stderr bytes.Buffer
	if code := run(path, "", &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "a.go:1:1 CONDITIONALS_BOUNDARY LIVED") {
		t.Fatalf("stdout = %q, want it to name the surviving mutant", stdout.String())
	}
}

func TestRunExitsZeroWhenClean(t *testing.T) {
	path := writeReport(t, `{"files":[{"file_name":"a.go","mutations":[{"type":"CONDITIONALS_BOUNDARY","status":"KILLED","line":1,"column":1}]}]}`)
	var stdout, stderr bytes.Buffer
	if code := run(path, "", &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
}

// NOT COVERED is information: the count is printed and the exit status stays 0.
func TestRunReportsNotCoveredWithoutFailing(t *testing.T) {
	path := writeReport(t, `{"files":[{"file_name":"a.go","mutations":[
		{"type":"CONDITIONALS_BOUNDARY","status":"KILLED","line":1,"column":1},
		{"type":"CONDITIONALS_NEGATION","status":"NOT COVERED","line":2,"column":1},
		{"type":"CONDITIONALS_NEGATION","status":"NOT COVERED","line":3,"column":1}
	]}]}`)
	var stdout, stderr bytes.Buffer
	if code := run(path, "", &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 mutant(s) not covered") {
		t.Fatalf("stdout = %q, want the not-covered count", stdout.String())
	}
}

func TestRunReportsErrorForMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run("/does/not/exist.json", "", &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("expected an error message on stderr")
	}
}

func TestRunExitsZeroWhenOnlySurvivorIsAReviewedEquivalent(t *testing.T) {
	reportPath := writeReport(t, `{"files":[{"file_name":"a.go","mutations":[{"type":"CONDITIONALS_BOUNDARY","status":"LIVED","line":1,"column":1}]}]}`)
	equivPath := t.TempDir() + "/equivalents.json"
	equiv := `[{"file":"a.go","line":1,"column":1,"type":"CONDITIONALS_BOUNDARY","reason":"proven equivalent"}]`
	if err := os.WriteFile(equivPath, []byte(equiv), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(reportPath, equivPath, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 known-equivalent") {
		t.Fatalf("stdout = %q, want it to say the equivalent was excluded, not hidden", stdout.String())
	}
}
