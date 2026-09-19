package config

import (
	"fmt"
	"path/filepath"
	"sort"
)

// Source is one tool's records on this machine, resolved from a built-in or a custom tool.
type Source struct {
	Tool           string   // <tool> folder name in the mirror
	Root           string   // absolute data directory
	Paths          []string // sub-paths of Root to archive, relative, slash-separated
	PluginManifest string   // file under Root to keep a timeline of; "" for none
}

// builtinTools is where each supported agent keeps its records, so operators never need to
// know. Only the listed sub-paths are archived: an agent's data directory can also hold
// credentials.
var builtinTools = map[string]func(home string) Source{
	"claude-code": func(home string) Source {
		return Source{
			Tool:           "claude-code",
			Root:           filepath.Join(home, ".claude"),
			Paths:          []string{"projects"},
			PluginManifest: "plugins/installed_plugins.json",
		}
	},
}

// BuiltinToolNames lists the tools that can be named in config.toml's tools field.
func BuiltinToolNames() []string {
	names := make([]string, 0, len(builtinTools))
	for name := range builtinTools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Sources resolves the enabled built-in tools, then the custom tools, in file order.
func (f File) Sources(home string) ([]Source, error) {
	var sources []Source
	for _, name := range f.Tools {
		builtin, ok := builtinTools[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool %q; built-in tools are %v", name, BuiltinToolNames())
		}
		sources = append(sources, builtin(home))
	}
	for _, c := range f.CustomTools {
		sources = append(sources, Source{Tool: c.Name, Root: c.Root, Paths: c.Paths, PluginManifest: c.PluginManifest})
	}
	return sources, nil
}
