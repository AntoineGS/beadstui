package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// pipePair returns two connected Conns.
func pipePair(t *testing.T, ha, hb Handler) (*Conn, *Conn) {
	t.Helper()
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	a := NewConn(ar, aw, ha, nil)
	b := NewConn(br, bw, hb, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); aw.Close(); bw.Close() })
	go a.Run(ctx)
	go b.Run(ctx)
	return a, b
}

func TestCallRoundTrip(t *testing.T) {
	echo := func(_ context.Context, method string, p json.RawMessage) (any, error) {
		if method != "echo" {
			return nil, &RPCError{Code: -32601, Message: "no"}
		}
		var v map[string]string
		_ = json.Unmarshal(p, &v)
		return v, nil
	}
	a, _ := pipePair(t, nil, echo)
	var out map[string]string
	if err := a.Call(context.Background(), "echo", map[string]string{"x": "1"}, &out); err != nil || out["x"] != "1" {
		t.Fatalf("Call = %v, %v", out, err)
	}
}

func TestCallPluginError(t *testing.T) {
	fail := func(context.Context, string, json.RawMessage) (any, error) {
		return nil, PluginError("no_repo", "no checkout")
	}
	a, _ := pipePair(t, nil, fail)
	err := a.Call(context.Background(), "x", nil, nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != CodePluginError || rpcErr.Message != "no checkout" {
		t.Fatalf("err = %v", err)
	}
	if rpcErr.Type() != "no_repo" {
		t.Fatalf("Type() = %q", rpcErr.Type())
	}
}

func TestNotificationsKeepOrder(t *testing.T) {
	var mu sync.Mutex
	var got []int
	done := make(chan struct{})
	h := func(_ context.Context, _ string, p json.RawMessage) (any, error) {
		var n int
		_ = json.Unmarshal(p, &n)
		mu.Lock()
		got = append(got, n)
		if len(got) == 50 {
			close(done)
		}
		mu.Unlock()
		return nil, nil
	}
	a, _ := pipePair(t, nil, h)
	for i := 0; i < 50; i++ {
		if err := a.Notify("n", i); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("notifications not delivered")
	}
	for i, n := range got {
		if n != i {
			t.Fatalf("order broken at %d: %v", i, got)
		}
	}
}

func TestRequestHandlerDoesNotBlockReader(t *testing.T) {
	release := make(chan struct{})
	h := func(_ context.Context, method string, _ json.RawMessage) (any, error) {
		if method == "slow" {
			<-release
		}
		return "ok", nil
	}
	a, _ := pipePair(t, nil, h)
	go a.Call(context.Background(), "slow", nil, nil)
	var out string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.Call(ctx, "fast", nil, &out); err != nil || out != "ok" {
		t.Fatalf("fast call blocked behind slow one: %v", err)
	}
	close(release)
}

func TestCallTimeoutAndClose(t *testing.T) {
	block := make(chan struct{})
	h := func(context.Context, string, json.RawMessage) (any, error) { <-block; return nil, nil }
	a, b := pipePair(t, nil, h)
	_ = b
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := a.Call(ctx, "x", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
	close(block)
}

func TestBadLinesReported(t *testing.T) {
	r, w := io.Pipe()
	var mu sync.Mutex
	bad := 0
	c := NewConn(r, io.Discard, nil, func(error) { mu.Lock(); bad++; mu.Unlock() })
	done := make(chan error)
	go func() { done <- c.Run(context.Background()) }()
	_, _ = w.Write([]byte("not json\n{\"jsonrpc\":\"2.0\",\"method\":\"x\"}\n{]\n"))
	w.Close()
	<-done
	if bad != 2 {
		t.Fatalf("bad lines = %d, want 2", bad)
	}
	if err := c.Call(context.Background(), "x", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("Call after close = %v, want ErrClosed", err)
	}
}
