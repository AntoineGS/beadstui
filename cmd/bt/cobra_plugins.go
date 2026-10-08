package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

var pluginsJSON bool

var pluginsCmd = &cobra.Command{
	Use:   "plugins",
	Short: "List configured plugins",
	Long: `List the plugins configured in ~/.config/bt/config.yaml.

Each plugin is shown with its name, whether it is enabled, its command, the
binary that command resolves to on PATH, and any key overrides. Nothing is
started. See docs/plugins.md.`,
	Args: cobra.NoArgs,
	RunE: runPlugins,
}

// pluginInfo is one plugin as printed by 'bt plugins'.
type pluginInfo struct {
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Command []string          `json:"command"`
	Binary  string            `json:"binary"`
	Keys    map[string]string `json:"keys,omitempty"`
}

func runPlugins(cmd *cobra.Command, _ []string) error {
	path, err := plugin.DefaultConfigPath()
	if err != nil {
		return err
	}
	// LoadConfig returns the valid entries alongside any error; print them
	// first so a typo in one entry does not hide the rest.
	configs, cfgErr := plugin.LoadConfig(path)

	infos := make([]pluginInfo, 0, len(configs))
	for _, c := range configs {
		info := pluginInfo{Name: c.Name, Enabled: c.IsEnabled(), Command: c.Command, Keys: c.Keys}
		if bin, err := exec.LookPath(c.Command[0]); err == nil {
			info.Binary = bin
		}
		infos = append(infos, info)
	}

	out := cmd.OutOrStdout()
	switch {
	case pluginsJSON:
		data, err := json.MarshalIndent(infos, "", "  ")
		if err != nil {
			return fmt.Errorf("encode plugins: %w", err)
		}
		fmt.Fprintln(out, string(data))
	case len(infos) == 0 && cfgErr == nil:
		fmt.Fprintf(out, "no plugins configured in %s\n", path)
	default:
		for _, info := range infos {
			fmt.Fprintln(out, formatPluginInfo(info))
		}
	}
	if cfgErr != nil {
		return fmt.Errorf("plugins config: %w", cfgErr)
	}
	return nil
}

func formatPluginInfo(info pluginInfo) string {
	state := "enabled"
	if !info.Enabled {
		state = "disabled"
	}
	binary := info.Binary
	if binary == "" {
		binary = "not found"
	}
	line := fmt.Sprintf("%s  %s  %s  (%s)", info.Name, state, strings.Join(info.Command, " "), binary)
	if len(info.Keys) > 0 {
		ids := make([]string, 0, len(info.Keys))
		for id := range info.Keys {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		pairs := make([]string, len(ids))
		for i, id := range ids {
			pairs[i] = id + "=" + info.Keys[id]
		}
		line += "  keys: " + strings.Join(pairs, " ")
	}
	return line
}

func init() {
	pluginsCmd.Flags().BoolVar(&pluginsJSON, "json", false, "Print the plugin list as JSON")
	rootCmd.AddCommand(pluginsCmd)
}
