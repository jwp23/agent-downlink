package setup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/rclone"
)

// Options are what the setup flow needs from its caller.
type Options struct {
	Home         string
	Hostname     string // offered, cleaned up, as the default machine name
	NoTimer      bool
	InstallTimer func() error // called once unless NoTimer; required unless NoTimer
}

var errCancelled = errors.New("cancelled; nothing was changed")

// Run is the once-per-machine setup: it asks for everything the tool needs, writes both
// configuration files, and installs the hourly schedule.
func Run(ctx context.Context, p *Prompter, o Options) error {
	paths := config.PathsFor(o.Home)
	runner, err := rclone.New("", paths.RcloneConf, rclone.Concurrency{}) // setup only obscures a password
	if err != nil {
		return err
	}
	cfg, secrets, err := carriedOver(p, paths)
	if err != nil {
		return err
	}

	if cfg.Machine, err = askMachine(p, o.Hostname); err != nil {
		return err
	}
	bucket, err := askBucket(p)
	if err != nil {
		return err
	}
	cfg.Storage, cfg.Rclone = "b2:"+bucket, runner.Binary()
	if secrets.B2, err = askStorageKey(p); err != nil {
		return err
	}
	password, generated, err := askPassword(p)
	if err != nil {
		return err
	}
	if secrets.Passwords[cfg.Machine], err = runner.Obscure(ctx, password); err != nil {
		return err
	}

	if err := replaceConfigPair(paths, cfg, secrets); err != nil {
		return err
	}

	report(p, paths, cfg.Machine, password, generated)
	return installSchedule(p, o)
}

// replaceConfigPair writes both configuration files. If rclone.conf fails to save after
// config.toml already has, config.toml is rolled back, so a later command never sees
// config.toml naming a new machine or storage while rclone.conf still holds the previous
// one's credentials.
func replaceConfigPair(paths config.Paths, cfg config.File, secrets config.RcloneConf) error {
	previous, hadPrevious, err := readIfExists(paths.ConfigFile)
	if err != nil {
		return err
	}
	if err := cfg.Save(paths.ConfigFile); err != nil {
		return err
	}
	if err := secrets.Save(paths.RcloneConf, cfg.Storage); err != nil {
		if restoreErr := restorePrevious(paths.ConfigFile, previous, hadPrevious); restoreErr != nil {
			return fmt.Errorf("%w (and the previous config.toml could not be restored: %v)", err, restoreErr)
		}
		return err
	}
	return nil
}

func readIfExists(path string) (data []byte, existed bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func restorePrevious(path string, data []byte, existed bool) error {
	if !existed {
		return os.Remove(path)
	}
	return os.WriteFile(path, data, 0o600)
}

// carriedOver is the configuration the answers are filled into: the defaults on a fresh
// machine, or, once the operator confirms a re-run, the settings the existing files keep.
// Those are the mirror path, the tools, and the other machines this one can already read.
func carriedOver(p *Prompter, paths config.Paths) (config.File, config.RcloneConf, error) {
	cfg := config.File{Mirror: paths.DefaultMirror, Tools: []string{"claude-code"}}
	secrets := config.RcloneConf{Passwords: map[string]string{}}
	if !exists(paths.ConfigFile) && !exists(paths.RcloneConf) {
		return cfg, secrets, nil
	}

	_, _ = fmt.Fprintf(p.out, "This machine is already set up (%s).\n"+
		"Continuing replaces its machine name, storage location, storage key, and encryption password.\n", paths.ConfigDir)
	if err := confirmReplacement(p, "Replace them?"); err != nil {
		return cfg, secrets, err
	}
	if old, err := config.Load(paths.ConfigFile); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return cfg, secrets, err
		}
	} else {
		cfg.Mirror, cfg.Tools, cfg.CustomTools = old.Mirror, old.Tools, old.CustomTools
	}
	if old, err := config.LoadRcloneConf(paths.RcloneConf); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return cfg, secrets, err
		}
	} else {
		secrets.Passwords = old.Passwords
	}
	return cfg, secrets, nil
}

// confirmReplacement returns errCancelled unless the operator types the whole word yes.
func confirmReplacement(p *Prompter, question string) error {
	ok, err := p.Confirm(question)
	if err != nil {
		return err
	}
	if !ok {
		return errCancelled
	}
	return nil
}

func askMachine(p *Prompter, hostname string) (string, error) {
	machine, err := p.Ask("Machine name (lowercase letters, digits, hyphens; visible to the storage provider)", config.DefaultMachineName(hostname))
	if err != nil {
		return "", err
	}
	return machine, config.ValidateMachineName(machine)
}

func askBucket(p *Prompter) (string, error) {
	bucket, err := p.Ask("Bucket name", "")
	if err != nil {
		return "", err
	}
	if bucket == "" || strings.ContainsAny(bucket, "/: \t") {
		return "", fmt.Errorf("invalid bucket name %q: enter the bucket's name alone, with no path", bucket)
	}
	return bucket, nil
}

func askStorageKey(p *Prompter) (*config.B2, error) {
	keyID, err := p.Ask("Storage key ID", "")
	if err != nil {
		return nil, err
	}
	if keyID == "" {
		return nil, errors.New("the storage key ID is empty")
	}
	key, err := p.AskSecret("Storage application key")
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, errors.New("the storage application key is empty")
	}
	return &config.B2{Account: keyID, Key: key}, nil
}

// askPassword returns the machine's encryption password and whether it was generated here,
// which is the only case in which the operator has not seen it yet.
func askPassword(p *Prompter) (password string, generated bool, err error) {
	password, err = p.AskSecret("Encryption password (press Enter to generate one; enter the existing one when replacing a machine of the same name)")
	if err != nil || password != "" {
		return password, false, err
	}
	password, err = generatePassword()
	return password, true, err
}

// report tells the operator what was written and what only they can keep.
func report(p *Prompter, paths config.Paths, machine, password string, generated bool) {
	_, _ = fmt.Fprintf(p.out, "\nConfiguration written to %s\n", paths.ConfigDir)
	if generated {
		_, _ = fmt.Fprintf(p.out, "\nSave these in your password manager now. The encryption password is not shown again,\n"+
			"and without it this machine's records cannot be read by anyone.\n\n"+
			"  Machine name:         %s\n  Encryption password:  %s\n", machine, password)
	}
	_, _ = fmt.Fprintf(p.out, "\nTo read this machine's records on another machine, run there:\n  agent-downlink add-machine %s\n", machine)
}

func installSchedule(p *Prompter, o Options) error {
	if o.NoTimer {
		_, _ = fmt.Fprintf(p.out, "\nNo hourly schedule was installed. Run 'agent-downlink push' now, or 'agent-downlink timer install' later.\n")
		return nil
	}
	if err := o.InstallTimer(); err != nil {
		return fmt.Errorf("the configuration was written, but the hourly schedule could not be installed: %w; fix that and run 'agent-downlink timer install'", err)
	}
	_, _ = fmt.Fprintf(p.out, "\nThe hourly schedule is installed. Run 'agent-downlink run' to make the first transfer now.\n")
	return nil
}

// generatePassword returns 256 random bits as text that is safe to paste anywhere.
func generatePassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
