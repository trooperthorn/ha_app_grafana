package models

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// PluginSettings is the non-secret configuration entered on the datasource's
// config page: the Technitium DNS Server's own web console URL (the API
// shares that origin, e.g. http://192.168.1.10:5380).
type PluginSettings struct {
	URL string `json:"url"`
	// QueryLogsAppName is the installed name of the "Query Logs (Sqlite)"
	// DNS app, for servers that renamed it at install time. Empty uses the
	// store default, "Query Logs (Sqlite)".
	QueryLogsAppName string                `json:"queryLogsAppName"`
	Secrets          *SecretPluginSettings `json:"-"`
}

// SecretPluginSettings holds the API token, created once in the Technitium
// console (Administration > Sessions > Create API Token) so it never expires
// the way a login session does. It is never sent back to the browser.
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
