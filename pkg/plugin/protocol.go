// Package plugin hosts external plugin processes. A plugin is an executable
// that speaks JSON-RPC 2.0 over newline-delimited JSON on its stdin and
// stdout. bt sends it bead snapshots and action requests; the plugin sends
// back per-bead state that bt renders as badges, detail sections and BQL
// fields. See docs/plugins.md for the protocol.
package plugin

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
)

// ProtocolVersion is the protocol major version this bt speaks.
const ProtocolVersion = 1

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var validTones = map[string]bool{"": true, "accent": true, "ok": true, "warn": true, "error": true, "muted": true}

// maxBadgeWidth is the widest badge a plugin may declare, in cells.
const maxBadgeWidth = 6

// Scope tells a plugin which beads bt is showing.
type Scope struct {
	Mode      string   `json:"mode"` // project, global or workspace
	Databases []string `json:"databases,omitempty"`
}

// InitializeParams are sent with the initialize request.
type InitializeParams struct {
	ProtocolVersion int `json:"protocolVersion"`
	BT              struct {
		Version string `json:"version"`
	} `json:"bt"`
	Scope   Scope          `json:"scope"`
	Popup   bool           `json:"popup"`
	Options map[string]any `json:"options"`
}

// Manifest is a plugin's answer to initialize.
type Manifest struct {
	ProtocolVersion int           `json:"protocolVersion"`
	Name            string        `json:"name"`
	Version         string        `json:"version"`
	Subscribe       Subscribe     `json:"subscribe"`
	Fields          []Field       `json:"fields"`
	Sections        []SectionDecl `json:"sections"`
	Actions         []ActionDecl  `json:"actions"`
}

// Subscribe narrows beads.sync. Empty Statuses means every status; empty
// MetadataPrefixes means no metadata is sent.
type Subscribe struct {
	MetadataPrefixes []string `json:"metadata_prefixes"`
	Statuses         []string `json:"statuses"`
}

// Field is an enum-valued per-bead field a plugin publishes.
type Field struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	Values []FieldValue `json:"values"`
}

// FieldValue is one value of a Field. A value without Badge draws no badge.
type FieldValue struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Badge string `json:"badge,omitempty"`
	Tone  string `json:"tone,omitempty"`
	Lane  *bool  `json:"lane,omitempty"`
}

// InLane reports whether the value gets a board lane (default true).
func (v FieldValue) InLane() bool { return v.Lane == nil || *v.Lane }

// SectionDecl declares a detail-pane section.
type SectionDecl struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// ActionDecl declares an action on the selected bead.
type ActionDecl struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Key   string `json:"key"`
}

// BeadKey identifies a bead across databases.
type BeadKey struct {
	DB string `json:"db"`
	ID string `json:"id"`
}

// BeadRef is a BeadKey plus the checkout path, null when unknown.
type BeadRef struct {
	DB   string  `json:"db"`
	ID   string  `json:"id"`
	Repo *string `json:"repo"`
}

// Bead is one entry of beads.sync.
type Bead struct {
	DB       string                     `json:"db"`
	ID       string                     `json:"id"`
	Repo     *string                    `json:"repo"`
	Status   string                     `json:"status"`
	Title    string                     `json:"title"`
	Type     string                     `json:"type"`
	Priority int                        `json:"priority"`
	Assignee string                     `json:"assignee"`
	Metadata map[string]json.RawMessage `json:"metadata"`
}

// SyncParams are the params of beads.sync.
type SyncParams struct {
	Revision int    `json:"revision"`
	Beads    []Bead `json:"beads"`
}

// FieldState is a bead's value for one field.
type FieldState struct {
	Value string     `json:"value"`
	Since *time.Time `json:"since,omitempty"`
}

// BeadState is a plugin's full state for one bead.
type BeadState struct {
	DB       string                `json:"db"`
	ID       string                `json:"id"`
	Fields   map[string]FieldState `json:"fields,omitempty"`
	Sections map[string]string     `json:"sections,omitempty"`
	Actions  []string              `json:"actions,omitempty"`
}

// StateSetParams are the params of state.set.
type StateSetParams struct {
	Replace bool        `json:"replace"`
	Beads   []BeadState `json:"beads"`
}

// StateClearParams are the params of state.clear.
type StateClearParams struct {
	Beads []BeadKey `json:"beads"`
}

// InvokeParams are the params of action.invoke.
type InvokeParams struct {
	Action string  `json:"action"`
	Bead   BeadRef `json:"bead"`
	View   string  `json:"view"`
}

// Toast is a short message shown in bt's footer.
type Toast struct {
	Message string `json:"message"`
	Tone    string `json:"tone,omitempty"`
}

// InvokeResult is the result of action.invoke.
type InvokeResult struct {
	Toast   *Toast `json:"toast,omitempty"`
	Dismiss bool   `json:"dismiss,omitempty"`
}

// ConfirmParams are the params of ui.confirm.
type ConfirmParams struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Confirm string `json:"confirm,omitempty"`
	Cancel  string `json:"cancel,omitempty"`
}

// SelectOption is one choice of ui.select.
type SelectOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// SelectParams are the params of ui.select.
type SelectParams struct {
	Title   string         `json:"title"`
	Options []SelectOption `json:"options"`
}

// Validate checks a manifest against the configured plugin name.
func (m Manifest) Validate(configName string) error {
	if m.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("protocol version %d, bt speaks %d", m.ProtocolVersion, ProtocolVersion)
	}
	if m.Name != configName {
		return fmt.Errorf("manifest name %q does not match config name %q", m.Name, configName)
	}
	seen := map[string]bool{}
	for _, f := range m.Fields {
		if err := checkID("field", f.ID, seen); err != nil {
			return err
		}
		if err := checkText("field "+f.ID+" label", f.Label); err != nil {
			return err
		}
		values := map[string]bool{}
		for _, v := range f.Values {
			if err := checkID("value of field "+f.ID, v.ID, values); err != nil {
				return err
			}
			if err := checkText("field "+f.ID+" value "+v.ID+" label", v.Label); err != nil {
				return err
			}
			if err := checkText("field "+f.ID+" value "+v.ID+" badge", v.Badge); err != nil {
				return err
			}
			if !validTones[v.Tone] {
				return fmt.Errorf("field %s value %s: unknown tone %q", f.ID, v.ID, v.Tone)
			}
			if lipgloss.Width(v.Badge) > maxBadgeWidth {
				return fmt.Errorf("field %s value %s: badge %q wider than %d cells", f.ID, v.ID, v.Badge, maxBadgeWidth)
			}
		}
	}
	sections := map[string]bool{}
	for _, s := range m.Sections {
		if err := checkID("section", s.ID, sections); err != nil {
			return err
		}
		if err := checkText("section "+s.ID+" title", s.Title); err != nil {
			return err
		}
	}
	actions := map[string]bool{}
	for _, a := range m.Actions {
		if err := checkID("action", a.ID, actions); err != nil {
			return err
		}
		if err := checkText("action "+a.ID+" label", a.Label); err != nil {
			return err
		}
		if a.Key == "" {
			return fmt.Errorf("action %s: empty key", a.ID)
		}
	}
	return nil
}

// checkText rejects manifest text bt draws that carries control characters,
// escape sequences included.
func checkText(what, s string) error {
	for _, r := range s {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s %q contains control characters", what, s)
		}
	}
	return nil
}

func checkID(kind, id string, seen map[string]bool) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%s id %q must match %s", kind, id, idPattern)
	}
	if seen[id] {
		return fmt.Errorf("duplicate %s id %q", kind, id)
	}
	seen[id] = true
	return nil
}
