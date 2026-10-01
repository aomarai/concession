package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aomarai/concession/internal/events"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/gin-gonic/gin"
)

// DefaultHeartbeat is how often an idle event stream sends a comment line, so
// proxies keep the connection open and the server notices lost clients.
const DefaultHeartbeat = 25 * time.Second

// EventsHandler streams a watchlist's live updates as server-sent events.
type EventsHandler struct {
	svc       *watchlist.Service
	hub       *events.Hub
	heartbeat time.Duration
}

// NewEventsHandler creates the handler; a non-positive heartbeat means
// DefaultHeartbeat.
func NewEventsHandler(svc *watchlist.Service, hub *events.Hub, heartbeat time.Duration) *EventsHandler {
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeat
	}
	return &EventsHandler{svc: svc, hub: hub, heartbeat: heartbeat}
}

// RegisterRoutes mounts the stream endpoint on r (authenticated group).
func (h *EventsHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/watchlists/:id/events", h.Stream)
}

// Stream serves GET /watchlists/:id/events. Anyone who may read the list may
// listen. Access is re-checked whenever an event is delivered and on every
// heartbeat, so a removed member (or everyone, once a public list turns
// private) stops receiving updates without reconnecting.
func (h *EventsHandler) Stream(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if err := h.svc.CanView(ctx, userID, id); err != nil {
		RespondServiceError(c, err)
		return
	}

	// Subscribe before announcing readiness so no event can slip in between.
	sub := h.hub.Subscribe(id)
	defer sub.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // do not let nginx buffer the stream
	c.Status(http.StatusOK)

	write := func(format string, args ...any) bool {
		if _, err := fmt.Fprintf(c.Writer, format, args...); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}
	if !write("retry: 5000\nevent: ready\ndata: {\"list_id\":%q}\n\n", id) {
		return
	}

	// revoked reports whether the user definitely lost access. Other errors
	// (a database hiccup) must not end the stream: the next check will tell.
	revoked := func() bool {
		err := h.svc.CanView(ctx, userID, id)
		if err != nil && !errors.Is(err, svcerr.ErrNotFound) {
			logging.FromContext(ctx).Warn("could not re-check access for an event stream", "error", err)
		}
		return errors.Is(err, svcerr.ErrNotFound)
	}

	ticker := time.NewTicker(h.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if revoked() || !write(": ping\n\n") {
				return
			}
		case ev, open := <-sub.C:
			if !open {
				return // the server is shutting down
			}
			// Everyone listening was authorized when they connected, so they are
			// told the list was deleted even though access is gone by now.
			if ev.Type != events.ListDeleted && revoked() {
				return // access was revoked
			}
			data, _ := json.Marshal(ev) // a plain struct always marshals
			if !write("id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, data) {
				return
			}
			if ev.Type == events.ListDeleted {
				return
			}
		}
		if sub.Lagged() && !write("event: resync\ndata: {}\n\n") {
			return
		}
	}
}
