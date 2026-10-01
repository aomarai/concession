package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/events"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

// sseMessage is one parsed server-sent event (comments become Name ":").
type sseMessage struct {
	Name string
	Data string
}

type sseStream struct {
	seen   []string
	t      *testing.T
	msgs   chan sseMessage
	cancel context.CancelFunc
	resp   *http.Response
}

// openStream connects to the running test server as the given user.
func (a *api) openStream(srv *httptest.Server, user uuid.UUID, listID uuid.UUID) *sseStream {
	a.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/watchlists/"+listID.String()+"/events", nil)
	req.Header.Set("X-Test-User", user.String())
	resp, err := srv.Client().Do(req)
	if err != nil {
		cancel()
		a.t.Fatal(err)
	}
	s := &sseStream{t: a.t, msgs: make(chan sseMessage, 256), cancel: cancel, resp: resp}
	if resp.StatusCode != http.StatusOK {
		return s // callers inspect resp for error cases
	}
	go func() {
		defer close(s.msgs)
		sc := bufio.NewScanner(resp.Body)
		var cur sseMessage
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if cur.Name != "" {
					s.msgs <- cur
				}
				cur = sseMessage{}
			case strings.HasPrefix(line, ":"):
				s.msgs <- sseMessage{Name: ":"}
			case strings.HasPrefix(line, "event: "):
				cur.Name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				cur.Data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	a.t.Cleanup(func() { cancel(); _ = resp.Body.Close() })
	return s
}

// next returns the next real event, skipping heartbeat comments.
func (s *sseStream) next() sseMessage {
	s.t.Helper()
	for {
		select {
		case m, ok := <-s.msgs:
			if !ok {
				s.t.Fatalf("stream ended; events seen so far: %v", s.seen)
			}
			if m.Name != ":" {
				s.seen = append(s.seen, m.Name)
				return m
			}
		case <-time.After(3 * time.Second):
			s.t.Fatal("timed out waiting for an event")
		}
	}
}

// expectEnd waits for the server to end the stream.
func (s *sseStream) expectEnd() {
	s.t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-s.msgs:
			if !ok {
				return
			}
		case <-deadline:
			s.t.Fatal("stream did not end")
		}
	}
}

func (s *sseStream) expect(name string) events.Event {
	s.t.Helper()
	m := s.next()
	if m.Name != name {
		s.t.Fatalf("got event %q (%s), want %q", m.Name, m.Data, name)
	}
	var ev events.Event
	if m.Data != "" && name != "ready" && name != "resync" {
		if err := json.Unmarshal([]byte(m.Data), &ev); err != nil {
			s.t.Fatalf("bad event data %q: %v", m.Data, err)
		}
	}
	return ev
}

func newStreamServer(t *testing.T) (*api, *httptest.Server) {
	t.Helper()
	a := newAPI(t)
	srv := httptest.NewServer(a.router)
	t.Cleanup(srv.Close)
	return a, srv
}

func waitSubscribers(t *testing.T, a *api, list uuid.UUID, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for a.hub.Subscribers(list) != want {
		if time.Now().After(deadline) {
			t.Fatalf("subscribers = %d, want %d", a.hub.Subscribers(list), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStreamDeliversEveryKindOfChange(t *testing.T) {
	a, srv := newStreamServer(t)
	owner, ann := a.newUser("owen"), a.newUser("ann")
	list := a.createList(owner, "movie")
	id := list.ID.String()

	s := a.openStream(srv, owner, list.ID)
	s.expect("ready")
	waitSubscribers(t, a, list.ID, 1)

	// Items.
	i1 := a.addItem(owner, list.ID, 10)
	ev := s.expect("item_added")
	if ev.ListID != list.ID || ev.ActorID != owner || ev.ItemID == nil || *ev.ItemID != i1.ID || ev.Type != "item_added" {
		t.Errorf("item_added: %+v", ev)
	}
	i2 := a.addItem(owner, list.ID, 20)
	s.expect("item_added")
	a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+id+"/items/"+i1.ID.String(), map[string]any{"notes": "n"}), http.StatusOK, "")
	if ev := s.expect("item_updated"); *ev.ItemID != i1.ID {
		t.Errorf("item_updated: %+v", ev)
	}
	a.expect(a.do(owner, http.MethodPut, "/watchlists/"+id+"/items/order", map[string]any{"item_ids": []uuid.UUID{i2.ID, i1.ID}}), http.StatusNoContent, "")
	s.expect("items_reordered")
	a.expect(a.do(owner, http.MethodDelete, "/watchlists/"+id+"/items/"+i2.ID.String(), nil), http.StatusNoContent, "")
	if ev := s.expect("item_removed"); *ev.ItemID != i2.ID {
		t.Errorf("item_removed: %+v", ev)
	}

	// List metadata.
	a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+id, map[string]any{"title": "New"}), http.StatusOK, "")
	s.expect("list_updated")

	// Membership: invite, accept, role change, decline, removal.
	a.expect(a.do(owner, http.MethodPost, "/watchlists/"+id+"/collaborators", map[string]any{"user": "ann", "role": "editor"}), http.StatusCreated, "")
	s.expect("members_changed")
	inv := decode[invitesBody](t, a.do(ann, http.MethodGet, "/me/invites", nil))
	a.expect(a.do(ann, http.MethodPost, "/invites/"+inv.Invites[0].ID.String()+"/accept", nil), http.StatusNoContent, "")
	if ev := s.expect("members_changed"); ev.ActorID != ann {
		t.Errorf("accept actor: %+v", ev)
	}
	a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+id+"/collaborators/"+ann.String(), map[string]any{"role": "viewer"}), http.StatusNoContent, "")
	s.expect("members_changed")
	a.expect(a.do(ann, http.MethodDelete, "/watchlists/"+id+"/collaborators/"+ann.String(), nil), http.StatusNoContent, "")
	s.expect("members_changed")

	cy := a.newUser("cy")
	a.expect(a.do(owner, http.MethodPost, "/watchlists/"+id+"/collaborators", map[string]any{"user": "cy", "role": "viewer"}), http.StatusCreated, "")
	s.expect("members_changed")
	cyInv := decode[invitesBody](t, a.do(cy, http.MethodGet, "/me/invites", nil))
	a.expect(a.do(cy, http.MethodPost, "/invites/"+cyInv.Invites[0].ID.String()+"/decline", nil), http.StatusNoContent, "")
	s.expect("members_changed")

	// Deleting the list is announced and then the stream ends.
	a.expect(a.do(owner, http.MethodDelete, "/watchlists/"+id, nil), http.StatusNoContent, "")
	s.expect("list_deleted")
	s.expectEnd()
	waitSubscribers(t, a, list.ID, 0)
}

func TestStreamAuthorization(t *testing.T) {
	a, srv := newStreamServer(t)
	owner, stranger, viewer := a.newUser("owen"), a.newUser("sam"), a.newUser("vic")
	list := a.createList(owner, "movie")
	a.db.Create(&domain.Collaborator{UserID: viewer, WatchlistID: list.ID, Role: domain.RoleViewer})

	// Strangers get the same 404 as for any private list; no stream is opened.
	s := a.openStream(srv, stranger, list.ID)
	if s.resp.StatusCode != http.StatusNotFound {
		t.Errorf("stranger status = %d", s.resp.StatusCode)
	}
	if a.hub.Subscribers(list.ID) != 0 {
		t.Error("a rejected request must not subscribe")
	}
	if r := a.openStream(srv, owner, uuid.New()); r.resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown list status = %d", r.resp.StatusCode)
	}

	// Viewers may listen.
	vs := a.openStream(srv, viewer, list.ID)
	if vs.resp.StatusCode != http.StatusOK || !strings.HasPrefix(vs.resp.Header.Get("Content-Type"), "text/event-stream") ||
		vs.resp.Header.Get("Cache-Control") != "no-cache" || vs.resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Errorf("viewer: %d %v", vs.resp.StatusCode, vs.resp.Header)
	}
	vs.expect("ready")

	// Anyone may listen to a public list...
	public := "public"
	a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+list.ID.String(), map[string]any{"privacy": public}), http.StatusOK, "")
	ps := a.openStream(srv, stranger, list.ID)
	ps.expect("ready")
	waitSubscribers(t, a, list.ID, 2) // viewer + stranger

	// ...until it turns private again: the next event ends their stream but not the viewer's.
	a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+list.ID.String(), map[string]any{"privacy": "private"}), http.StatusOK, "")
	ps.expectEnd()
	vs.expect("list_updated")
	waitSubscribers(t, a, list.ID, 1)
}

func TestRemovedMembersStopReceivingUpdates(t *testing.T) {
	a, srv := newStreamServer(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	editor := a.newUser("ed")
	a.db.Create(&domain.Collaborator{UserID: editor, WatchlistID: list.ID, Role: domain.RoleEditor})

	es := a.openStream(srv, editor, list.ID)
	es.expect("ready")
	os := a.openStream(srv, owner, list.ID)
	os.expect("ready")
	waitSubscribers(t, a, list.ID, 2)

	a.expect(a.do(owner, http.MethodDelete, "/watchlists/"+list.ID.String()+"/collaborators/"+editor.String(), nil), http.StatusNoContent, "")
	os.expect("members_changed")
	es.expectEnd() // the removal event itself fails the editor's access re-check
	waitSubscribers(t, a, list.ID, 1)
}

func TestStreamHeartbeats(t *testing.T) {
	a, srv := newStreamServer(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	s := a.openStream(srv, owner, list.ID)
	// next() hides comments, so read the channel directly for a heartbeat.
	deadline := time.After(3 * time.Second)
	for {
		select {
		case m := <-s.msgs:
			if m.Name == ":" {
				return
			}
		case <-deadline:
			t.Fatal("no heartbeat arrived")
		}
	}
}

func TestHeartbeatEndsStreamWhenAccessIsLost(t *testing.T) {
	a, srv := newStreamServer(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	s := a.openStream(srv, owner, list.ID)
	s.expect("ready")
	// Delete the list behind the hub's back (no event), so only the heartbeat
	// re-check can notice.
	if err := a.db.Unscoped().Delete(&domain.Watchlist{}, list.ID).Error; err != nil {
		t.Fatal(err)
	}
	s.expectEnd()
}

func TestClientDisconnectUnsubscribes(t *testing.T) {
	a, srv := newStreamServer(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	s := a.openStream(srv, owner, list.ID)
	s.expect("ready")
	waitSubscribers(t, a, list.ID, 1)
	s.cancel()
	waitSubscribers(t, a, list.ID, 0)
}

func TestHubCloseEndsOpenStreams(t *testing.T) {
	a, srv := newStreamServer(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	s := a.openStream(srv, owner, list.ID)
	s.expect("ready")
	a.hub.Close()
	s.expectEnd()
}

func TestStreamErrors(t *testing.T) {
	a := newAPI(t)
	u := a.newUser("ann")
	a.expect(a.do(u, http.MethodGet, "/watchlists/nope/events", nil), http.StatusBadRequest, "bad_request")
	a.expect(a.do(uuid.Nil, http.MethodGet, "/watchlists/"+uuid.NewString()+"/events", nil), http.StatusInternalServerError, "internal_error")

	list := a.createList(u, "movie")
	testutil.FailOn(t, a.db, "query", "watchlists")
	a.expect(a.do(u, http.MethodGet, "/watchlists/"+list.ID.String()+"/events", nil), http.StatusInternalServerError, "internal_error")
}

func TestNewEventsHandlerDefaultsTheHeartbeat(t *testing.T) {
	h := NewEventsHandler(nil, events.NewHub(), 0)
	if h.heartbeat != DefaultHeartbeat {
		t.Errorf("heartbeat = %v", h.heartbeat)
	}
	if NewEventsHandler(nil, events.NewHub(), time.Second).heartbeat != time.Second {
		t.Error("an explicit heartbeat is kept")
	}
}

// gatedWriter lets the first n writes through and then blocks until released,
// simulating a client that stops reading.
type gatedWriter struct {
	*httptest.ResponseRecorder
	mu      sync.Mutex
	allowed int
	gate    chan struct{}
	body    strings.Builder
	// failAfterGate, when >= 0, is how many writes succeed once the gate opens
	// before writes start failing; -1 never fails.
	failAfterGate int
}

func (g *gatedWriter) Write(b []byte) (int, error) {
	g.mu.Lock()
	if g.allowed > 0 {
		g.allowed--
		g.mu.Unlock()
	} else {
		g.mu.Unlock()
		<-g.gate
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.failAfterGate == 0 {
		return 0, http.ErrAbortHandler
	}
	if g.failAfterGate > 0 {
		g.failAfterGate--
	}
	g.body.Write(b)
	return len(b), nil
}

func (g *gatedWriter) Flush() {}

func (g *gatedWriter) text() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.body.String()
}

func TestSlowClientsAreToldToResync(t *testing.T) {
	a := newAPI(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/watchlists/"+list.ID.String()+"/events", nil).WithContext(ctx)
	req.Header.Set("X-Test-User", owner.String())
	w := &gatedWriter{ResponseRecorder: httptest.NewRecorder(), allowed: 1, gate: make(chan struct{}), failAfterGate: -1}

	done := make(chan struct{})
	go func() { defer close(done); a.router.ServeHTTP(w, req) }()
	waitSubscribers(t, a, list.ID, 1)

	// The handler is blocked writing the first event; flood the hub past the buffer.
	for i := 0; i < 60; i++ {
		a.hub.Publish(events.Event{Type: events.ItemAdded, ListID: list.ID, ActorID: owner})
	}
	close(w.gate) // the client starts reading again
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(w.text(), "event: resync") {
		if time.Now().After(deadline) {
			t.Fatalf("no resync event in %q", w.text())
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestStreamEndsWhenTheClientCannotBeWrittenTo(t *testing.T) {
	a := newAPI(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/watchlists/"+list.ID.String()+"/events", nil)
	req.Header.Set("X-Test-User", owner.String())

	// Fail the very first write (the "ready" event).
	w := &failingWriter{ResponseRecorder: httptest.NewRecorder(), failAfter: 0}
	done := make(chan struct{})
	go func() { defer close(done); a.router.ServeHTTP(w, req) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return after a failed write")
	}
	waitSubscribers(t, a, list.ID, 0)

	// Fail a later write: the "ready" event passes, then an event write fails.
	w2 := &failingWriter{ResponseRecorder: httptest.NewRecorder(), failAfter: 1}
	done2 := make(chan struct{})
	go func() { defer close(done2); a.router.ServeHTTP(w2, req) }()
	waitSubscribers(t, a, list.ID, 1)
	a.hub.Publish(events.Event{Type: events.ItemAdded, ListID: list.ID, ActorID: owner})
	select {
	case <-done2:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return after a failed event write")
	}

	// And a failed heartbeat write.
	w3 := &failingWriter{ResponseRecorder: httptest.NewRecorder(), failAfter: 1}
	done3 := make(chan struct{})
	go func() { defer close(done3); a.router.ServeHTTP(w3, req) }()
	select {
	case <-done3:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return after a failed heartbeat write")
	}
}

type failingWriter struct {
	*httptest.ResponseRecorder
	mu        sync.Mutex
	failAfter int
}

func (f *failingWriter) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAfter <= 0 {
		return 0, http.ErrAbortHandler
	}
	f.failAfter--
	return f.ResponseRecorder.Write(b)
}

func (f *failingWriter) Flush() {}

func TestTransientDatabaseErrorsDoNotEndStreams(t *testing.T) {
	a, srv := newStreamServer(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	s := a.openStream(srv, owner, list.ID)
	s.expect("ready")
	waitSubscribers(t, a, list.ID, 1)

	// Every access re-check now fails with a database error, not "not found".
	testutil.FailOn(t, a.db, "query", "watchlists")
	a.hub.Publish(events.Event{Type: events.ItemAdded, ListID: list.ID, ActorID: owner})
	s.expect("item_added")            // still delivered
	time.Sleep(80 * time.Millisecond) // a couple of heartbeats, which also re-check
	a.hub.Publish(events.Event{Type: events.ItemUpdated, ListID: list.ID, ActorID: owner})
	s.expect("item_updated")
}

func TestStreamEndsWhenTheResyncNoticeCannotBeWritten(t *testing.T) {
	a := newAPI(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/watchlists/"+list.ID.String()+"/events", nil)
	req.Header.Set("X-Test-User", owner.String())

	// "ready" passes, the first event write blocks until released and then
	// succeeds, and the resync notice that follows fails (the count includes
	// the "ready" write, which also goes through the failure counter).
	w := &gatedWriter{ResponseRecorder: httptest.NewRecorder(), allowed: 1, gate: make(chan struct{}), failAfterGate: 2}
	done := make(chan struct{})
	go func() { defer close(done); a.router.ServeHTTP(w, req) }()
	waitSubscribers(t, a, list.ID, 1)
	for i := 0; i < 60; i++ {
		a.hub.Publish(events.Event{Type: events.ItemAdded, ListID: list.ID, ActorID: owner})
	}
	close(w.gate)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not return after the resync write failed")
	}
}
