package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dashboard/stats/get" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "ok",
			"response": {
				"stats": {"totalQueries": 42},
				"mainChartData": {
					"labels": ["00:00", "01:00"],
					"datasets": [{"label": "Total Queries", "data": [10, 32]}]
				},
				"topClients": [{"name": "192.168.1.5", "hits": 20}],
				"topDomains": [{"name": "example.com", "hits": 15}],
				"topBlockedDomains": [{"name": "ads.example", "hits": 5}]
			}
		}`))
	}))
}

func TestQueryData(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: newTechnitiumClient(srv.URL, "test-token")}

	qJSON, err := json.Marshal(queryModel{StatType: "LastDay", Series: "volume"})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := ds.QueryData(
		context.Background(),
		&backend.QueryDataRequest{
			Queries: []backend.DataQuery{
				{RefID: "A", JSON: qJSON},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Responses) != 1 {
		t.Fatal("QueryData must return a response")
	}
	if err := resp.Responses["A"].Error; err != nil {
		t.Fatalf("query A failed: %v", err)
	}
	if len(resp.Responses["A"].Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(resp.Responses["A"].Frames))
	}
}

func TestQueryDataTopClients(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: newTechnitiumClient(srv.URL, "test-token")}

	qJSON, _ := json.Marshal(queryModel{StatType: "LastDay", Series: "topClients"})

	resp, err := ds.QueryData(
		context.Background(),
		&backend.QueryDataRequest{Queries: []backend.DataQuery{{RefID: "A", JSON: qJSON}}},
	)
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
