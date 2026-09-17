package plugin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// haSOCClient talks to a Home Assistant instance's own core WebSocket API
// (/api/websocket) to reach HA SOC's ha_soc/* commands (registered by
// trooperthorn/ha_int_soc's websocket_api.py on that same API - HA SOC has
// no HTTP view of its own, only WebSocket commands). Authenticated with a
// long-lived access token for an admin user, the same handshake every HA
// frontend and integration uses:
// https://developers.home-assistant.io/docs/api/websocket.
//
// One connection per call: a Grafana backend plugin answers one request
// per query, so there is no benefit to holding a persistent connection
// open between them.
type haSOCClient struct {
	origin    string
	token     string
	verifySSL bool
	nextID    atomic.Int64
}

func newHASOCClient(rawURL, token string, verifySSL bool) *haSOCClient {
	origin := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if !strings.Contains(origin, "://") {
		origin = "http://" + origin
	}
	return &haSOCClient{origin: origin, token: token, verifySSL: verifySSL}
}

func (c *haSOCClient) wsURL() (string, error) {
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
// every ha_soc/* command answers with.
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

func (c *haSOCClient) dial(ctx context.Context) (*websocket.Conn, error) {
	wsURL, err := c.wsURL()
	if err != nil {
		return nil, err
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: !c.verifySSL}, //nolint:gosec // operator opt-in, off by default.
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

// call sends one ha_soc/* command and returns its result.
func (c *haSOCClient) call(ctx context.Context, cmdType string, params map[string]any) (json.RawMessage, error) {
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
		if resp.ID != id || resp.Type != "result" {
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

// riskPosture calls ha_soc/risk/posture: the whole-install security score.
func (c *haSOCClient) riskPosture(ctx context.Context) (*posture, error) {
	raw, err := c.call(ctx, "ha_soc/risk/posture", nil)
	if err != nil {
		return nil, err
	}
	var p posture
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decoding risk posture: %w", err)
	}
	return &p, nil
}

// riskList calls ha_soc/risk/list: per-user risk scores.
func (c *haSOCClient) riskList(ctx context.Context) ([]userRisk, error) {
	raw, err := c.call(ctx, "ha_soc/risk/list", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Risk map[string]userRisk `json:"risk"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("decoding risk list: %w", err)
	}
	out := make([]userRisk, 0, len(wrapper.Risk))
	for _, r := range wrapper.Risk {
		out = append(out, r)
	}
	return out, nil
}

// auditQuery calls ha_soc/audit/query, bounded to [since, until] (both RFC
// 3339) and limit, matching the parameters HA SOC's own websocket_api.py
// accepts.
func (c *haSOCClient) auditQuery(ctx context.Context, since, until string, limit int) ([]auditEvent, error) {
	params := map[string]any{"limit": limit}
	if since != "" {
		params["since"] = since
	}
	if until != "" {
		params["until"] = until
	}
	raw, err := c.call(ctx, "ha_soc/audit/query", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Events []auditEvent `json:"events"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("decoding audit query: %w", err)
	}
	return wrapper.Events, nil
}
