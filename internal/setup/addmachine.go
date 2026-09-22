package setup

import (
	"context"
	"errors"
	"fmt"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/rclone"
)

// AddMachine makes another machine's records readable on this one by storing that machine's
// encryption password here. It adds a machine; it never removes one.
func AddMachine(ctx context.Context, p *Prompter, home, machine string) error {
	if err := config.ValidateMachineName(machine); err != nil {
		return err
	}
	paths := config.PathsFor(home)
	if err := config.CheckPermissions(paths); err != nil {
		return err
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return err
	}
	secrets, err := config.LoadRcloneConf(paths.RcloneConf)
	if err != nil {
		return err
	}
	runner, err := rclone.New(cfg.Rclone, paths.RcloneConf, rclone.Concurrency{}) // add-machine only obscures a password
	if err != nil {
		return err
	}

	if _, present := secrets.Passwords[machine]; present {
		_, _ = fmt.Fprintf(p.out, "%s already has an encryption password here.\n", machine)
		if err := confirmReplacement(p, "Replace it?"); err != nil {
			return err
		}
	}
	password, err := p.AskSecret(fmt.Sprintf("Encryption password of %s", machine))
	if err != nil {
		return err
	}
	if password == "" {
		return errors.New("the password is empty")
	}
	if secrets.Passwords[machine], err = runner.Obscure(ctx, password); err != nil {
		return err
	}
	if err := secrets.Save(paths.RcloneConf, cfg.Storage); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(p.out, "%s is now readable here. The next run pulls its records into %s\n", machine, cfg.Mirror)
	return nil
}
