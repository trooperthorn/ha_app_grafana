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
// (auth handshake, recorder/statistics_during_period, and the
// system_health/info subscription protocol) to exercise haClient.
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
		case "recorder/statistics_during_period":
			_ = conn.WriteJSON(map[string]any{
				"id": id, "type": "result", "success": true,
				"result": map[string]any{
					"sensor.outdoor_temperature": []map[string]any{
						{"start": 1700000000000.0, "mean": 21.5, "min": 20.0, "max": 23.0},
					},
				},
			})
		case "system_health/info":
			_ = conn.WriteJSON(map[string]any{"id": id, "type": "result", "success": true})
			_ = conn.WriteJSON(map[string]any{
				"id": id, "type": "event",
				"event": map[string]any{
					"type": "initial",
					"data": map[string]any{
						"homeassistant": map[string]any{"info": map[string]any{"version": "2026.9.0"}},
					},
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"id": id, "type": "event",
				"event": map[string]any{"type": "finish"},
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

func wsTestClient(t *testing.T, srv *httptest.Server) *haClient {
	t.Helper()
	return newHAClient(strings.Replace(srv.URL, "http://", "ws://", 1), "test-token", false)
}

func TestQueryDataStatistics(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: wsTestClient(t, srv)}
	qJSON, _ := json.Marshal(queryModel{Series: "statistics", StatisticIDs: "sensor.outdoor_temperature", Period: "hour"})

	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{
			RefID: "A", JSON: qJSON,
			TimeRange: backend.TimeRange{From: time.Now().Add(-time.Hour), To: time.Now()},
		}},
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

func TestQueryDataSystemHealth(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: wsTestClient(t, srv)}
	qJSON, _ := json.Marshal(queryModel{Series: "system_health"})

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

func TestAuthInvalidToken(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	client := newHAClient(strings.Replace(srv.URL, "http://", "ws://", 1), "wrong-token", false)
	if _, err := client.systemHealthInfo(context.Background()); err == nil {
		t.Fatal("expected an authentication error for a wrong token")
	}
}
