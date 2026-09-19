// Package config reads and writes the tool's two configuration files and knows where
// everything the tool owns lives on disk.
package config

import "path/filepath"

// Paths are the locations of every file the tool owns, all under one home directory.
type Paths struct {
	ConfigDir, ConfigFile, RcloneConf       string
	StateDir, StatusFile, LogFile, LockFile string
	DefaultMirror                           string
}

// PathsFor returns the tool's file locations under home. The locations are the same on
// every OS so that "where is the log" has one answer.
func PathsFor(home string) Paths {
	configDir := filepath.Join(home, ".config", "agent-downlink")
	stateDir := filepath.Join(home, ".local", "state", "agent-downlink")
	return Paths{
		ConfigDir:     configDir,
		ConfigFile:    filepath.Join(configDir, "config.toml"),
		RcloneConf:    filepath.Join(configDir, "rclone.conf"),
		StateDir:      stateDir,
		StatusFile:    filepath.Join(stateDir, "status.json"),
		LogFile:       filepath.Join(stateDir, "agent-downlink.log"),
		LockFile:      filepath.Join(stateDir, "run.lock"),
		DefaultMirror: filepath.Join(home, ".local", "share", "agent-downlink", "mirror"),
	}
}
