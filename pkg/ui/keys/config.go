package keys

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	DefaultLeaderKey     = "space"
	DefaultWhichKeyDelay = 250 * time.Millisecond
)

// BindValue accepts either one key or a list of keys. The string "none"
// represents an empty binding.
type BindValue []string

// UnmarshalYAML decodes the scalar-or-list syntax used by keys.bind.
func (b *BindValue) UnmarshalYAML(n *yaml.Node) error {
	if n == nil {
		return errors.New("keys.bind value must be a string or list of strings")
	}
	switch n.Kind {
	case yaml.ScalarNode:
		if n.ShortTag() != "!!str" {
			return errors.New("keys.bind value must be a string or list of strings")
		}
		if strings.EqualFold(n.Value, "none") {
			*b = BindValue{}
		} else {
			*b = BindValue{n.Value}
		}
		return nil
	case yaml.SequenceNode:
		values := make(BindValue, 0, len(n.Content))
		for _, item := range n.Content {
			if item.Kind != yaml.ScalarNode || item.ShortTag() != "!!str" {
				return errors.New("keys.bind list entries must be strings")
			}
			values = append(values, item.Value)
		}
		*b = values
		return nil
	default:
		return errors.New("keys.bind value must be a string or list of strings")
	}
}

// Config is the keys: section in ~/.config/bt/config.yaml.
type Config struct {
	Leader        string               `yaml:"leader"`
	WhichKeyDelay string               `yaml:"which_key_delay"`
	Bind          map[string]BindValue `yaml:"bind"`
}

// LoadConfig reads the keys: section from path. A missing file means no
// overrides and is not an error.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("keys config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Config{}, fmt.Errorf("keys config %s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return Config{}, fmt.Errorf("keys config %s: expected a mapping", path)
	}

	var keysNode *yaml.Node
	root := doc.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value != "keys" {
			continue
		}
		if keysNode != nil {
			return Config{}, fmt.Errorf("keys config %s: duplicate keys section", path)
		}
		keysNode = root.Content[i+1]
	}
	if keysNode == nil || keysNode.ShortTag() == "!!null" {
		return Config{}, nil
	}
	config, err := decodeConfigNode(keysNode)
	if err != nil {
		return config, fmt.Errorf("keys config %s: %w", path, err)
	}
	return config, nil
}

func decodeConfigNode(node *yaml.Node) (Config, error) {
	var raw struct {
		Leader        string    `yaml:"leader"`
		WhichKeyDelay string    `yaml:"which_key_delay"`
		Bind          yaml.Node `yaml:"bind"`
	}
	if err := node.Decode(&raw); err != nil {
		return Config{}, err
	}

	cfg := Config{Leader: raw.Leader, WhichKeyDelay: raw.WhichKeyDelay}
	if raw.Bind.Kind == 0 {
		return cfg, nil
	}
	if raw.Bind.Kind != yaml.MappingNode {
		return cfg, errors.New("keys.bind must be a map")
	}

	cfg.Bind = make(map[string]BindValue, len(raw.Bind.Content)/2)
	var errs []error
	for i := 0; i < len(raw.Bind.Content); i += 2 {
		keyNode, valueNode := raw.Bind.Content[i], raw.Bind.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode || keyNode.ShortTag() != "!!str" {
			errs = append(errs, errors.New("keys.bind action IDs must be strings"))
			continue
		}
		var value BindValue
		if err := value.UnmarshalYAML(valueNode); err != nil {
			errs = append(errs, fmt.Errorf("keys.bind.%s: %w", keyNode.Value, err))
			continue
		}
		cfg.Bind[keyNode.Value] = value
	}
	return cfg, errors.Join(errs...)
}

// Settings are the resolved leader and which-key panel settings.
type Settings struct {
	Leader        string
	WhichKeyDelay time.Duration
	WhichKeyOff   bool
}

// Shadow describes a key removed from one action because another action
// claimed it. Conflict handling is added in the next config phase.
type Shadow struct{ ID, Key, By string }

// Apply validates and applies config entries in sorted action-ID order. Bad
// entries are reported without preventing other entries from applying.
func (a *AppKeys) Apply(cfg Config) (Settings, []Shadow, []error) {
	settings := Settings{Leader: DefaultLeaderKey, WhichKeyDelay: DefaultWhichKeyDelay}
	var errs []error

	if cfg.Leader != "" {
		if validPhysicalKey(cfg.Leader) {
			settings.Leader = cfg.Leader
		} else {
			errs = append(errs, fmt.Errorf("keys.leader %q: invalid key", cfg.Leader))
		}
	}

	if cfg.WhichKeyDelay != "" {
		if strings.EqualFold(cfg.WhichKeyDelay, "off") {
			settings.WhichKeyOff = true
		} else if delay, err := time.ParseDuration(cfg.WhichKeyDelay); err != nil {
			errs = append(errs, fmt.Errorf("keys.which_key_delay %q: expected a Go duration or off", cfg.WhichKeyDelay))
		} else {
			settings.WhichKeyDelay = delay
		}
	}

	ids := make([]string, 0, len(cfg.Bind))
	for id := range cfg.Bind {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		bindings := a.Lookup(id)
		if len(bindings) == 0 {
			errs = append(errs, fmt.Errorf("keys.bind.%s: unknown action ID", id))
			continue
		}

		configured := cfg.Bind[id]
		valid := true
		for _, raw := range configured {
			if !validConfiguredKey(raw) {
				errs = append(errs, fmt.Errorf("keys.bind.%s: invalid key %q", id, raw))
				valid = false
				break
			}
		}
		if !valid {
			continue
		}

		for _, binding := range bindings {
			var tokens []string
			for _, old := range binding.Keys() {
				if IsToken(old) {
					tokens = append(tokens, old)
				}
			}
			if len(tokens) == 0 {
				tokens = append(tokens, Token(id))
			}
			resolved := make([]string, 0, len(configured)+len(tokens))
			resolved = append(resolved, configured...)
			resolved = append(resolved, tokens...)
			binding.SetKeys(resolved...)
			help := binding.Help()
			binding.SetHelp(HelpLabel(resolved, LeaderLabel(settings.Leader)), help.Desc)
		}
	}

	return settings, nil, errs
}

func validConfiguredKey(raw string) bool {
	if raw == "" || IsToken(raw) {
		return false
	}
	if prefix, steps, ok := ParseSeq(raw); ok {
		if prefix == "" || len(steps) == 0 {
			return false
		}
		for _, step := range steps {
			if !validPhysicalKey(step) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(raw, "<") || strings.HasPrefix(raw, LeaderPrefix) {
		return false
	}
	return validPhysicalKey(raw)
}

func validPhysicalKey(raw string) bool {
	if raw == "" || IsToken(raw) {
		return false
	}
	if utf8.RuneCountInString(raw) == 1 {
		r, _ := utf8.DecodeRuneInString(raw)
		return unicode.IsPrint(r) && !unicode.IsSpace(r)
	}

	parts := strings.Split(raw, "+")
	base := parts[len(parts)-1]
	if !knownKeyName(base) {
		return false
	}
	if len(parts) == 1 {
		return true
	}
	modifiers := map[string]int{"ctrl": 0, "alt": 1, "shift": 2, "meta": 3, "hyper": 4, "super": 5}
	lastOrder := -1
	seen := make(map[string]bool, len(parts)-1)
	for _, modifier := range parts[:len(parts)-1] {
		order, ok := modifiers[modifier]
		if !ok || seen[modifier] || order <= lastOrder {
			return false
		}
		seen[modifier] = true
		lastOrder = order
	}
	return true
}

func knownKeyName(name string) bool {
	switch name {
	case "up", "down", "left", "right", "begin", "find", "insert", "delete", "select",
		"pgup", "pgdown", "home", "end", "kpenter", "kpequal", "kpmultiply", "kpplus",
		"kpcomma", "kpminus", "kpdecimal", "kpdivide", "kpsep", "kpup", "kpdown",
		"kpleft", "kpright", "kppgup", "kppgdown", "kphome", "kpend", "kpinsert", "kpdelete",
		"kpbegin", "capslock", "scrolllock", "numlock", "printscreen", "pause", "menu",
		"backspace", "tab", "enter", "return", "esc", "space":
		return true
	}
	if strings.HasPrefix(name, "f") {
		var n int
		if _, err := fmt.Sscanf(name, "f%d", &n); err == nil && n >= 1 && n <= 63 && name == fmt.Sprintf("f%d", n) {
			return true
		}
	}
	if strings.HasPrefix(name, "kp") && len(name) == 3 && name[2] >= '0' && name[2] <= '9' {
		return true
	}
	return false
}
