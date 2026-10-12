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

	"charm.land/bubbles/v2/key"
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
	var cfg Config
	if node == nil || node.Kind != yaml.MappingNode {
		return cfg, errors.New("keys must be a map")
	}
	var errs []error
	for i := 0; i < len(node.Content); i += 2 {
		fieldNode, valueNode := node.Content[i], node.Content[i+1]
		if fieldNode.Kind != yaml.ScalarNode || fieldNode.ShortTag() != "!!str" {
			errs = append(errs, errors.New("keys field names must be strings"))
			continue
		}
		switch fieldNode.Value {
		case "leader":
			value, err := decodeConfigString(valueNode)
			if err != nil {
				errs = append(errs, fmt.Errorf("keys.leader: %w", err))
				continue
			}
			cfg.Leader = value
		case "which_key_delay":
			value, err := decodeConfigString(valueNode)
			if err != nil {
				errs = append(errs, fmt.Errorf("keys.which_key_delay: %w", err))
				continue
			}
			cfg.WhichKeyDelay = value
		case "bind":
			if valueNode.Kind != yaml.MappingNode {
				errs = append(errs, errors.New("keys.bind must be a map"))
				continue
			}
			if cfg.Bind == nil {
				cfg.Bind = make(map[string]BindValue, len(valueNode.Content)/2)
			}
			errs = append(errs, decodeBindEntries(valueNode, cfg.Bind)...)
		}
	}
	return cfg, errors.Join(errs...)
}

func decodeConfigString(node *yaml.Node) (string, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.ShortTag() == "!!null" {
		return "", errors.New("must be a scalar value")
	}
	var value string
	if err := node.Decode(&value); err != nil {
		return "", err
	}
	return value, nil
}

func decodeBindEntries(node *yaml.Node, bindings map[string]BindValue) []error {
	var errs []error
	for i := 0; i < len(node.Content); i += 2 {
		keyNode, valueNode := node.Content[i], node.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode || keyNode.ShortTag() != "!!str" {
			errs = append(errs, errors.New("keys.bind action IDs must be strings"))
			continue
		}
		var value BindValue
		if err := value.UnmarshalYAML(valueNode); err != nil {
			errs = append(errs, fmt.Errorf("keys.bind.%s: %w", keyNode.Value, err))
			continue
		}
		bindings[keyNode.Value] = value
	}
	return errs
}

// Settings are the resolved leader and which-key panel settings.
type Settings struct {
	Leader        string
	WhichKeyDelay time.Duration
	WhichKeyOff   bool
}

// Shadow describes a key removed from one action because another action
// claimed it.
type Shadow struct{ ID, Key, By string }

const leaderActionID = "keys.leader"

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
	accepted := make(map[string]BindValue, len(ids))
	var acceptedIDs []string
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
		if first, second, conflict := conflictingConfiguredKeys(configured); conflict {
			errs = append(errs, fmt.Errorf("keys.bind.%s: keys %q and %q conflict", id, first, second))
			continue
		}
		if hasSequence(configured) && actionHasModalOrTextBinding(a, id) {
			errs = append(errs, fmt.Errorf("keys.bind.%s: leader and gg sequences are unsupported for modal/text actions; use direct keys", id))
			continue
		}
		accepted[id] = append(BindValue(nil), configured...)
		acceptedIDs = append(acceptedIDs, id)

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

	leaderShadows := reserveLeaderKey(a, settings.Leader)
	shadows, conflictErrs := resolveConfiguredConflicts(a, acceptedIDs, accepted, settings.Leader)
	shadows = append(leaderShadows, shadows...)
	errs = append(errs, conflictErrs...)
	return settings, shadows, errs
}

func conflictingConfiguredKeys(configured BindValue) (string, string, bool) {
	for i, current := range configured {
		for _, previous := range configured[:i] {
			if !keysConflict(current, previous) {
				continue
			}
			if current < previous {
				return current, previous, true
			}
			return previous, current, true
		}
	}
	return "", "", false
}

func reserveLeaderKey(a *AppKeys, leader string) []Shadow {
	var shadows []Shadow
	seen := make(map[Shadow]struct{})
	a.Walk(func(ref Ref, binding *key.Binding) {
		if ref.Map != "Global" {
			if _, view := viewMaps[ref.Map]; !view {
				return
			}
		}
		if !removeBindingKey(binding, leader, leader) {
			return
		}
		shadow := Shadow{ID: ref.ID, Key: leader, By: leaderActionID}
		if _, exists := seen[shadow]; exists {
			return
		}
		seen[shadow] = struct{}{}
		shadows = append(shadows, shadow)
	})
	return shadows
}

type bindingRef struct {
	ref     Ref
	binding *key.Binding
}

func resolveConfiguredConflicts(a *AppKeys, ids []string, configured map[string]BindValue, leader string) ([]Shadow, []error) {
	var refs []bindingRef
	a.Walk(func(ref Ref, binding *key.Binding) {
		refs = append(refs, bindingRef{ref: ref, binding: binding})
	})

	var shadows []Shadow
	seenShadows := make(map[Shadow]struct{})
	var errs []error
	conflictErrors := make(map[string]struct{})
	for _, winnerID := range ids {
		for _, winner := range refs {
			if winner.ref.ID != winnerID {
				continue
			}
			for _, winnerKey := range configured[winnerID] {
				if !bindingHasKey(winner.binding, winnerKey) {
					continue
				}
				for _, other := range refs {
					if other.ref.ID == winnerID || !mapsOverlap(winner.ref.Map, other.ref.Map) {
						continue
					}
					for _, otherKey := range other.binding.Keys() {
						if IsToken(otherKey) || !keysConflict(winnerKey, otherKey) {
							continue
						}
						if !removeBindingKey(other.binding, otherKey, leader) {
							continue
						}
						shadow := Shadow{ID: other.ref.ID, Key: otherKey, By: winnerID}
						if _, seen := seenShadows[shadow]; !seen {
							seenShadows[shadow] = struct{}{}
							shadows = append(shadows, shadow)
						}
						if containsConfiguredKey(configured[other.ref.ID], otherKey) {
							if _, reported := conflictErrors[other.ref.ID]; !reported {
								conflictErrors[other.ref.ID] = struct{}{}
								errs = append(errs, fmt.Errorf("keys.bind.%s: key %q conflicts with configured binding %s", other.ref.ID, otherKey, winnerID))
							}
						}
					}
				}
			}
		}
	}
	return shadows, errs
}

func bindingHasKey(binding *key.Binding, want string) bool {
	for _, existing := range binding.Keys() {
		if existing == want {
			return true
		}
	}
	return false
}

func containsConfiguredKey(configured BindValue, want string) bool {
	for _, existing := range configured {
		if existing == want {
			return true
		}
	}
	return false
}

func removeBindingKey(binding *key.Binding, removed, leader string) bool {
	oldKeys := binding.Keys()
	keys := make([]string, 0, len(oldKeys))
	for _, existing := range oldKeys {
		if existing != removed {
			keys = append(keys, existing)
		}
	}
	if len(keys) == len(oldKeys) {
		return false
	}
	binding.SetKeys(keys...)
	help := binding.Help()
	binding.SetHelp(HelpLabel(keys, LeaderLabel(leader)), help.Desc)
	return true
}

func mapsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	if a == "Global" {
		_, view := viewMaps[b]
		return view
	}
	if b == "Global" {
		_, view := viewMaps[a]
		return view
	}
	return false
}

func keysConflict(a, b string) bool {
	if a == b {
		return true
	}
	aPrefix, aSteps, aSequence := ParseSeq(a)
	bPrefix, bSteps, bSequence := ParseSeq(b)
	if aSequence && !bSequence && b == aPrefix {
		return true
	}
	if bSequence && !aSequence && a == bPrefix {
		return true
	}
	if !aSequence || !bSequence || aPrefix != bPrefix {
		return false
	}
	return sequencePrefix(aSteps, bSteps) || sequencePrefix(bSteps, aSteps)
}

func sequencePrefix(prefix, steps []string) bool {
	if len(prefix) > len(steps) {
		return false
	}
	for i := range prefix {
		if prefix[i] != steps[i] {
			return false
		}
	}
	return true
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
	if !knownKeyName(base) && !isPrintableKeyRune(base) {
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

func isPrintableKeyRune(raw string) bool {
	if utf8.RuneCountInString(raw) != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(raw)
	return unicode.IsPrint(r) && !unicode.IsSpace(r)
}

func hasSequence(bindings BindValue) bool {
	for _, raw := range bindings {
		if _, _, ok := ParseSeq(raw); ok {
			return true
		}
	}
	return false
}

var modalMaps = map[string]struct{}{
	"LabelPickerNav": {}, "LabelPickerSearch": {}, "RecipePicker": {},
	"BQLQuery": {}, "TimeTravelInput": {}, "Settings": {},
	"RepoPickerNav": {}, "RepoPickerSearch": {}, "EpicCard": {},
	"Confirm": {}, "FieldSelect": {}, "FieldPicker": {}, "FieldInput": {},
	"LongformEdit": {}, "PluginSelect": {}, "Help": {}, "Notifications": {},
	"Alerts": {}, "LabelHealth": {}, "LabelDrilldown": {}, "LabelGraph": {},
	"Cass": {}, "AgentPrompt": {}, "Tutorial": {}, "Update": {},
}

var viewMaps = map[string]struct{}{
	"ListNormal": {}, "Tree": {}, "BoardNormal": {}, "Graph": {}, "Insights": {},
	"HistoryNormal": {}, "HistoryFileTree": {}, "Actionable": {}, "FlowMatrix": {},
	"Epics": {}, "LabelDashboard": {}, "Attention": {}, "MemoriesNormal": {},
}

var textInputMaps = map[string]struct{}{
	"ListSearch": {}, "BoardSearch": {}, "HistorySearch": {}, "MemoriesSearch": {},
}

func actionHasModalOrTextBinding(a *AppKeys, id string) bool {
	found := false
	a.Walk(func(ref Ref, _ *key.Binding) {
		if ref.ID != id {
			return
		}
		if _, ok := modalMaps[ref.Map]; ok {
			found = true
		}
		if _, ok := textInputMaps[ref.Map]; ok {
			found = true
		}
	})
	return found
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
