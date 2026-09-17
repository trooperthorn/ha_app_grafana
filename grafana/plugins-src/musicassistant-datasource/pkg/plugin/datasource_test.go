package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api" {
			http.NotFound(w, r)
			return
		}
		var req commandMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Command != "players/all" {
			http.Error(w, "unknown command", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"message_id": %d,
			"result": [
				{
					"player_id": "living_room",
					"name": "Living Room",
					"available": true,
					"powered": true,
					"playback_state": "playing",
					"volume_level": 42,
					"elapsed_time": 12.5,
					"current_media": {"title": "Song", "artist": "Artist", "album": "Album", "duration": 200}
				},
				{
					"player_id": "kitchen",
					"name": "Kitchen",
					"available": true,
					"powered": false,
					"playback_state": "idle",
					"volume_level": 10,
					"elapsed_time": 0,
					"current_media": null
				}
			]
		}`, req.MessageID)
	}))
}

func TestQueryDataPlayers(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: newMusicAssistantClient(srv.URL, "test-token")}
	qJSON, _ := json.Marshal(queryModel{Series: "players"})

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
	if frame.Rows() != 2 {
		t.Fatalf("expected 2 rows, got %d", frame.Rows())
	}
}

func TestQueryDataNowPlaying(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: newMusicAssistantClient(srv.URL, "test-token")}
	qJSON, _ := json.Marshal(queryModel{Series: "nowPlaying"})

	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: qJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame := resp.Responses["A"].Frames[0]
	if frame.Rows() != 1 {
		t.Fatalf("expected 1 row (only the playing player), got %d", frame.Rows())
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
