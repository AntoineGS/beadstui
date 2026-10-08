package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

// maxLine is the longest JSON-RPC message accepted, in bytes.
const maxLine = 4 << 20

// writeQueue is how many outgoing messages may wait for the writer goroutine.
const writeQueue = 256

// notifyTimeout bounds how long Notify and replies wait for queue space.
const notifyTimeout = 2 * time.Second

// CodePluginError is the JSON-RPC error code for plugin-defined failures.
const CodePluginError = -32000

// ErrClosed is returned by calls on a connection whose peer went away.
var ErrClosed = errors.New("plugin connection closed")

// Handler answers a request or handles a notification from the peer.
type Handler func(ctx context.Context, method string, params json.RawMessage) (any, error)

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

// Type returns data.type of a plugin error, or "".
func (e *RPCError) Type() string {
	var d struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(e.Data, &d)
	return d.Type
}

// PluginError builds a -32000 error with data.type set.
func PluginError(typ, message string) *RPCError {
	data, _ := json.Marshal(map[string]string{"type": typ})
	return &RPCError{Code: CodePluginError, Message: message, Data: data}
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type reply struct {
	result json.RawMessage
	err    error
}

// Conn is one JSON-RPC 2.0 connection over newline-delimited JSON.
type Conn struct {
	r         io.Reader
	h         Handler
	onBadLine func(error)

	w         io.Writer
	out       chan []byte
	done      chan struct{}
	startOnce sync.Once

	mu      sync.Mutex
	nextID  int64
	pending map[string]chan reply
	closed  bool
}

// NewConn returns a connection reading r and writing w. h handles the peer's
// requests and notifications and may be nil. onBadLine, when set, is called
// for every line that is not a valid message. Writes go through a queue
// drained by a single goroutine, so a peer that stops reading never blocks
// callers beyond their own context or timeout.
func NewConn(r io.Reader, w io.Writer, h Handler, onBadLine func(error)) *Conn {
	return &Conn{
		r: r, w: w, h: h, onBadLine: onBadLine,
		out:     make(chan []byte, writeQueue),
		done:    make(chan struct{}),
		pending: map[string]chan reply{},
	}
}

// Run reads messages until r ends or ctx is done. When ctx is done, Run
// closes r if it implements io.Closer, fails every pending call with
// ErrClosed and returns ctx.Err() without waiting for a blocked read.
// However Run ends, every pending and future call fails with ErrClosed.
func (c *Conn) Run(ctx context.Context) error {
	defer c.close()
	res := make(chan error, 1)
	go func() { res <- c.readLoop(ctx) }()
	select {
	case err := <-res:
		return err
	case <-ctx.Done():
		if cl, ok := c.r.(io.Closer); ok {
			_ = cl.Close()
		}
		return ctx.Err()
	}
}

func (c *Conn) readLoop(ctx context.Context) error {
	sc := bufio.NewScanner(c.r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var m message
		if err := json.Unmarshal(line, &m); err != nil || m.JSONRPC != "2.0" {
			if err == nil {
				err = errors.New("missing jsonrpc 2.0")
			}
			c.bad(fmt.Errorf("invalid message: %w", err))
			continue
		}
		c.dispatch(ctx, m)
	}
	if err := sc.Err(); err != nil {
		c.bad(fmt.Errorf("read: %w", err))
		return err
	}
	return nil
}

func (c *Conn) bad(err error) {
	if c.onBadLine != nil {
		c.onBadLine(err)
	}
}

func (c *Conn) dispatch(ctx context.Context, m message) {
	switch {
	case m.Method == "" && len(m.ID) > 0: // response
		c.mu.Lock()
		ch, ok := c.pending[string(m.ID)]
		delete(c.pending, string(m.ID))
		issued := c.nextID
		c.mu.Unlock()
		if !ok {
			// A late answer to a call that already timed out is not a bad line.
			if n, err := strconv.ParseInt(string(m.ID), 10, 64); err != nil || n < 1 || n > issued {
				c.bad(fmt.Errorf("response to unknown id %s", m.ID))
			}
			return
		}
		if m.Error != nil {
			ch <- reply{err: m.Error}
		} else {
			ch <- reply{result: m.Result}
		}
	case m.Method != "" && len(m.ID) > 0: // request
		go func() {
			result, err := c.handle(ctx, m.Method, m.Params)
			resp := message{JSONRPC: "2.0", ID: m.ID}
			if err != nil {
				var rpcErr *RPCError
				if !errors.As(err, &rpcErr) {
					rpcErr = &RPCError{Code: -32603, Message: err.Error()}
				}
				resp.Error = rpcErr
			} else {
				b, mErr := json.Marshal(result)
				if mErr != nil {
					resp.Error = &RPCError{Code: -32603, Message: mErr.Error()}
				} else {
					resp.Result = b
				}
			}
			rctx, cancel := context.WithTimeout(ctx, notifyTimeout)
			defer cancel()
			_ = c.send(rctx, resp)
		}()
	case m.Method != "": // notification
		_, _ = c.handle(ctx, m.Method, m.Params)
	default:
		c.bad(errors.New("message is neither request, response nor notification"))
	}
}

func (c *Conn) handle(ctx context.Context, method string, params json.RawMessage) (result any, err error) {
	if c.h == nil {
		return nil, &RPCError{Code: -32601, Message: "method not found: " + method}
	}
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, &RPCError{Code: -32603, Message: "internal error"}
		}
	}()
	return c.h(ctx, method, params)
}

// send queues m for the writer goroutine, waiting for space until ctx is
// done or the connection closes.
func (c *Conn) send(ctx context.Context, m message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.startOnce.Do(func() { go c.writeLoop() })
	select {
	case <-c.done:
		return ErrClosed
	default:
	}
	select {
	case c.out <- b:
		return nil
	case <-c.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// writeLoop is the only goroutine that writes to w. A write error closes the
// connection.
func (c *Conn) writeLoop() {
	for {
		select {
		case b := <-c.out:
			if _, err := c.w.Write(b); err != nil {
				c.close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// Notify sends a notification. It fails if the outgoing queue stays full for
// two seconds or the connection is closed.
func (c *Conn) Notify(method string, params any) error {
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	if err := c.send(ctx, message{JSONRPC: "2.0", Method: method, Params: p}); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}

// Call sends a request and waits for its response, ctx, or the connection
// closing. result may be nil.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.nextID++
	id := json.RawMessage(strconv.FormatInt(c.nextID, 10))
	ch := make(chan reply, 1)
	c.pending[string(id)] = ch
	c.mu.Unlock()

	if err := c.send(ctx, message{JSONRPC: "2.0", ID: id, Method: method, Params: p}); err != nil {
		c.forget(id)
		return fmt.Errorf("%s: %w", method, err)
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if result != nil && len(r.result) > 0 {
			return json.Unmarshal(r.result, result)
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	}
}

func (c *Conn) forget(id json.RawMessage) {
	c.mu.Lock()
	delete(c.pending, string(id))
	c.mu.Unlock()
}

func (c *Conn) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.done)
	for id, ch := range c.pending {
		ch <- reply{err: ErrClosed}
		delete(c.pending, id)
	}
}
