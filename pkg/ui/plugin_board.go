package ui

import (
	"image/color"
	"strings"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

// pluginBoardModes builds one board mode per plugin field: a lane per
// lane=true value in manifest order, then a lane for beads with no value.
// Beads whose value has lane=false stay off the board in that mode. value
// looks a bead's field up by its BQL name.
func pluginBoardModes(fields []plugin.LaneField, value func(*model.Issue, string) (string, bool)) []BoardMode {
	modes := make([]BoardMode, 0, len(fields))
	for _, f := range fields {
		lanes := make([]BoardLane, 0, len(f.Values)+1)
		index := make(map[string]int, len(f.Values))
		for i, v := range f.Values {
			index[v.ID] = i
			lanes = append(lanes, pluginLane(v))
		}
		label := f.Label
		if label == "" {
			label = f.Name
		}
		none := len(lanes)
		lanes = append(lanes, BoardLane{
			Title: "NO " + strings.ToUpper(label),
			Icon:  func() string { return "" },
			Color: func(Theme) color.Color { return ColorMuted },
		})
		name := f.Name
		modes = append(modes, BoardMode{
			Key:   SwimLaneMode(name),
			Name:  name,
			Lanes: lanes,
			Lane: func(issue model.Issue) (int, bool) {
				v, ok := value(&issue, name)
				if !ok {
					return none, true
				}
				i, ok := index[v]
				return i, ok
			},
			ShowEmpty: true,
		})
	}
	return modes
}

// pluginLane is the board lane of a field value: its label, badge and tone.
func pluginLane(v plugin.FieldValue) BoardLane {
	badge, title := v.Badge, v.Label
	if title == "" {
		title = v.ID
	}
	lane := BoardLane{
		Title: strings.ToUpper(title),
		Icon:  func() string { return badge },
		Color: func(Theme) color.Color { return ColorSecondary },
	}
	if v.Tone != "" {
		tone := v.BadgeTone()
		lane.Color = func(Theme) color.Color { return toneColor(tone) }
	}
	return lane
}

// refreshPluginBoardModes rebuilds the board's plugin modes from the active
// plugins, so a plugin's modes appear when it starts and go when it stops,
// and regroups the board when it shows a plugin mode.
func (m *Model) refreshPluginBoardModes() {
	var fields []plugin.LaneField
	if m.pluginLaneFields != nil {
		fields = m.pluginLaneFields()
	}
	m.board.SetExtraModes(pluginBoardModes(fields, m.slotRegistry.FieldValue))
}
