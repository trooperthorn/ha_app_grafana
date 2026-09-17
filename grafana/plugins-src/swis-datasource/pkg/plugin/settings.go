package plugin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// Settings is what the data source configuration page stores. Everything a user types
// lands in Grafana's jsonData except the two values that must never leave the server
// unencrypted: the Orion password and the CA bundle. Those come from secureJsonData,
// which Grafana encrypts at rest and never returns to the browser.
type Settings struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	TLSSkipVerify bool   `json:"tlsSkipVerify"`
	// TLSIgnoreHostname keeps chain verification against the pasted certificate but skips
	// the name check. The stock SWIS certificate is issued to a fixed name with no subject
	// alternative names, so with it pinned the chain verifies and the name never does.
	TLSIgnoreHostname bool     `json:"tlsIgnoreHostname"`
	MaxRows           int      `json:"maxRows"`
	TimeoutSecs       int      `json:"timeoutSeconds"`
	InvokeAllow       []string `json:"invokeAllow"`

	Password string `json:"-"`
	CACert   string `json:"-"`
}

const (
	defaultPort    = 17774
	defaultMaxRows = 10000
	defaultTimeout = 60
)

// LoadSettings parses the instance settings and applies the platform defaults documented
// in OrionGuides: REST on 17774 from platform release 2023.1 onward, HTTPS only.
func LoadSettings(src backend.DataSourceInstanceSettings) (*Settings, error) {
	s := &Settings{}
	if len(src.JSONData) > 0 {
		if err := json.Unmarshal(src.JSONData, s); err != nil {
			return nil, fmt.Errorf("could not read data source settings: %w", err)
		}
	}
	s.Host = strings.TrimSpace(s.Host)
	s.Username = strings.TrimSpace(s.Username)
	if s.Port == 0 {
		s.Port = defaultPort
	}
	if s.MaxRows <= 0 {
		s.MaxRows = defaultMaxRows
	}
	if s.TimeoutSecs <= 0 {
		s.TimeoutSecs = defaultTimeout
	}
	cleaned := make([]string, 0, len(s.InvokeAllow))
	for _, v := range s.InvokeAllow {
		if v = strings.TrimSpace(v); v != "" {
			cleaned = append(cleaned, v)
		}
	}
	s.InvokeAllow = cleaned
	s.Password = src.DecryptedSecureJSONData["password"]
	s.CACert = src.DecryptedSecureJSONData["caCert"]

	if s.Host == "" {
		return nil, fmt.Errorf("the Orion server host is not set")
	}
	if s.Username == "" {
		return nil, fmt.Errorf("the SWIS username is not set")
	}
	if s.Password == "" {
		return nil, fmt.Errorf("the SWIS password is not set")
	}
	return s, nil
}

// InvokeAllowed reports whether Entity.Verb is on the allowlist. The comparison is exact
// and case sensitive because SWIS verb names are, and because an allowlist that matched
// loosely would be an allowlist that permitted more than the administrator wrote down.
func (s *Settings) InvokeAllowed(entity, verb string) bool {
	want := entity + "." + verb
	for _, v := range s.InvokeAllow {
		if v == want {
			return true
		}
	}
	return false
}
