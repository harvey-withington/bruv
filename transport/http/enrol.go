package http

import (
	"encoding/json"
	nethttp "net/http"
)

// enrolRequest is the POST /auth/enrol body. Bootstrap token is
// required and is checked by requireBootstrap middleware before this
// handler runs; device name is optional and defaults to "Unnamed
// device" on the store side.
type enrolRequest struct {
	BootstrapToken string `json:"bootstrap_token"`
	DeviceName     string `json:"device_name"`
}

// enrolResponse returns the plaintext device token (shown exactly
// once — the server only stores the hash) plus the device record
// metadata so the client can cache it.
type enrolResponse struct {
	DeviceToken string `json:"device_token"`
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
}

// enrolHandler accepts a bootstrap token and returns a fresh device
// token. Callable only with an existing bootstrap bearer — the
// middleware gate is wired in server.go.
func enrolHandler(store *DeviceStore) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodPost {
			nethttp.Error(w, `{"error":"method must be POST"}`, nethttp.StatusMethodNotAllowed)
			return
		}

		var req enrolRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			nethttp.Error(w, `{"error":"invalid JSON"}`, nethttp.StatusBadRequest)
			return
		}

		// Bootstrap token is validated both by middleware and here —
		// the body is the authoritative source for which bootstrap to
		// use (future multi-tenant support), even if today it just
		// echoes the Authorization header.
		token, dev, err := store.Enrol(req.BootstrapToken, req.DeviceName)
		if err != nil {
			nethttp.Error(w, `{"error":"`+err.Error()+`"}`, nethttp.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(enrolResponse{
			DeviceToken: token,
			DeviceID:    dev.ID,
			DeviceName:  dev.Name,
		})
	}
}
