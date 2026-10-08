package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is one entry of the plugins: list in bt's config.yaml.
type Config struct {
	Name    string            `yaml:"name"`
	Command []string          `yaml:"command"`
	Enabled *bool             `yaml:"enabled"`
	Env     map[string]string `yaml:"env"`
	Options map[string]any    `yaml:"options"`
	Keys    map[string]string `yaml:"keys"`
}

// IsEnabled reports whether the plugin should run; unset means enabled.
func (c Config) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

// DefaultConfigPath returns bt's user config file, $HOME/.config/bt/config.yaml.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("plugin config: %w", err)
	}
	if home == "" {
		return "", errors.New("plugin config: no home directory")
	}
	return filepath.Join(home, ".config", "bt", "config.yaml"), nil
}

// LoadConfig reads the plugins: list from path. A missing file yields no
// plugins and no error. Invalid entries and duplicate names (the later entry)
// are left out of the result, and an error describing each is returned
// alongside the valid entries.
func LoadConfig(path string) ([]Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("plugin config: %w", err)
	}
	var doc struct {
		Plugins []Config `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("plugin config %s: %w", path, err)
	}
	var out []Config
	var errs []error
	seen := map[string]bool{}
	for i, c := range doc.Plugins {
		switch {
		case !idPattern.MatchString(c.Name):
			errs = append(errs, fmt.Errorf("plugin %d: name %q must match %s", i+1, c.Name, idPattern))
		case len(c.Command) == 0 || c.Command[0] == "":
			errs = append(errs, fmt.Errorf("plugin %s: empty command", c.Name))
		case seen[c.Name]:
			errs = append(errs, fmt.Errorf("plugin %s: duplicate name, later entry disabled", c.Name))
		default:
			seen[c.Name] = true
			out = append(out, c)
		}
	}
	return out, errors.Join(errs...)
}
