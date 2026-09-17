package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// testServer fakes just enough of Home Assistant's core WebSocket API
// (auth handshake plus the ha_soc/* commands this plugin calls) to
// exercise haSOCClient.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		if err := conn.WriteJSON(map[string]string{"type": "auth_required"}); err != nil {
			return
		}
		var authMsg map[string]string
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}
		if authMsg["access_token"] != "test-token" {
			_ = conn.WriteJSON(map[string]string{"type": "auth_invalid", "message": "invalid token"})
			return
		}
		if err := conn.WriteJSON(map[string]string{"type": "auth_ok"}); err != nil {
			return
		}

		var cmd map[string]any
		if err := conn.ReadJSON(&cmd); err != nil {
			return
		}
		id := cmd["id"]

		switch cmd["type"] {
		case "ha_soc/risk/posture":
			_ = conn.WriteJSON(map[string]any{
				"id": id, "type": "result", "success": true,
				"result": map[string]any{
					"score": 87, "grade": "B", "provisional": false,
					"missing_terms": []string{},
					"breakdown": map[string]any{
						"p_user": 3.2, "p_vuln": 1.0, "p_misconfig": 0.5, "p_integration": 0.0, "p_detection": 2.0,
					},
				},
			})
		case "ha_soc/risk/list":
			_ = conn.WriteJSON(map[string]any{
				"id": id, "type": "result", "success": true,
				"result": map[string]any{
					"risk": map[string]any{
						"user1": map[string]any{
							"user_id": "user1", "score": 40, "band": "moderate",
							"factors": []map[string]any{{"name": "admin_without_mfa", "points": 40, "detail": "no mfa"}},
						},
					},
				},
			})
		case "ha_soc/audit/query":
			_ = conn.WriteJSON(map[string]any{
				"id": id, "type": "result", "success": true,
				"result": map[string]any{
					"events": []map[string]any{
						{"ts": "2026-09-17T00:00:00+00:00", "user_id": "user1", "category": "login_success",
							"domain": "auth", "service": "", "entity_ids": []string{}, "ip": "192.168.1.5", "seq": 1},
					},
				},
			})
		default:
			_ = conn.WriteJSON(map[string]any{
				"id": id, "type": "result", "success": false,
				"error": map[string]string{"code": "unknown_command", "message": "no handler"},
			})
		}
	}
	return httptest.NewServer(http.HandlerFunc(handler))
}

func wsTestClient(t *testing.T, srv *httptest.Server) *haSOCClient {
	t.Helper()
	return newHASOCClient(strings.Replace(srv.URL, "http://", "ws://", 1), "test-token", false)
}

func TestQueryDataPosture(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: wsTestClient(t, srv)}
	qJSON, _ := json.Marshal(queryModel{Series: "posture"})

	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: qJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Responses["A"].Error; err != nil {
		t.Fatalf("query A failed: %v", err)
	}
	frame := resp.Responses["A"].Frames[0]
	if frame.Rows() != 1 {
		t.Fatalf("expected 1 row, got %d", frame.Rows())
	}
}

func TestQueryDataRisk(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: wsTestClient(t, srv)}
	qJSON, _ := json.Marshal(queryModel{Series: "risk"})

	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: qJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame := resp.Responses["A"].Frames[0]
	if frame.Rows() != 1 {
		t.Fatalf("expected 1 row, got %d", frame.Rows())
	}
}

func TestQueryDataAudit(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: wsTestClient(t, srv)}
	qJSON, _ := json.Marshal(queryModel{Series: "audit", AuditLimit: 50})

	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{
			RefID: "A", JSON: qJSON,
			TimeRange: backend.TimeRange{From: time.Now().Add(-24 * time.Hour), To: time.Now()},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame := resp.Responses["A"].Frames[0]
	if frame.Rows() != 1 {
		t.Fatalf("expected 1 row, got %d", frame.Rows())
	}
}

func TestCheckHealthMissingSettings(t *testing.T) {
	ds := Datasource{}
	res, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{
		PluginContext: backend.PluginContext{
			DataSourceInstanceSettings: &backend.DataSourceInstanceSettings{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != backend.HealthStatusError {
		t.Fatalf("expected error status for missing url/token, got %v: %s", res.Status, res.Message)
	}
}
