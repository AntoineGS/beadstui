package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/debug"
)

// handle answers the plugin's requests and notifications. Notifications run
// on the connection's reader goroutine, so nothing here may wait on the UI
// except the ui.* requests, which run on their own goroutine.
func (s *session) handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if !s.active.Load() {
		return nil, PluginError("not_ready", "plugin is not initialized")
	}
	p := s.p
	switch method {
	case "state.set":
		var set StateSetParams
		if err := json.Unmarshal(params, &set); err != nil {
			return nil, s.invalidParams(method, err)
		}
		p.h.store.set(p.cfg.Name, set)
	case "state.clear":
		var clr StateClearParams
		if err := json.Unmarshal(params, &clr); err != nil {
			return nil, s.invalidParams(method, err)
		}
		p.h.store.clear(p.cfg.Name, clr.Beads)
	case "ui.toast":
		var t Toast
		if err := json.Unmarshal(params, &t); err != nil {
			return nil, s.invalidParams(method, err)
		}
		p.toast(t)
	case "ui.confirm":
		var c ConfirmParams
		if err := json.Unmarshal(params, &c); err != nil {
			return nil, s.invalidParams(method, err)
		}
		return p.prompt(ctx, PromptMsg{Plugin: p.cfg.Name, Confirm: &c})
	case "ui.select":
		var sel SelectParams
		if err := json.Unmarshal(params, &sel); err != nil {
			return nil, s.invalidParams(method, err)
		}
		return p.prompt(ctx, PromptMsg{Plugin: p.cfg.Name, Select: &sel})
	default:
		s.badLine(fmt.Errorf("unknown method %q", method))
		return nil, &RPCError{Code: -32601, Message: "method not found: " + method}
	}
	return nil, nil
}

func (s *session) invalidParams(method string, err error) error {
	s.badLine(fmt.Errorf("%s: invalid params: %w", method, err))
	return &RPCError{Code: -32602, Message: fmt.Sprintf("%s: invalid params: %v", method, err)}
}

// badLine counts a malformed line or unknown method; too many kill the
// plugin, which then counts as crashed.
func (s *session) badLine(err error) {
	debug.Log("plugin %s: %v", s.p.cfg.Name, err)
	if s.badLines.Add(1) == maxBadLines {
		s.killFor(fmt.Sprintf("killed after %d invalid messages: %v", maxBadLines, err))
	}
}

// toast forwards a ui.toast, dropping any that follow the previous one
// within the toast interval.
func (p *proc) toast(t Toast) {
	now := time.Now()
	p.mu.Lock()
	if !p.lastToast.IsZero() && now.Sub(p.lastToast) < p.h.toastInterval {
		p.mu.Unlock()
		debug.Log("plugin %s: toast dropped by rate limit", p.cfg.Name)
		return
	}
	p.lastToast = now
	p.mu.Unlock()
	p.h.emit(ToastMsg{Plugin: p.cfg.Name, Toast: t})
}

// prompt shows msg and waits for the answer. It is refused unless one of the
// plugin's actions is in flight, and cancelled when the last one ends or the
// connection closes.
func (p *proc) prompt(ctx context.Context, msg PromptMsg) (any, error) {
	p.mu.Lock()
	if p.inflight == 0 {
		p.mu.Unlock()
		return nil, PluginError("no_action", "prompts are only allowed while an action runs")
	}
	done := p.actionsDone
	p.mu.Unlock()

	answers := make(chan any, 1)
	var once sync.Once
	msg.Reply = func(answer any) {
		once.Do(func() { answers <- answer })
	}
	p.h.emit(msg)
	select {
	case answer := <-answers:
		return answer, nil
	case <-done:
		return nil, PluginError("cancelled", "the action ended")
	case <-ctx.Done():
		return nil, PluginError("cancelled", "the plugin connection closed")
	}
}
