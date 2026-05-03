package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Notification is the JSON payload delivered to the service worker's
// `push` event. Stays small to fit comfortably under the Web Push
// service body-size limits (~4KB across all push services).
type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	URL   string `json:"url,omitempty"`  // path to navigate on tap (eg /m/c/<id>)
	Icon  string `json:"icon,omitempty"` // optional icon URL; SW falls back to manifest icon
	Tag   string `json:"tag,omitempty"`  // collapse key — same tag replaces an earlier notification
}

// Sender wraps webpush-go with our keyring and registry. Use New to
// construct; Send is goroutine-safe.
type Sender struct {
	vapid    *VAPID
	registry *Registry
	httpc    *nethttp.Client
}

// NewSender constructs a Sender. The default HTTP client has a 10s
// timeout — push services typically respond quickly; longer waits
// make agent runs feel hung when push is misconfigured.
func NewSender(vapid *VAPID, registry *Registry) *Sender {
	return &Sender{
		vapid:    vapid,
		registry: registry,
		httpc:    &nethttp.Client{Timeout: 10 * time.Second},
	}
}

// SendToDevice fires a single notification at one device's
// subscription. Returns nil on success and on benign failures
// ("subscription gone, cleaned up"). Real errors come back wrapped.
//
// Cleanup behaviour: 404 / 410 from the push service indicates the
// subscription is permanently gone (browser uninstalled, user revoked).
// We drop it from the registry and return nil — there's nothing the
// caller can do.
func (s *Sender) SendToDevice(ctx context.Context, deviceID string, n Notification) error {
	sub, ok := s.registry.Get(deviceID)
	if !ok {
		// No subscription is a benign condition for the caller —
		// not every device opts into push. Don't escalate to error.
		return nil
	}
	return s.sendOne(ctx, sub, n)
}

// SendToAll fires the same notification at every registered
// subscription. Best-effort — failures are logged per device and
// don't abort the loop. Returns the count of successful deliveries
// and the first non-cleanup error observed (for caller diagnostics).
func (s *Sender) SendToAll(ctx context.Context, n Notification) (sent int, firstErr error) {
	for _, sub := range s.registry.All() {
		if err := s.sendOne(ctx, sub, n); err != nil {
			slog.Warn("push: send to device failed",
				"device_id", sub.DeviceID, "endpoint_host", endpointHost(sub.Endpoint), "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		sent++
	}
	return sent, firstErr
}

func (s *Sender) sendOne(ctx context.Context, sub Subscription, n Notification) error {
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	wpsub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	resp, err := webpush.SendNotificationWithContext(ctx, payload, wpsub, &webpush.Options{
		VAPIDPublicKey:  s.vapid.Public(),
		VAPIDPrivateKey: s.vapid.Private(),
		Subscriber:      s.vapid.Subject(),
		// TTL: how long the push service should hold the notification
		// if the device is offline. 24h matches Chrome's default. Past
		// that, the user has either picked up their phone or won't
		// care about a stale ping.
		TTL: 24 * 60 * 60,
		// Urgency low/normal/high — affects whether the push wakes a
		// dozing device. "Normal" is right for routine agent updates;
		// "high" is for "this is going to fire if you don't act now"
		// kinds of alerts. Default to normal here; callers can extend
		// the API if a per-notification urgency knob is needed.
		Urgency: webpush.UrgencyNormal,
	})
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == 404 || resp.StatusCode == 410:
		// Subscription is permanently gone. Drop it from the registry
		// and return success — there's nothing useful for the caller
		// to do, and retrying would just produce the same response
		// every time.
		if remErr := s.registry.Remove(sub.DeviceID); remErr != nil {
			slog.Warn("push: cleanup failed", "device_id", sub.DeviceID, "err", remErr)
		} else {
			slog.Info("push: subscription expired, removed from registry",
				"device_id", sub.DeviceID, "status", resp.StatusCode)
		}
		return nil
	default:
		return fmt.Errorf("push service returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
}

// endpointHost extracts a host hint from an endpoint URL for log
// readability without leaking the full path (which can include a
// device-identifying token component on some push services). Best
// effort — falls back to "unknown" on parse failure.
func endpointHost(endpoint string) string {
	const proto = "://"
	i := bytes.Index([]byte(endpoint), []byte(proto))
	if i < 0 {
		return "unknown"
	}
	rest := endpoint[i+len(proto):]
	for j := 0; j < len(rest); j++ {
		if rest[j] == '/' {
			return rest[:j]
		}
	}
	return rest
}

// ErrNoSender is returned by callers that need a Sender but the
// server was started without push configured. Not used internally
// today; provided for downstream integration.
var ErrNoSender = errors.New("push: sender not configured")
