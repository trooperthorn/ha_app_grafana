package plugin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// haClient talks to a Home Assistant instance's own core WebSocket API
// (/api/websocket), authenticated with a long-lived access token, the same
// handshake every HA frontend and integration uses:
// https://developers.home-assistant.io/docs/api/websocket. There is no
// REST equivalent for long-term statistics (recorder/statistics_during_period)
// or system health (system_health/info); both are WebSocket-only commands,
// which is why this client speaks WebSocket rather than plain HTTP like
// this repository's other bundled plugins.
//
// One connection per call: a Grafana backend plugin answers one request per
// query, so there is no benefit to holding a persistent connection open
// between them, and doing so would need its own reconnect/keepalive logic
// this plugin has no reason to carry.
type haClient struct {
	origin     string
	token      string
	verifySSL  bool
	httpClient *http.Client
	nextID     atomic.Int64
}

func newHAClient(rawURL, token string, verifySSL bool) *haClient {
	origin := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if !strings.Contains(origin, "://") {
		origin = "http://" + origin
	}
	return &haClient{
		origin:     origin,
		token:      token,
		verifySSL:  verifySSL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *haClient) wsURL() (string, error) {
	u, err := url.Parse(c.origin)
	if err != nil {
		return "", fmt.Errorf("parsing url: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// Already a WebSocket scheme (only reachable in tests, which dial a
		// plain httptest server directly).
	default:
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	u.Path = "/api/websocket"
	return u.String(), nil
}

// resultMessage is the generic {"id","type":"result","success","result"} or
// {"id","type":"result","success":false,"error":{code,message}} envelope
// every command answers with, except system_health/info's subscription
// (see systemHealthInfo).
type resultMessage struct {
	ID      int64           `json:"id"`
	Type    string          `json:"type"`
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// dial opens a connection and completes the auth_required/auth/auth_ok
// handshake. The caller owns closing it.
func (c *haClient) dial(ctx context.Context) (*websocket.Conn, error) {
	wsURL, err := c.wsURL()
	if err != nil {
		return nil, err
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: !c.verifySSL}, //nolint:gosec // operator opt-in, off by default, matching this repo's Unifi plugins.
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to home assistant: %w", err)
	}

	var authRequired struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&authRequired); err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading auth_required: %w", err)
	}
	if authRequired.Type != "auth_required" {
		conn.Close()
		return nil, fmt.Errorf("expected auth_required, got %q", authRequired.Type)
	}

	if err := conn.WriteJSON(map[string]string{"type": "auth", "access_token": c.token}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sending auth: %w", err)
	}

	var authResult struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := conn.ReadJSON(&authResult); err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading auth result: %w", err)
	}
	if authResult.Type != "auth_ok" {
		conn.Close()
		msg := authResult.Message
		if msg == "" {
			msg = authResult.Type
		}
		return nil, fmt.Errorf("authentication failed: %s", msg)
	}

	return conn, nil
}

// call sends one command and returns its result, for every command that
// answers with the plain result envelope (i.e. everything except
// system_health/info).
func (c *haClient) call(ctx context.Context, cmdType string, params map[string]any) (json.RawMessage, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	id := c.nextID.Add(1)
	msg := map[string]any{"id": id, "type": cmdType}
	for k, v := range params {
		msg[k] = v
	}
	if err := conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("sending %s: %w", cmdType, err)
	}

	for {
		var resp resultMessage
		if err := conn.ReadJSON(&resp); err != nil {
			return nil, fmt.Errorf("reading %s response: %w", cmdType, err)
		}
		if resp.ID != id {
			continue
		}
		if resp.Type != "result" {
			continue
		}
		if !resp.Success {
			msg := "unknown error"
			if resp.Error != nil {
				msg = fmt.Sprintf("%s: %s", resp.Error.Code, resp.Error.Message)
			}
			return nil, fmt.Errorf("home assistant rejected %s: %s", cmdType, msg)
		}
		return resp.Result, nil
	}
}

// systemHealthInfo runs the system_health/info subscription to completion
// on its own connection and returns the merged {domain: {key: value}} data.
// Unlike every other command here, this one does not answer with a plain
// result: HA confirms the subscription with a bare {"success":true} result
// (no "result" key), then streams {"type":"event","event":{"type":"initial"|
// "update"|"finish", ...}} frames on the same connection until "finish".
// See homeassistant/components/system_health/__init__.py's handle_info.
func (c *haClient) systemHealthInfo(ctx context.Context) (map[string]map[string]any, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	id := c.nextID.Add(1)
	if err := conn.WriteJSON(map[string]any{"id": id, "type": "system_health/info"}); err != nil {
		return nil, fmt.Errorf("sending system_health/info: %w", err)
	}

	data := map[string]map[string]any{}

	for {
		var raw struct {
			ID      int64  `json:"id"`
			Type    string `json:"type"`
			Success bool   `json:"success"`
			Error   *struct {
				Message string `json:"message"`
			} `json:"error"`
			Event json.RawMessage `json:"event"`
		}
		if err := conn.ReadJSON(&raw); err != nil {
			return nil, fmt.Errorf("reading system_health/info response: %w", err)
		}
		if raw.ID != id {
			continue
		}
		switch raw.Type {
		case "result":
			if !raw.Success {
				msg := "unknown error"
				if raw.Error != nil {
					msg = raw.Error.Message
				}
				return nil, fmt.Errorf("home assistant rejected system_health/info: %s", msg)
			}
			// Subscription confirmed; the actual data arrives as events.
		case "event":
			var eventType struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw.Event, &eventType); err != nil {
				return nil, fmt.Errorf("decoding system_health/info event: %w", err)
			}
			switch eventType.Type {
			case "initial":
				var initial struct {
					Data map[string]domainData `json:"data"`
				}
				if err := json.Unmarshal(raw.Event, &initial); err != nil {
					return nil, fmt.Errorf("decoding system_health/info initial event: %w", err)
				}
				for domain, dd := range initial.Data {
					info := map[string]any{}
					for k, v := range dd.Info {
						info[k] = v
					}
					data[domain] = info
				}
			case "update":
				var update struct {
					Domain string `json:"domain"`
					Key    string `json:"key"`
					Data   any    `json:"data"`
				}
				if err := json.Unmarshal(raw.Event, &update); err != nil {
					return nil, fmt.Errorf("decoding system_health/info update event: %w", err)
				}
				if data[update.Domain] == nil {
					data[update.Domain] = map[string]any{}
				}
				data[update.Domain][update.Key] = update.Data
			case "finish":
				return data, nil
			}
		}
	}
}

// domainData is system_health/info's per-domain "initial" payload: an
// "info" map of health-fact key to value (a value still pending resolution
// arrives as {"type":"pending"} and is overwritten by a later "update"
// event for the same domain/key).
type domainData struct {
	Info map[string]any `json:"info"`
}
