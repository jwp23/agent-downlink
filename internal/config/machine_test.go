package config

import (
	"strings"
	"testing"
)

func TestValidateMachineName(t *testing.T) {
	for _, name := range []string{"workstation", "laptop", "my-laptop-2", "a", "9"} {
		if err := ValidateMachineName(name); err != nil {
			t.Errorf("ValidateMachineName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{"", "Workstation", "my_laptop", "-laptop", "laptop-", "lap top", "laptop/x", "laptop.local", "läptop"} {
		err := ValidateMachineName(name)
		if err == nil {
			t.Errorf("ValidateMachineName(%q) = nil, want an error", name)
			continue
		}
		if !strings.Contains(err.Error(), "lowercase letters, digits, and hyphens") {
			t.Errorf("ValidateMachineName(%q) error = %q, want it to state the rule", name, err)
		}
	}
}

func TestDefaultMachineName(t *testing.T) {
	cases := map[string]string{
		"workstation":  "workstation",
		"Laptop.local": "laptop",
		"My_Laptop 2":  "my-laptop-2",
		"--odd--":      "odd",
		"":             "",
		"...":          "",
	}
	for hostname, want := range cases {
		if got := DefaultMachineName(hostname); got != want {
			t.Errorf("DefaultMachineName(%q) = %q, want %q", hostname, got, want)
		}
		if got := DefaultMachineName(hostname); got != "" && ValidateMachineName(got) != nil {
			t.Errorf("DefaultMachineName(%q) = %q, which is not a valid machine name", hostname, got)
		}
	}
}
