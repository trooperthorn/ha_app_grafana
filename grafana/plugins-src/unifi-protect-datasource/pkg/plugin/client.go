package plugin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	protectAPIPath = "/proxy/protect/integration/v1"
	maxBodyBytes   = 8 * 1024 * 1024
)

// row is one camera record as the console returns it: field names vary
// across firmwares (see docs/UNIFI-LOCAL-API-CONTRACT.md in
// trooperthorn/ha_int_soc, which this client's field candidates are taken
// from), so this plugin reads them as loosely-typed maps and resolves a
// value from a candidate-key list rather than trusting one name.
type row map[string]any

// unifiProtectClient talks to a Unifi Protect console's local Integration
// API (X-API-KEY over HTTPS, normally self-signed) the same way
// trooperthorn/ha_int_soc's unifi.py does: no redirects followed (so the
// key can never be forwarded to a redirect target) and a bounded response
// size. Protect's /cameras is an unpaginated array, unlike Network's
// offset/limit collections, so there is no pagination helper here.
type unifiProtectClient struct {
	origin     string
	apiKey     string
	httpClient *http.Client
}

func newUnifiProtectClient(host, apiKey string, verifySSL bool) *unifiProtectClient {
	origin := strings.TrimRight(strings.TrimSpace(host), "/")
	if !strings.Contains(origin, "://") {
		origin = "https://" + origin
	}

	transport := &http.Transport{}
	if !verifySSL {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // operator opt-in for a typically self-signed console, matching ha_int_soc's default.
	}

	return &unifiProtectClient{
		origin: origin,
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *unifiProtectClient) get(ctx context.Context, path string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+protectAPIPath+path, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling unifi protect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("the console returned an unexpected redirect; refusing to follow it")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed - check the API key")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("endpoint not found (%s)", path)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unifi protect returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBodyBytes {
		return nil, fmt.Errorf("the console response is too large to process")
	}

	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
			if len(body) > maxBodyBytes {
				return nil, fmt.Errorf("the console response is too large to process")
			}
		}
		if readErr != nil {
			break
		}
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("the console returned an unexpected (non-JSON) response: %w", err)
	}
	return payload, nil
}

// getCameras calls GET /cameras. Protect 7.2.105 documents this as an
// unpaginated JSON array (see docs/UNIFI-LOCAL-API-CONTRACT.md), unlike
// Network's offset/limit collections; appending pagination params here
// would be wrong, not just unnecessary.
func (c *unifiProtectClient) getCameras(ctx context.Context) ([]row, error) {
	payload, err := c.get(ctx, "/cameras")
	if err != nil {
		return nil, err
	}
	list, ok := payload.([]any)
	if !ok {
		return nil, fmt.Errorf("expected /cameras to return a JSON array")
	}
	out := make([]row, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, row(m))
		}
	}
	return out, nil
}

// first returns the first present, non-empty string value among candidate
// keys, formatting a non-string scalar with fmt.Sprint (the console sends
// both string and numeric ids depending on field and firmware).
func first(r row, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != nil {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			case bool:
				continue
			default:
				s := fmt.Sprint(t)
				if s != "" && s != "<nil>" {
					return s
				}
			}
		}
	}
	return ""
}

func firstBool(r row, keys ...string) (bool, bool) {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != nil {
			switch t := v.(type) {
			case bool:
				return t, true
			case float64:
				return t != 0, true
			}
		}
	}
	return false, false
}

func firstNumber(r row, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				return t, true
			case string:
				if f, err := strconv.ParseFloat(t, 64); err == nil {
					return f, true
				}
			}
		}
	}
	return 0, false
}

// asEpoch converts an epoch value in seconds or milliseconds (a value above
// 10,000,000,000 is treated as milliseconds, the same heuristic
// unifi.py's _as_epoch uses) into epoch seconds.
func asEpoch(v float64) int64 {
	i := int64(v)
	if i > 10_000_000_000 {
		return i / 1000
	}
	return i
}
