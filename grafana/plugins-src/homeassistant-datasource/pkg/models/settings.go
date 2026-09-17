package models

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// PluginSettings is the non-secret configuration entered on the datasource's
// config page: the Home Assistant instance's own origin (e.g.
// "http://homeassistant.local:8123" or "https://192.168.1.5:8123").
type PluginSettings struct {
	URL       string                `json:"url"`
	VerifySSL bool                  `json:"verifySSL"`
	Secrets   *SecretPluginSettings `json:"-"`
}

// SecretPluginSettings holds a long-lived access token, created under the
// user's own profile (Security > Long-Lived Access Tokens). Never sent
// back to the browser.
type SecretPluginSettings struct {
	AccessToken string `json:"accessToken"`
}

func LoadPluginSettings(source backend.DataSourceInstanceSettings) (*PluginSettings, error) {
	settings := PluginSettings{}
	if len(source.JSONData) > 0 {
		if err := json.Unmarshal(source.JSONData, &settings); err != nil {
			return nil, fmt.Errorf("could not unmarshal PluginSettings json: %w", err)
		}
	}

	settings.Secrets = loadSecretPluginSettings(source.DecryptedSecureJSONData)

	return &settings, nil
}

func loadSecretPluginSettings(source map[string]string) *SecretPluginSettings {
	return &SecretPluginSettings{
		AccessToken: source["accessToken"],
	}
}
