package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/tailnet"
)

// keepalive bounds how long a proxy sees an idle SSE connection. Many
// reverse proxies close one after 60s of silence; a comment frame costs
// nothing and keeps the stream open on a quiet tailnet.
const keepalive = 25 * time.Second

// eventPayload is what a browser receives when the tailnet changes. It is
// deliberately a notification, not the data: the SPA refetches the queries it
// actually has open, so a member never receives the whole tailnet over a
// channel that was opened for their own two devices.
type eventPayload struct {
	Revision  uint64    `json:"revision"`
	Nodes     int       `json:"nodes"`
	Users     int       `json:"users"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// EventsHandler streams tailnet changes over server-sent events.
//
// SSE rather than WebSocket: the traffic is one-way, it survives ordinary HTTP
// proxies, and browsers reconnect on their own.
func EventsHandler(watcher *tailnet.Watcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.PrincipalFrom(r.Context()); !ok {
			http.Error(w, "not signed in", http.StatusUnauthorized)

			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)

			return
		}

		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		// Nginx buffers proxied responses by default, which holds every
		// event until the buffer fills — indistinguishable from a broken
		// stream.
		h.Set("X-Accel-Buffering", "no")

		w.WriteHeader(http.StatusOK)

		events, unsubscribe := watcher.Subscribe()
		defer unsubscribe()

		// Send the current state immediately so a browser that connects
		// between changes is not left with an empty view until something
		// happens.
		if snap, err := watcher.Current(); err == nil && snap != nil {
			writeEvent(w, flusher, *snap)
		}

		ticker := time.NewTicker(keepalive)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return

			case snap, open := <-events:
				if !open {
					return
				}

				writeEvent(w, flusher, snap)

			case <-ticker.C:
				fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}
}

func writeEvent(w http.ResponseWriter, flusher http.Flusher, snap tailnet.Snapshot) {
	body, err := json.Marshal(eventPayload{
		Revision:  snap.Revision,
		Nodes:     len(snap.Nodes),
		Users:     len(snap.Users),
		FetchedAt: snap.FetchedAt,
	})
	if err != nil {
		return
	}

	// The id lets a reconnecting browser tell whether it missed anything.
	fmt.Fprintf(w, "id: %d\nevent: tailnet\ndata: %s\n\n", snap.Revision, body)
	flusher.Flush()
}
