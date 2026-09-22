package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jwp23/agent-downlink/internal/config"
	"github.com/jwp23/agent-downlink/internal/status"
)

// RemoveMachine stops pulling another machine here: it forgets that machine's encryption
// password and status, and deletes its folder from the local mirror. This is the one place
// the tool deletes anything. The machine's area in the bucket is untouched; removing it is a
// runbook procedure.
func RemoveMachine(p *Prompter, home, machine string) error {
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
	if machine == cfg.Machine {
		return fmt.Errorf("%s is this machine; retire it with setup and timer remove instead", machine)
	}
	secrets, err := config.LoadRcloneConf(paths.RcloneConf)
	if err != nil {
		return err
	}
	if _, present := secrets.Passwords[machine]; !present {
		return fmt.Errorf("%s is not readable here", machine)
	}
	st, err := status.Load(paths.StatusFile)
	if err != nil {
		return err
	}

	folder := filepath.Join(cfg.Mirror, machine)
	_, _ = fmt.Fprintf(p.out, "This deletes %s and forgets %s's encryption password here.\nIts records stay in the bucket.\n", folder, machine)
	if err := confirmReplacement(p, fmt.Sprintf("Remove %s here?", machine)); err != nil {
		return err
	}

	// The password goes first, so that a failure later on cannot leave the machine still
	// being pulled.
	delete(secrets.Passwords, machine)
	if err := secrets.Save(paths.RcloneConf, cfg.Storage); err != nil {
		return err
	}
	delete(st.Steps, "pull:"+machine)
	if err := st.Save(paths.StatusFile); err != nil {
		return err
	}
	if err := os.RemoveAll(folder); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(p.out, "%s is no longer readable here. Its area in the bucket, %s/%s, is untouched.\n", machine, cfg.Storage, machine)
	return nil
}
