package keys

import (
	"strings"
)

// HelpLabel renders physical keys and sequence keys for help surfaces.
// Action tokens are implementation details and are omitted.
func HelpLabel(keys []string, leaderLabel string) string {
	var direct []string
	var sequences []string
	for _, raw := range keys {
		if IsToken(raw) {
			continue
		}
		if prefix, steps, ok := ParseSeq(raw); ok && len(steps) > 0 {
			sequences = append(sequences, FormatSeq(prefix, steps, leaderLabel))
			continue
		}
		direct = append(direct, raw)
	}

	labels := make([]string, 0, len(sequences)+1)
	if len(direct) > 0 {
		labels = append(labels, directKeysLabel(direct))
	}
	labels = append(labels, sequences...)
	return strings.Join(labels, " / ")
}

// LeaderLabel is the compact display form of the leader key.
func LeaderLabel(leader string) string {
	if leader == "space" {
		return "SPC"
	}
	return leader
}

func directKeysLabel(keys []string) string {
	if len(keys) == 9 {
		consecutive := true
		for i, raw := range keys {
			if raw != string(rune('1'+i)) {
				consecutive = false
				break
			}
		}
		if consecutive {
			return "1-9"
		}
	}
	labels := make([]string, len(keys))
	for i, raw := range keys {
		labels[i] = physicalKeyLabel(raw)
	}
	return strings.Join(labels, "/")
}

func physicalKeyLabel(raw string) string {
	switch raw {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "enter":
		return "⏎"
	case "tab":
		return "⇥"
	case "backspace":
		return "⌫"
	case "delete":
		return "⌦"
	case "pgup":
		return "pgup"
	case "pgdown":
		return "pgdn"
	case "space":
		return "space"
	}
	for _, modifier := range []struct{ name, symbol string }{
		{name: "ctrl+", symbol: "⌃"},
		{name: "shift+", symbol: "⇧"},
		{name: "alt+", symbol: "⌥"},
	} {
		if strings.HasPrefix(raw, modifier.name) {
			return modifier.symbol + physicalKeyLabel(strings.TrimPrefix(raw, modifier.name))
		}
	}
	return raw
}
