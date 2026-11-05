package nativeui

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// fakeControl answers the native control like the App: prepare-edge gives an
// offer, everything else an empty success.
type fakeControl struct {
	mu       sync.Mutex
	requests []string
	events   chan any
}

func (f *fakeControl) Handle(_ context.Context, payload []byte) (any, error) {
	var request struct {
		ID           string `json:"id"`
		Type         string `json:"type"`
		ConnectionID string `json:"connectionId"`
	}
	_ = json.Unmarshal(payload, &request)
	f.mu.Lock()
	f.requests = append(f.requests, request.Type+":"+request.ConnectionID)
	f.mu.Unlock()
	if request.Type == "prepare-edge" {
		return map[string]any{"version": 10, "id": request.ID, "type": "edge-offer", "connectionId": request.ConnectionID, "sdp": "v=0"}, nil
	}
	return map[string]any{"version": 10, "id": request.ID, "type": "ok"}, nil
}
func (f *fakeControl) Events() <-chan any { return f.events }
func (f *fakeControl) Close() error       { return nil }

func (f *fakeControl) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, request := range f.requests {
		if len(request) >= len(prefix) && request[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}

func liveHost(t *testing.T) (*hostSession, *fakeControl) {
	control := &fakeControl{events: make(chan any)}
	h := &hostSession{env: HostEnv{}, invalidate: func() {}, ctx: context.Background(), cancel: func() {},
		phase: phaseLive, capacity: 2, edges: map[string]*edge{}, active: map[string]string{},
		share: "share-generation-1", serverShare: "share-generation-1"}
	h.sender = func(map[string]any) bool { return true }
	h.voice = newVoiceEndpoint(h)
	h.control = newControlClient(control, nil)
	t.Cleanup(h.control.Close)
	return h, control
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestReconcileServesAssignedChildrenUpToCapacity(t *testing.T) {
	h, control := liveHost(t)
	h.reconcile([]string{"viewer-a", "viewer-b", "viewer-c"}, 3)
	waitFor(t, "two offers", func() bool { return control.count("prepare-edge") == 2 })
	h.mu.Lock()
	if len(h.active) != 2 || h.active["viewer-c"] != "" {
		t.Fatalf("active %v", h.active)
	}
	h.mu.Unlock()
	// viewer-a leaves: its edge closes, viewer-b keeps its own.
	h.reconcile([]string{"viewer-b"}, 4)
	waitFor(t, "close-edge", func() bool { return control.count("close-edge") == 1 })
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.active) != 1 || h.active["viewer-b"] == "" {
		t.Fatalf("active %v", h.active)
	}
}

func TestRouteUpdatesPromoteThePreparedChild(t *testing.T) {
	h, control := liveHost(t)
	prepare := serverMessage{Type: "route-update", Revision: 5, Phase: "prepare", Assignment: routeAssignment{ChildPeerIDs: []string{"viewer-a"}}}
	prepare.Candidate.ChildPeerID, prepare.Candidate.ConnectionID, prepare.Candidate.Transport = "viewer-a", "route_abcdefgh12345678", "direct"
	h.routeUpdate(prepare)
	h.routeUpdate(prepare) // duplicate: ignored
	waitFor(t, "prepared offer", func() bool { return control.count("prepare-edge:route_abcdefgh12345678") == 1 })
	h.routeUpdate(serverMessage{Type: "route-update", Revision: 5, Phase: "active", Assignment: routeAssignment{ChildPeerIDs: []string{"viewer-a"}}})
	h.mu.Lock()
	connection := h.active["viewer-a"]
	prepared := h.prepared
	h.mu.Unlock()
	if connection != "route_abcdefgh12345678" || prepared != nil {
		t.Fatalf("active %q prepared %v", connection, prepared)
	}
	// A stale revision changes nothing.
	h.routeUpdate(serverMessage{Type: "route-update", Revision: 4, Phase: "active"})
	time.Sleep(50 * time.Millisecond)
	if control.count("prepare-edge") != 1 || control.count("close-edge") != 0 {
		t.Fatalf("requests %v", control.requests)
	}
}

func TestPreparePlanRejectsTooManyNewChildren(t *testing.T) {
	h, _ := liveHost(t)
	if h.preparePlanValid([]string{"a", "b"}, "a", "route_x") {
		t.Fatal("two new children at once must be refused")
	}
	if !h.preparePlanValid([]string{"a"}, "a", "route_x") {
		t.Fatal("one new child must be accepted")
	}
	if h.preparePlanValid([]string{"a", "b", "c"}, "a", "route_x") {
		t.Fatal("over capacity must be refused")
	}
}

func TestParticipantColorIsStable(t *testing.T) {
	a, b := participantColor("viewer-aaaaaaaa"), participantColor("viewer-aaaaaaaa")
	if a != b || a.A != 255 {
		t.Fatalf("%v %v", a, b)
	}
}
