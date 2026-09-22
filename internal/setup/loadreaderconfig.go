package setup

import "github.com/jwp23/agent-downlink/internal/config"

// loadReaderConfig loads the paths, this machine's config, and the readable-machine
// secrets a command needs before acting on them.
func loadReaderConfig(home string) (config.Paths, config.File, config.RcloneConf, error) {
	paths := config.PathsFor(home)
	if err := config.CheckPermissions(paths); err != nil {
		return paths, config.File{}, config.RcloneConf{}, err
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return paths, config.File{}, config.RcloneConf{}, err
	}
	secrets, err := config.LoadRcloneConf(paths.RcloneConf)
	if err != nil {
		return paths, cfg, config.RcloneConf{}, err
	}
	return paths, cfg, secrets, nil
}
