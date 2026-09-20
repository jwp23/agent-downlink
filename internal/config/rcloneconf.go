package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	storageRemote = "b2"
	cryptPrefix   = "crypt-"
)

// B2 is the storage key.
type B2 struct{ Account, Key string }

// RcloneConf is the tool's own rclone.conf. Passwords are in rclone's obscured form.
type RcloneConf struct {
	B2        *B2
	Passwords map[string]string
}

// CryptRemote is the rclone path of a machine's decrypted area.
func CryptRemote(machine string) string { return cryptPrefix + machine + ":" }

// Machines lists every machine this rclone.conf can decrypt, sorted.
func (c RcloneConf) Machines() []string {
	machines := make([]string, 0, len(c.Passwords))
	for m := range c.Passwords {
		machines = append(machines, m)
	}
	sort.Strings(machines)
	return machines
}

// LoadRcloneConf reads the file Save wrote. Errors name a line number and never quote the
// file, because every value in it is a secret.
func LoadRcloneConf(path string) (RcloneConf, error) {
	f, err := os.Open(path)
	if err != nil {
		return RcloneConf{}, err
	}
	defer func() { _ = f.Close() }()

	conf := RcloneConf{Passwords: map[string]string{}}
	section := ""
	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = line[1 : len(line)-1]
			if section == storageRemote {
				conf.B2 = &B2{}
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section == "" {
			return RcloneConf{}, fmt.Errorf("%s: line %d is not a section header or a key = value pair", path, n)
		}
		conf.setField(section, strings.TrimSpace(key), strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return RcloneConf{}, fmt.Errorf("%s: %w", path, err)
	}
	return conf, nil
}

// setField records one key = value line from the section it appeared in.
func (c *RcloneConf) setField(section, key, value string) {
	switch {
	case section == storageRemote && key == "account":
		c.B2.Account = value
	case section == storageRemote && key == "key":
		c.B2.Key = value
	case strings.HasPrefix(section, cryptPrefix) && key == "password":
		c.Passwords[strings.TrimPrefix(section, cryptPrefix)] = value
	}
}

// Save writes rclone.conf for the given storage location (config.File.Storage). It writes a
// private temporary file and renames it into place, so a crash never leaves a torn file of
// secrets and the file is never briefly readable by others.
func (c RcloneConf) Save(path, storage string) error {
	var b strings.Builder
	if c.B2 != nil {
		fmt.Fprintf(&b, "[%s]\ntype = b2\naccount = %s\nkey = %s\n", storageRemote, c.B2.Account, c.B2.Key)
	}
	for _, machine := range c.Machines() {
		if err := ValidateMachineName(machine); err != nil {
			return err
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[%s%s]\ntype = crypt\nremote = %s/%s\npassword = %s\n", cryptPrefix, machine, storage, machine, c.Passwords[machine])
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".rclone.conf-*") // CreateTemp makes the file 0600
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // a no-op once the rename has happened
	if _, err := tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// CheckPermissions refuses to proceed when the secrets are readable by anyone but the owner,
// the same rule ssh applies to private keys.
func CheckPermissions(p Paths) error {
	checks := []struct {
		path string
		fix  string
	}{
		{p.ConfigDir, "chmod 700"},
		{p.RcloneConf, "chmod 600"},
	}
	for _, check := range checks {
		info, err := os.Stat(check.path)
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s does not exist; run 'agent-downlink setup' first", check.path)
		}
		if err != nil {
			return err
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return fmt.Errorf("%s has mode %04o, which lets other users read this machine's secrets; fix it with: %s %s", check.path, perm, check.fix, check.path)
		}
	}
	return nil
}
