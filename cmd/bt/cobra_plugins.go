package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/seanmartinsmith/beadstui/pkg/plugin"
	"github.com/seanmartinsmith/beadstui/pkg/ui"
	"github.com/seanmartinsmith/beadstui/pkg/version"
)

var (
	pluginsJSON    bool
	pluginsNoStart bool
)

// pluginProbeTimeout bounds the wait for every plugin to become active or
// fail: the 5s initialize timeout plus slack for starting the process.
var pluginProbeTimeout = 7 * time.Second

var pluginsCmd = &cobra.Command{
	Use:   "plugins",
	Short: "List configured plugins and their status",
	Long: `List the plugins configured in ~/.config/bt/config.yaml.

Each plugin is shown with its name, whether it is enabled, its command, the
binary that command resolves to on PATH, and any key overrides. Each enabled
plugin is then started once, without restarts, to report its state (active
or failed), version, last error and key conflicts, and shut down again.
--no-start skips that and only reads the config. See docs/plugins.md.`,
	Args: cobra.NoArgs,
	RunE: runPlugins,
}

// pluginInfo is one plugin as printed by 'bt plugins'. The runtime fields
// are left out with --no-start.
type pluginInfo struct {
	Name      string                 `json:"name"`
	Enabled   bool                   `json:"enabled"`
	Command   []string               `json:"command"`
	Binary    string                 `json:"binary"`
	Keys      map[string]string      `json:"keys,omitempty"`
	State     string                 `json:"state,omitempty"`
	Version   string                 `json:"version,omitempty"`
	Restarts  *int                   `json:"restarts,omitempty"`
	LastError string                 `json:"last_error,omitempty"`
	Conflicts []ui.PluginKeyConflict `json:"conflicts,omitempty"`
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
	if !pluginsNoStart && len(configs) > 0 {
		addPluginStatus(infos, probePlugins(configs))
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

// probePlugins starts every enabled plugin once, waits until each is active
// or failed (at most pluginProbeTimeout), and returns their statuses after
// shutting them down. A plugin still starting at the deadline is reported
// as starting.
func probePlugins(configs []plugin.Config) []plugin.Status {
	h := plugin.NewHost(plugin.Options{
		Configs:   configs,
		BTVersion: version.Version,
		Scope:     plugin.Scope{Mode: "project"},
		NoRestart: true,
	})
	h.Start()
	defer h.Stop()
	deadline := time.Now().Add(pluginProbeTimeout)
	for {
		statuses := h.Statuses()
		starting := false
		for _, st := range statuses {
			if st.State == "starting" {
				starting = true
				break
			}
		}
		if !starting || time.Now().After(deadline) {
			return statuses
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// addPluginStatus copies statuses, in config order like infos, into infos.
func addPluginStatus(infos []pluginInfo, statuses []plugin.Status) {
	conflicts := map[string][]ui.PluginKeyConflict{}
	for _, c := range ui.PluginKeyConflicts(statuses) {
		conflicts[c.Plugin] = append(conflicts[c.Plugin], c)
	}
	for i, st := range statuses {
		if i >= len(infos) || infos[i].Name != st.Name {
			continue
		}
		restarts := st.Restarts
		infos[i].State = st.State
		infos[i].Version = st.Version
		infos[i].Restarts = &restarts
		infos[i].LastError = st.LastError
		infos[i].Conflicts = conflicts[st.Name]
	}
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
	if info.Enabled && info.State != "" {
		line += "\n  state: " + info.State
		if info.Version != "" {
			line += "  version: " + plainText(info.Version)
		}
		if info.Restarts != nil && *info.Restarts > 0 {
			line += fmt.Sprintf("  restarts: %d", *info.Restarts)
		}
	}
	if info.LastError != "" {
		line += "\n  error: " + plainText(info.LastError)
	}
	for _, c := range info.Conflicts {
		line += fmt.Sprintf("\n  key %s (%s): %s", plainText(c.Key), c.Action, plainText(c.Reason))
	}
	return line
}

// plainText strips escape sequences and control characters from plugin
// text so it cannot drive the terminal.
func plainText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
}

func init() {
	pluginsCmd.Flags().BoolVar(&pluginsJSON, "json", false, "Print the plugin list as JSON")
	pluginsCmd.Flags().BoolVar(&pluginsNoStart, "no-start", false, "Only read the config; do not start plugins")
	rootCmd.AddCommand(pluginsCmd)
}
