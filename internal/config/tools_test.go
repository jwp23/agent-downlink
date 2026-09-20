package config

import (
	"reflect"
	"testing"
)

func TestSourcesResolvesBuiltinsThenCustomTools(t *testing.T) {
	f := validFile()
	f.CustomTools = []CustomTool{{Name: "other-agent", Root: "/data/other", Paths: []string{"sessions"}}}
	got, err := f.Sources("/home/u")
	if err != nil {
		t.Fatal(err)
	}
	want := []Source{
		{Tool: "claude-code", Root: "/home/u/.claude", Paths: []string{"projects"}, PluginManifest: "plugins/installed_plugins.json"},
		{Tool: "other-agent", Root: "/data/other", Paths: []string{"sessions"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Sources = %+v\nwant %+v", got, want)
	}
}

func TestSourcesWithNoToolsIsEmpty(t *testing.T) {
	f := validFile()
	f.Tools = nil
	got, err := f.Sources("/home/u")
	if err != nil || len(got) != 0 {
		t.Errorf("Sources = %+v, %v; want empty, nil", got, err)
	}
}

func TestBuiltinToolNames(t *testing.T) {
	if got := BuiltinToolNames(); !reflect.DeepEqual(got, []string{"claude-code"}) {
		t.Errorf("BuiltinToolNames = %v", got)
	}
}
