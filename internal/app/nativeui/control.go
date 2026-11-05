package nativeui

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/TNTcraftHIM/Piik/internal/app/loopback"
)

// ControlSession is loopback.ControlSession: the App's native capture and
// media control, used in-process with the same JSON messages the web page
// sends over the loopback WebSocket.
type ControlSession interface {
	Handle(context.Context, []byte) (any, error)
	Events() <-chan any
	Close() error
}

const controlVersion = 10 // loopback.ProtocolVersion

type controlFailure struct {
	Type string
	Code string
}

func (f controlFailure) Error() string { return fmt.Sprintf("native control %s: %s", f.Type, f.Code) }

// controlClient adds request IDs, waits for responses completed later through
// Events, and hands every other event to onEvent.
type controlClient struct {
	session ControlSession
	onEvent func(kind string, raw json.RawMessage)

	mu      sync.Mutex
	pending map[string]chan json.RawMessage
	closed  chan struct{}
	once    sync.Once
	// life is the connection's context: commands such as start-share keep
	// their capture running until it ends, like the loopback socket's.
	life   context.Context
	cancel context.CancelFunc
}

func newControlClient(session ControlSession, onEvent func(string, json.RawMessage)) *controlClient {
	life, cancel := context.WithCancel(context.Background())
	client := &controlClient{session: session, onEvent: onEvent, pending: map[string]chan json.RawMessage{}, closed: make(chan struct{}),
		life: life, cancel: cancel}
	go client.pump()
	return client
}

func randomID(bytes int) string {
	buffer := make([]byte, bytes)
	_, _ = rand.Read(buffer)
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func (c *controlClient) pump() {
	for {
		select {
		case <-c.closed:
			return
		case event, ok := <-c.session.Events():
			if !ok {
				return
			}
			value, current := unwrapEvent(event)
			if !current {
				continue
			}
			raw, err := json.Marshal(value)
			if err != nil {
				continue
			}
			var head struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			}
			_ = json.Unmarshal(raw, &head)
			if head.ID != "" {
				c.mu.Lock()
				waiter := c.pending[head.ID]
				delete(c.pending, head.ID)
				c.mu.Unlock()
				if waiter != nil {
					waiter <- raw
					continue
				}
			}
			if c.onEvent != nil {
				c.onEvent(head.Type, raw)
			}
		}
	}
}

// request sends message (a struct or map without version/id) and returns the
// response JSON. Responses of type "request-failed" become controlFailure.
func (c *controlClient) request(ctx context.Context, kind string, fields map[string]any) (json.RawMessage, error) {
	id := randomID(12)
	message := map[string]any{"version": controlVersion, "id": id, "type": kind}
	for key, value := range fields {
		message[key] = value
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	waiter := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[id] = waiter
	c.mu.Unlock()
	result, err := c.session.Handle(c.life, payload)
	if err != nil {
		c.forget(id)
		return nil, err
	}
	var raw json.RawMessage
	if result != nil {
		c.forget(id)
		value, _ := unwrapEvent(result)
		if raw, err = json.Marshal(value); err != nil {
			return nil, err
		}
	} else {
		select {
		case raw = <-waiter:
		case <-ctx.Done():
			c.forget(id)
			return nil, ctx.Err()
		case <-c.closed:
			return nil, errors.New("native control closed")
		}
	}
	var head struct {
		Type string `json:"type"`
		Code string `json:"code"`
	}
	_ = json.Unmarshal(raw, &head)
	if head.Type == "request-failed" {
		return raw, controlFailure{Type: kind, Code: head.Code}
	}
	return raw, nil
}

// unwrapEvent applies loopback.ControlEvent's checks like the WebSocket writer.
func unwrapEvent(value any) (any, bool) {
	if event, ok := value.(loopback.ControlEvent); ok {
		if event.Current != nil && !event.Current() {
			return nil, false
		}
		if event.BeforeWrite != nil {
			event.BeforeWrite()
		}
		return event.Value, true
	}
	return value, true
}

func (c *controlClient) forget(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *controlClient) Close() {
	c.once.Do(func() {
		close(c.closed)
		c.cancel()
		_ = c.session.Close()
	})
}
