package setup

import (
	"fmt"
	"io"

	"github.com/jwp23/agent-downlink/internal/config"
)

// ListMachines prints every machine readable here, one per line, sorted, marking this machine.
// It reads only the config files: no rclone call and no status.
func ListMachines(w io.Writer, home string) error {
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
	for _, machine := range secrets.Machines() {
		if machine == cfg.Machine {
			_, _ = fmt.Fprintf(w, "%s (this machine)\n", machine)
		} else {
			_, _ = fmt.Fprintln(w, machine)
		}
	}
	return nil
}
