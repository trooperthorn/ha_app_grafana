package models

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// PluginSettings is the non-secret configuration entered on the datasource's
// config page: the Music Assistant server's own web console origin (e.g.
// http://192.168.1.20:8095).
type PluginSettings struct {
	URL     string                `json:"url"`
	Secrets *SecretPluginSettings `json:"-"`
}

// SecretPluginSettings holds a long-lived API token, created once from the
// Music Assistant web UI (or its "auth/token/create" API command) so it
// does not need renewing the way a login session token does. It is never
// sent back to the browser.
type SecretPluginSettings struct {
	APIToken string `json:"apiToken"`
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
		APIToken: source["apiToken"],
	}
}
