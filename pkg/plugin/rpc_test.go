package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
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
	slowErr := make(chan error, 1)
	go func() { slowErr <- a.Call(context.Background(), "slow", nil, nil) }()
	var out string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.Call(ctx, "fast", nil, &out); err != nil || out != "ok" {
		t.Fatalf("fast call blocked behind slow one: %v", err)
	}
	close(release)
	select {
	case err := <-slowErr:
		if err != nil {
			t.Fatalf("slow call = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow call did not finish after release")
	}
}

func TestCallFailsWhenPeerCloses(t *testing.T) {
	r, w := io.Pipe()
	c := NewConn(r, io.Discard, nil, nil)
	go c.Run(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- c.Call(context.Background(), "x", nil, nil) }()
	time.Sleep(50 * time.Millisecond)
	w.Close()
	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("err = %v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight Call did not return after peer closed")
	}
}

func TestLateResponseIsNotBadLine(t *testing.T) {
	r, w := io.Pipe()
	bad := make(chan error, 8)
	c := NewConn(r, io.Discard, nil, func(err error) { bad <- err })
	go c.Run(context.Background())
	t.Cleanup(func() { w.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Call(ctx, "x", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
	// Late answer to id 1, then ids never issued; lines are handled in order,
	// so the first bad line reported must come from id 999.
	_, _ = w.Write([]byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":1}\n{\"jsonrpc\":\"2.0\",\"id\":999,\"result\":1}\n{\"jsonrpc\":\"2.0\",\"id\":null,\"result\":1}\n"))
	for i, want := range []string{"id 999", "id null"} {
		select {
		case err := <-bad:
			if !strings.Contains(err.Error(), want[3:]) {
				t.Fatalf("bad line %d = %v, want it to mention %q", i, err, want[3:])
			}
		case <-time.After(time.Second):
			t.Fatalf("bad line %d (%s) not reported", i, want)
		}
	}
	select {
	case err := <-bad:
		t.Fatalf("unexpected extra bad line: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
}

// blockingReader blocks in Read until released and is not an io.Closer.
type blockingReader struct{ release chan struct{} }

func (b blockingReader) Read([]byte) (int, error) {
	<-b.release
	return 0, io.EOF
}

func TestRunCancelFailsPendingCall(t *testing.T) {
	br := blockingReader{release: make(chan struct{})}
	t.Cleanup(func() { close(br.release) })
	c := NewConn(br, io.Discard, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- c.Run(ctx) }()
	callErr := make(chan error, 1)
	go func() { callErr <- c.Call(context.Background(), "x", nil, nil) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-callErr:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Call = %v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending Call not failed after Run ctx cancelled")
	}
	select {
	case err := <-runErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancelled")
	}
}

func TestStuckPeerDoesNotHangCallers(t *testing.T) {
	r, _ := io.Pipe()
	pr, pw := io.Pipe() // nobody reads pr: every write blocks
	t.Cleanup(func() { pr.Close() })
	c := NewConn(r, pw, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := c.Call(ctx, "x", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call = %v, want deadline", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Call took %v", d)
	}

	start = time.Now()
	var err error
	for i := 0; i < writeQueue+10 && err == nil; i++ {
		err = c.Notify("n", i)
	}
	if err == nil {
		t.Fatal("Notify never failed against a stuck peer")
	}
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Fatalf("Notify took %v", d)
	}
}

func TestUnknownMethodRequestGetsMethodNotFound(t *testing.T) {
	a, _ := pipePair(t, nil, nil)
	err := a.Call(context.Background(), "nope", nil, nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("err = %v, want code -32601", err)
	}
}

func TestHandlerPanicIsRecovered(t *testing.T) {
	h := func(context.Context, string, json.RawMessage) (any, error) { panic("boom") }
	a, _ := pipePair(t, nil, h)
	err := a.Call(context.Background(), "x", nil, nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32603 || rpcErr.Message != "internal error" {
		t.Fatalf("err = %v, want -32603 internal error", err)
	}
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
