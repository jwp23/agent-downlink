package config

import "testing"

func TestPathsForFollowsTheDesignLayout(t *testing.T) {
	p := PathsFor("/home/u")
	want := Paths{
		ConfigDir:     "/home/u/.config/agent-downlink",
		ConfigFile:    "/home/u/.config/agent-downlink/config.toml",
		RcloneConf:    "/home/u/.config/agent-downlink/rclone.conf",
		StateDir:      "/home/u/.local/state/agent-downlink",
		StatusFile:    "/home/u/.local/state/agent-downlink/status.json",
		LogFile:       "/home/u/.local/state/agent-downlink/agent-downlink.log",
		LockFile:      "/home/u/.local/state/agent-downlink/run.lock",
		DefaultMirror: "/home/u/.local/share/agent-downlink/mirror",
	}
	if p != want {
		t.Errorf("PathsFor = %+v\nwant %+v", p, want)
	}
}
