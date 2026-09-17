package plugin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	networkAPIPath = "/proxy/network/integration/v1"
	pageLimit      = 200
	maxPages       = 15
	maxBodyBytes   = 8 * 1024 * 1024
)

// row is one client/device/site record as the controller returns it: field
// names vary across firmwares and API generations (see
// docs/UNIFI-LOCAL-API-CONTRACT.md in trooperthorn/ha_int_soc, which this
// client's field candidates are taken from), so this plugin reads them as
// loosely-typed maps and resolves a value from a candidate-key list rather
// than trusting one name.
type row map[string]any

// unifiNetworkClient talks to a Unifi Network controller's local
// Integration API (X-API-KEY over HTTPS, normally self-signed) the same way
// trooperthorn/ha_int_soc's unifi.py does: no redirects followed (so the
// key can never be forwarded to a redirect target), a bounded response
// size, and offset/limit pagination that stops on a short page.
type unifiNetworkClient struct {
	origin     string
	apiKey     string
	httpClient *http.Client
}

func newUnifiNetworkClient(host, apiKey string, verifySSL bool) *unifiNetworkClient {
	origin := strings.TrimRight(strings.TrimSpace(host), "/")
	if !strings.Contains(origin, "://") {
		origin = "https://" + origin
	}

	transport := &http.Transport{}
	if !verifySSL {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // operator opt-in for a typically self-signed console, matching ha_int_soc's default.
	}

	return &unifiNetworkClient{
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

func (c *unifiNetworkClient) get(ctx context.Context, path string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+networkAPIPath+path, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling unifi network: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, fmt.Errorf("the controller returned an unexpected redirect; refusing to follow it")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed - check the API key")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("endpoint not found (%s)", path)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unifi network returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBodyBytes {
		return nil, fmt.Errorf("the controller response is too large to process")
	}

	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
			if len(body) > maxBodyBytes {
				return nil, fmt.Errorf("the controller response is too large to process")
			}
		}
		if readErr != nil {
			break
		}
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("the controller returned an unexpected (non-JSON) response: %w", err)
	}
	return payload, nil
}

// rows pulls the record list out of a response, tolerating a bare list or
// the Integration API's {"data": [...]} paginated envelope.
func rows(payload any) []row {
	switch v := payload.(type) {
	case []any:
		out := make([]row, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				out = append(out, row(m))
			}
		}
		return out
	case map[string]any:
		if data, ok := v["data"].([]any); ok {
			out := make([]row, 0, len(data))
			for _, item := range data {
				if m, ok := item.(map[string]any); ok {
					out = append(out, row(m))
				}
			}
			return out
		}
	}
	return nil
}

func (c *unifiNetworkClient) getPaginated(ctx context.Context, path string) ([]row, error) {
	var out []row
	offset := 0
	for i := 0; i < maxPages; i++ {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		payload, err := c.get(ctx, fmt.Sprintf("%s%soffset=%d&limit=%d", path, sep, offset, pageLimit))
		if err != nil {
			return nil, err
		}
		page := rows(payload)
		out = append(out, page...)
		_, isMap := payload.(map[string]any)
		if !isMap || len(page) < pageLimit {
			break
		}
		offset += pageLimit
	}
	return out, nil
}

// resolveSiteID returns the first site's id; most home installs have
// exactly one ("default").
func (c *unifiNetworkClient) resolveSiteID(ctx context.Context) (string, error) {
	payload, err := c.get(ctx, "/sites")
	if err != nil {
		return "", err
	}
	sites := rows(payload)
	if len(sites) == 0 {
		return "", fmt.Errorf("the controller reported no sites for this API key")
	}
	id := first(sites[0], "id", "_id", "siteId", "name")
	if id == "" {
		return "", fmt.Errorf("could not determine the unifi site id")
	}
	return id, nil
}

func (c *unifiNetworkClient) getClients(ctx context.Context, siteID string) ([]row, error) {
	return c.getPaginated(ctx, "/sites/"+url.PathEscape(siteID)+"/clients")
}

func (c *unifiNetworkClient) getDevices(ctx context.Context, siteID string) ([]row, error) {
	return c.getPaginated(ctx, "/sites/"+url.PathEscape(siteID)+"/devices")
}

// first returns the first present, non-empty string value among candidate
// keys, formatting a non-string scalar with fmt.Sprint (the controller
// sends both string and numeric ids depending on field and firmware).
func first(r row, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != nil {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			case bool:
				// A bool candidate (e.g. "enabled") is never a valid string
				// field value here; skip it rather than printing "true".
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
