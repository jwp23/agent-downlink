package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CustomTool describes an agent tool that is not built in.
type CustomTool struct {
	Name           string   `toml:"name" comment:"Folder name for this tool in the mirror: lowercase letters, digits, and hyphens."`
	Root           string   `toml:"root" comment:"Absolute path of the tool's data directory."`
	Paths          []string `toml:"paths" comment:"Sub-paths of root to archive. Each is copied to <mirror>/<machine>/<name>/<path>."`
	PluginManifest string   `toml:"plugin_manifest,omitempty" comment:"Optional file under root; a dated copy is kept whenever its content changes."`
}

// File is config.toml: the non-secret settings.
type File struct {
	Machine     string       `toml:"machine" comment:"This machine's name. Its encrypted area in the bucket sits under this name."`
	Storage     string       `toml:"storage" comment:"The rclone path of the bucket: b2:<bucket-name>."`
	Mirror      string       `toml:"mirror" comment:"Directory holding the decrypted mirror of every readable machine. Move it to an encrypted volume if you have one."`
	Rclone      string       `toml:"rclone" comment:"Absolute path of the rclone binary. Schedulers run with a minimal PATH. Empty means search PATH."`
	Tools       []string     `toml:"tools" comment:"Built-in agent tools whose records are archived from this machine."`
	CustomTools []CustomTool `toml:"custom_tools,omitempty" comment:"Agent tools that are not built in."`
}

// Load reads and validates config.toml.
func Load(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	dec := toml.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := f.Validate(); err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Validate reports the first setting that the tool cannot work with.
func (f File) Validate() error {
	if err := ValidateMachineName(f.Machine); err != nil {
		return err
	}
	if f.Storage == "" {
		return fmt.Errorf("storage is empty; it should be b2:<bucket-name>")
	}
	if strings.HasPrefix(f.Storage, storageRemote+":") {
		if strings.TrimPrefix(f.Storage, storageRemote+":") == "" {
			return fmt.Errorf("storage is missing a bucket name; it should be b2:<bucket-name>")
		}
	} else if !filepath.IsAbs(f.Storage) {
		return fmt.Errorf("storage must be b2:<bucket-name> or an absolute path, got %q", f.Storage)
	}
	if !filepath.IsAbs(f.Mirror) {
		return fmt.Errorf("mirror must be an absolute path, got %q", f.Mirror)
	}
	if f.Rclone != "" && !filepath.IsAbs(f.Rclone) {
		return fmt.Errorf("rclone must be an absolute path or empty, got %q", f.Rclone)
	}
	names := map[string]bool{}
	for _, name := range f.Tools {
		if _, ok := builtinTools[name]; !ok {
			return fmt.Errorf("unknown tool %q; built-in tools are %v", name, BuiltinToolNames())
		}
		if names[name] {
			return fmt.Errorf("duplicate tool name %q", name)
		}
		names[name] = true
	}
	for _, c := range f.CustomTools {
		if err := c.validate(); err != nil {
			return err
		}
		if names[c.Name] {
			return fmt.Errorf("duplicate tool name %q", c.Name)
		}
		names[c.Name] = true
	}
	return nil
}

func (c CustomTool) validate() error {
	if err := validateName("custom tool name", c.Name); err != nil {
		return err
	}
	if _, ok := builtinTools[c.Name]; ok {
		return fmt.Errorf("custom tool %q has the name of a tool that is built in; list it under tools instead", c.Name)
	}
	if !filepath.IsAbs(c.Root) {
		return fmt.Errorf("custom tool %q: root must be an absolute path, got %q", c.Name, c.Root)
	}
	if len(c.Paths) == 0 {
		return fmt.Errorf("custom tool %q: list at least one path to archive", c.Name)
	}
	for _, p := range append(append([]string{}, c.Paths...), c.PluginManifest) {
		if p != "" && !filepath.IsLocal(p) {
			return fmt.Errorf("custom tool %q: %q must stay inside root", c.Name, p)
		}
	}
	return nil
}

// Save validates f and writes it with a comment above every field.
func (f File) Save(path string) error {
	if err := f.Validate(); err != nil {
		return err
	}
	b, err := toml.Marshal(f)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Written to a temporary file and renamed into place, so a write error or the process
	// dying mid-write never truncates or corrupts an existing config.toml.
	tmp, err := os.CreateTemp(dir, ".config.toml-*") // CreateTemp makes the file 0600
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // a no-op once the rename has happened
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
