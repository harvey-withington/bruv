// Package notify is the NotifyService — notification config and the
// in-app notification list.
//
// Extracted from app.go as part of the service-layer refactor. The tray
// tooltip refresh that follows mutations is intentionally kept in the
// App forwarder, not here: it's a host-platform concern (system tray)
// that doesn't belong inside the domain service.
package notify

import "bruv/internal/config"

// Service exposes notification config + notification-list operations.
// All methods delegate to internal/config which owns file persistence
// and keychain-backed secret handling.
type Service struct{}

// New constructs a NotifyService. The service is stateless — config
// package handles persistence.
func New() *Service { return &Service{} }

// GetConfig returns the current notification configuration (channels
// enabled, SMTP settings, webhook URLs).
func (s *Service) GetConfig() (config.NotifyConfig, error) {
	return config.LoadNotifyConfig()
}

// SetConfig persists the notification configuration.
func (s *Service) SetConfig(c config.NotifyConfig) error {
	return config.SaveNotifyConfig(c)
}

// List returns the in-app notification history.
func (s *Service) List() ([]config.Notification, error) {
	return config.LoadNotifications()
}

// MarkRead flags a single notification as read.
func (s *Service) MarkRead(id string) error {
	return config.MarkNotificationRead(id)
}

// MarkAllRead flags every notification as read.
func (s *Service) MarkAllRead() error {
	return config.MarkAllNotificationsRead()
}

// ClearAll deletes the in-app notification history.
func (s *Service) ClearAll() error {
	return config.ClearAllNotifications()
}
