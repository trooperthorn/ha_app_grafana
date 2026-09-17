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
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy/protect/integration/v1/cameras", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"cam1","name":"Front Door","host":"192.168.1.30","mac":"AA:BB:CC:DD:EE:01",
			 "isRecording":true,"state":"CONNECTED","channels":[{"name":"High"},{"name":"Low"}]},
			{"id":"cam2","name":"Backyard","host":"192.168.1.31","mac":"AA:BB:CC:DD:EE:02",
			 "recordingSettings":{"mode":"never"},"isConnected":false,"channels":[{"name":"High"}]}
		]`))
	})
	return httptest.NewServer(mux)
}

func TestQueryDataCameras(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	client := newUnifiProtectClient(srv.URL, "test-key", true)
	ds := Datasource{client: client, origin: client.origin}
	qJSON, _ := json.Marshal(queryModel{Series: "cameras"})

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

	var recording []bool
	var channelCount []int64
	for _, f := range frame.Fields {
		switch f.Name {
		case "is_recording":
			for i := 0; i < f.Len(); i++ {
				v, _ := f.At(i).(bool)
				recording = append(recording, v)
			}
		case "channel_count":
			for i := 0; i < f.Len(); i++ {
				v, _ := f.At(i).(int64)
				channelCount = append(channelCount, v)
			}
		}
	}
	if len(recording) != 2 || !recording[0] || recording[1] {
		t.Fatalf("expected [true, false] recording (bool vs. recordingSettings.mode), got %v", recording)
	}
	if len(channelCount) != 2 || channelCount[0] != 2 || channelCount[1] != 1 {
		t.Fatalf("expected channel counts [2, 1], got %v", channelCount)
	}
}

func TestQueryDataUnknownSeries(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	client := newUnifiProtectClient(srv.URL, "test-key", true)
	ds := Datasource{client: client, origin: client.origin}
	qJSON, _ := json.Marshal(queryModel{Series: "events"})

	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: qJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Responses["A"].Error == nil {
		t.Fatal("expected an error for an unknown series (Protect has no events REST route)")
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
		t.Fatalf("expected error status for missing host/key, got %v: %s", res.Status, res.Message)
	}
}
