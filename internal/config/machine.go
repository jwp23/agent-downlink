package config

import (
	"fmt"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidateMachineName enforces the naming rule. The name becomes a bucket folder and an
// rclone remote name, and is the one thing the storage provider sees in plaintext.
func ValidateMachineName(name string) error {
	return validateName("machine name", name)
}

func validateName(kind, name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid %s %q: use lowercase letters, digits, and hyphens, and start and end with a letter or digit", kind, name)
	}
	return nil
}

var notNameChar = regexp.MustCompile(`[^a-z0-9]+`)

// DefaultMachineName turns a hostname into a valid machine name to offer as the default.
// It returns "" when nothing usable remains.
func DefaultMachineName(hostname string) string {
	short, _, _ := strings.Cut(strings.ToLower(hostname), ".")
	return strings.Trim(notNameChar.ReplaceAllString(short, "-"), "-")
}
