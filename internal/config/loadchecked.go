package config

// LoadChecked loads the paths, this machine's config, and the readable-machine secrets a
// command needs before acting on them, refusing first if rclone.conf's permissions have
// loosened.
func LoadChecked(home string) (Paths, File, RcloneConf, error) {
	paths := PathsFor(home)
	if err := CheckPermissions(paths); err != nil {
		return paths, File{}, RcloneConf{}, err
	}
	cfg, err := Load(paths.ConfigFile)
	if err != nil {
		return paths, File{}, RcloneConf{}, err
	}
	secrets, err := LoadRcloneConf(paths.RcloneConf)
	if err != nil {
		return paths, cfg, RcloneConf{}, err
	}
	return paths, cfg, secrets, nil
}
