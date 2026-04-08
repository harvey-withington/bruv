package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const maxNotifications = 200

// Notification represents a single notification entry.
type Notification struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Source    string    `json:"source"`
	CardID    string    `json:"card_id,omitempty"`
	CardTitle string    `json:"card_title,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Read      bool      `json:"read"`
}

func notificationsPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "notifications.json"), nil
}

// LoadNotifications reads the notification history from disk.
func LoadNotifications() ([]Notification, error) {
	path, err := notificationsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Notification{}, nil
		}
		return nil, err
	}
	var list []Notification
	if err := json.Unmarshal(data, &list); err != nil {
		return []Notification{}, nil
	}
	return list, nil
}

func saveNotifications(list []Notification) error {
	path, err := notificationsPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AppendNotification adds a notification to the history (newest first), trimming to maxNotifications.
func AppendNotification(n Notification) error {
	list, err := LoadNotifications()
	if err != nil {
		list = []Notification{}
	}
	list = append([]Notification{n}, list...)
	if len(list) > maxNotifications {
		list = list[:maxNotifications]
	}
	return saveNotifications(list)
}

// MarkNotificationRead marks a single notification as read.
func MarkNotificationRead(id string) error {
	list, err := LoadNotifications()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == id {
			list[i].Read = true
			break
		}
	}
	return saveNotifications(list)
}

// MarkAllNotificationsRead marks all notifications as read.
func MarkAllNotificationsRead() error {
	list, err := LoadNotifications()
	if err != nil {
		return err
	}
	for i := range list {
		list[i].Read = true
	}
	return saveNotifications(list)
}

// ClearAllNotifications removes all notifications.
func ClearAllNotifications() error {
	return saveNotifications([]Notification{})
}
