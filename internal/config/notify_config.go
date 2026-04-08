package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// NotifyConfig holds global notification channel settings.
type NotifyConfig struct {
	SystemEnabled     bool   `json:"system_enabled"`
	SMTPHost          string `json:"smtp_host,omitempty"`
	SMTPPort          int    `json:"smtp_port,omitempty"`
	SMTPUsername      string `json:"smtp_username,omitempty"`
	SMTPPassword      string `json:"smtp_password,omitempty"`
	SMTPFromAddr      string `json:"smtp_from_addr,omitempty"`
	SMTPToAddr        string `json:"smtp_to_addr,omitempty"`
	SMTPTLS           bool   `json:"smtp_tls"`
	WebhookURL        string `json:"webhook_url,omitempty"`
	WebhookAuthHeader string `json:"webhook_auth_header,omitempty"`
}

func notifyConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "notify_config.json"), nil
}

// DefaultNotifyConfig returns sensible defaults for new installs.
func DefaultNotifyConfig() NotifyConfig {
	return NotifyConfig{
		SystemEnabled: true,
		SMTPPort:      587,
		SMTPTLS:       true,
	}
}

// LoadNotifyConfig reads the notification config from disk, returning defaults if not found.
func LoadNotifyConfig() (NotifyConfig, error) {
	path, err := notifyConfigPath()
	if err != nil {
		return DefaultNotifyConfig(), err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultNotifyConfig(), nil
		}
		return DefaultNotifyConfig(), err
	}
	var c NotifyConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return DefaultNotifyConfig(), err
	}
	return c, nil
}

// SaveNotifyConfig writes the notification config to disk.
func SaveNotifyConfig(c NotifyConfig) error {
	path, err := notifyConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
