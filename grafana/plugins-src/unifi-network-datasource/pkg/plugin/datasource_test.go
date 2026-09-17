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
	mux.HandleFunc("/proxy/network/integration/v1/sites", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"default"}]}`))
	})
	mux.HandleFunc("/proxy/network/integration/v1/sites/default/clients", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"offset":0,"limit":200,"count":1,"totalCount":1,"data":[
			{"id":"c1","name":"laptop","macAddress":"AA:BB:CC:DD:EE:FF","ipAddress":"192.168.1.50",
			 "vlan":10,"ssid":"home","type":"WIRELESS","uptime":3600,"connectedAt":1000000000}
		]}`))
	})
	mux.HandleFunc("/proxy/network/integration/v1/sites/default/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"offset":0,"limit":200,"count":1,"totalCount":1,"data":[
			{"id":"d1","name":"gateway","macAddress":"11:22:33:44:55:66","ipAddress":"192.168.1.1",
			 "model":"UDM-Pro","state":"CONNECTED","type":"gateway",
			 "uplink":{"name":"eth8","up":true,"ip":"203.0.113.5","rxRateBps":1000,"txRateBps":2000}}
		]}`))
	})
	return httptest.NewServer(mux)
}

func TestQueryDataClients(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: newUnifiNetworkClient(srv.URL, "test-key", true)}
	qJSON, _ := json.Marshal(queryModel{Series: "clients"})

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

func TestQueryDataDevices(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: newUnifiNetworkClient(srv.URL, "test-key", true)}
	qJSON, _ := json.Marshal(queryModel{Series: "devices"})

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

func TestQueryDataWAN(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	ds := Datasource{client: newUnifiNetworkClient(srv.URL, "test-key", true)}
	qJSON, _ := json.Marshal(queryModel{Series: "wan"})

	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: qJSON}},
	})
	if err != nil {
		t.Fatal(err)
	}
	frame := resp.Responses["A"].Frames[0]
	var ip string
	for _, f := range frame.Fields {
		if f.Name == "ip" {
			ip, _ = f.At(0).(string)
		}
	}
	if ip != "203.0.113.5" {
		t.Fatalf("expected derived WAN ip 203.0.113.5, got %q", ip)
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
