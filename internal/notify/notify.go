package notify

import (
	"bruv/internal/config"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/google/uuid"
)

// Channel identifies a notification delivery method.
type Channel string

const (
	ChannelInApp   Channel = "in-app"
	ChannelSystem  Channel = "system"
	ChannelEmail   Channel = "email"
	ChannelWebhook Channel = "webhook"
)

// Request describes a notification to send.
type Request struct {
	Title     string
	Body      string
	Source    string // "agent", "system"
	CardID    string
	CardTitle string
	Channels  []Channel
}

// Dispatcher sends notifications to configured channels.
type Dispatcher struct {
	cfg       config.NotifyConfig
	emitEvent func(name string, data any)
}

// NewDispatcher creates a dispatcher with the given config and event emitter.
func NewDispatcher(cfg config.NotifyConfig, emitEvent func(string, any)) *Dispatcher {
	return &Dispatcher{cfg: cfg, emitEvent: emitEvent}
}

// Send dispatches a notification to all requested channels.
// Each channel runs in its own goroutine; errors are logged but don't block.
func (d *Dispatcher) Send(req Request) {
	n := config.Notification{
		ID:        uuid.New().String()[:8],
		Title:     req.Title,
		Body:      req.Body,
		Source:    req.Source,
		CardID:    req.CardID,
		CardTitle: req.CardTitle,
		CreatedAt: time.Now().UTC(),
		Read:      false,
	}

	for _, ch := range req.Channels {
		switch ch {
		case ChannelInApp:
			go d.sendInApp(n)
		case ChannelSystem:
			go d.sendSystem(n)
		case ChannelEmail:
			go d.sendEmail(n)
		case ChannelWebhook:
			go d.sendWebhook(req, n)
		}
	}
}

func (d *Dispatcher) sendInApp(n config.Notification) {
	if err := config.AppendNotification(n); err != nil {
		log.Printf("notify: in-app persist error: %v\n", err)
	}
	if d.emitEvent != nil {
		d.emitEvent("notification:new", n)
	}
}

func (d *Dispatcher) sendSystem(n config.Notification) {
	if !d.cfg.SystemEnabled {
		log.Printf("notify: system notifications disabled, skipping")
		return
	}
	log.Printf("notify: sending system notification: %q", n.Title)
	if err := beeep.Notify(n.Title, n.Body, ""); err != nil {
		log.Printf("notify: system notification error: %v", err)
	}
}

// TestSystemNotification sends a test OS notification synchronously and returns any error.
func TestSystemNotification() error {
	return beeep.Notify("BRUV", "Desktop notifications are working!", "")
}

func (d *Dispatcher) sendEmail(n config.Notification) {
	if d.cfg.SMTPHost == "" || d.cfg.SMTPToAddr == "" {
		return
	}

	from := d.cfg.SMTPFromAddr
	if from == "" {
		from = d.cfg.SMTPUsername
	}
	to := d.cfg.SMTPToAddr
	subject := n.Title

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		from, to, subject, n.Body)

	addr := fmt.Sprintf("%s:%d", d.cfg.SMTPHost, d.cfg.SMTPPort)
	auth := smtp.PlainAuth("", d.cfg.SMTPUsername, d.cfg.SMTPPassword, d.cfg.SMTPHost)

	var err error
	if d.cfg.SMTPTLS {
		err = d.sendEmailTLS(addr, auth, from, to, []byte(msg))
	} else {
		err = smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
	}
	if err != nil {
		log.Printf("notify: email error: %v\n", err)
	}
}

func (d *Dispatcher) sendEmailTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: d.cfg.SMTPHost}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, d.cfg.SMTPHost)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return client.Quit()
}

func (d *Dispatcher) sendWebhook(req Request, n config.Notification) {
	if d.cfg.WebhookURL == "" {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"title":      n.Title,
		"body":       n.Body,
		"source":     n.Source,
		"card_id":    n.CardID,
		"card_title": n.CardTitle,
		"timestamp":  n.CreatedAt.Format(time.RFC3339),
	})
	if err != nil {
		log.Printf("notify: webhook marshal error: %v\n", err)
		return
	}

	httpReq, err := http.NewRequest("POST", d.cfg.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("notify: webhook request error: %v\n", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "BRUV/1.0")
	if d.cfg.WebhookAuthHeader != "" {
		httpReq.Header.Set("Authorization", d.cfg.WebhookAuthHeader)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("notify: webhook error: %v\n", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("notify: webhook returned HTTP %d\n", resp.StatusCode)
	}
}

// ParseChannels converts a comma-separated channel string into a slice.
// In-app is always included — it cannot be disabled.
func ParseChannels(s string) []Channel {
	channels := []Channel{ChannelInApp}
	if s == "" {
		return channels
	}
	for _, part := range splitTrim(s) {
		ch := Channel(part)
		if ch == ChannelInApp {
			continue // already included
		}
		switch ch {
		case ChannelSystem, ChannelEmail, ChannelWebhook:
			channels = append(channels, ch)
		}
	}
	return channels
}

func splitTrim(s string) []string {
	var parts []string
	for _, p := range bytes.Split([]byte(s), []byte(",")) {
		t := string(bytes.TrimSpace(p))
		if t != "" {
			parts = append(parts, t)
		}
	}
	return parts
}
