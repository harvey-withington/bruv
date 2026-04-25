package http

import (
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"time"

	"bruv/core/events"
)

// sseHandler streams events from the bus to the client using
// Server-Sent Events. EventSource in the browser handles reconnection;
// the Go desktop client will need its own streaming reader with
// backoff + ?since= resume (phase 4 work).
//
// SSE frame format per spec:
//
//	event: <topic>
//	id: <event-id>
//	data: <json-payload>
//	\n
//
// Heartbeat: a comment line every 15s keeps proxies from closing idle
// connections and lets the client detect transport failure.
func sseHandler(bus *events.MemBus) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		flusher, ok := w.(nethttp.Flusher)
		if !ok {
			nethttp.Error(w, "streaming unsupported", nethttp.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

		ch, unsub := bus.Subscribe()
		defer unsub()

		// Initial comment flushes response headers to the client so
		// EventSource's onopen fires before the first real event.
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case ev, ok := <-ch:
				if !ok {
					return
				}
				payload, err := json.Marshal(ev.Payload)
				if err != nil {
					// Skip un-encodable event rather than kill the stream.
					continue
				}
				if _, err := fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n",
					ev.Topic, ev.ID, payload); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
