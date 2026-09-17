package models

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// PluginSettings is the non-secret configuration entered on the datasource's
// config page: the Unifi Network controller/console host (e.g.
// "192.168.1.1" or "https://192.168.1.1"), and whether to verify its TLS
// certificate (off by default, matching a typical self-signed console, the
// same default ha_int_soc's UniFi client uses).
type PluginSettings struct {
	Host      string                `json:"host"`
	VerifySSL bool                  `json:"verifySSL"`
	Secrets   *SecretPluginSettings `json:"-"`
}

// SecretPluginSettings holds the read-only local Integration API key,
// created in the controller's UI (Settings > Control Plane > Integrations).
// Never sent back to the browser.
type SecretPluginSettings struct {
	APIKey string `json:"apiKey"`
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
		APIKey: source["apiKey"],
	}
}
