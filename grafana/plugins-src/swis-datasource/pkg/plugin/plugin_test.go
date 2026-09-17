package plugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// stubSWIS answers the two REST calls the plugin makes, records what it received, and
// runs over TLS with a self-signed certificate so the CA-bundle path is exercised too.
type stubSWIS struct {
	srv        *httptest.Server
	lastQuery  string
	lastParams map[string]any
	lastInvoke string
	lastArgs   []any
	results    string
	status     int
	failBody   string
}

func newStub(t *testing.T) *stubSWIS {
	return newStubWithCert(t, nil)
}

// newStubWithCert starts the stub over TLS. With a nil certificate it uses httptest's,
// which carries subject alternative names for localhost; a certificate passed in is used
// as-is, which is how the stock SWIS certificate shape (a fixed common name, no SANs) is
// reproduced.
func newStubWithCert(t *testing.T, cert *tls.Certificate) *stubSWIS {
	t.Helper()
	s := &stubSWIS{results: "[]", status: http.StatusOK}
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "grafana" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"Message":"bad credentials"}`))
			return
		}
		if !strings.HasPrefix(r.URL.Path, basePath+"/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, basePath+"/")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case rest == "Query" && r.Method == http.MethodPost:
			var body struct {
				Query      string         `json:"query"`
				Parameters map[string]any `json:"parameters"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.lastQuery, s.lastParams = body.Query, body.Parameters
			if s.status != http.StatusOK {
				w.WriteHeader(s.status)
				_, _ = w.Write([]byte(s.failBody))
				return
			}
			_, _ = w.Write([]byte(`{"results":` + s.results + `}`))
		case strings.HasPrefix(rest, "Invoke/") && r.Method == http.MethodPost:
			s.lastInvoke = strings.TrimPrefix(rest, "Invoke/")
			_ = json.NewDecoder(r.Body).Decode(&s.lastArgs)
			_, _ = w.Write([]byte(`true`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	if cert != nil {
		s.srv.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	}
	s.srv.StartTLS()
	t.Cleanup(s.srv.Close)
	return s
}

// selfSignedNoSAN makes a certificate the way SWIS ships one: self-signed, a fixed
// common name, and no subject alternative names at all.
func selfSignedNoSAN(t *testing.T, cn string) (*tls.Certificate, []byte) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func (s *stubSWIS) settings(t *testing.T, extra map[string]any) backend.DataSourceInstanceSettings {
	t.Helper()
	u, _ := url.Parse(s.srv.URL)
	port, _ := strconv.Atoi(u.Port())
	jsonData := map[string]any{"host": u.Hostname(), "port": port, "username": "grafana"}
	for k, v := range extra {
		jsonData[k] = v
	}
	raw, _ := json.Marshal(jsonData)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.srv.Certificate().Raw})
	return backend.DataSourceInstanceSettings{
		JSONData:                raw,
		DecryptedSecureJSONData: map[string]string{"password": "secret", "caCert": string(caPEM)},
	}
}

func newDS(t *testing.T, s *stubSWIS, extra map[string]any) *Datasource {
	t.Helper()
	inst, err := NewDatasource(context.Background(), s.settings(t, extra))
	if err != nil {
		t.Fatalf("NewDatasource: %v", err)
	}
	return inst.(*Datasource)
}

func runQuery(t *testing.T, ds *Datasource, model QueryModel) backend.DataResponse {
	t.Helper()
	raw, _ := json.Marshal(model)
	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{
			RefID: "A",
			JSON:  raw,
			TimeRange: backend.TimeRange{
				From: time.Date(2026, 9, 15, 10, 0, 0, 0, time.FixedZone("plus2", 2*3600)),
				To:   time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC),
			},
		}},
	})
	if err != nil {
		t.Fatalf("QueryData: %v", err)
	}
	return resp.Responses["A"]
}

func TestSettingsDefaultsAndValidation(t *testing.T) {
	_, err := LoadSettings(backend.DataSourceInstanceSettings{JSONData: []byte(`{"host":"orion"}`)})
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("expected a missing-username error, got %v", err)
	}
	s, err := LoadSettings(backend.DataSourceInstanceSettings{
		JSONData:                []byte(`{"host":" orion.example.com ","username":"svc","invokeAllow":[" Orion.Nodes.PollNow ",""]}`),
		DecryptedSecureJSONData: map[string]string{"password": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Port != 17774 || s.MaxRows != 10000 || s.TimeoutSecs != 60 || s.Host != "orion.example.com" {
		t.Fatalf("defaults not applied: %+v", s)
	}
	if !s.InvokeAllowed("Orion.Nodes", "PollNow") || s.InvokeAllowed("Orion.Nodes", "Unmanage") || s.InvokeAllowed("orion.nodes", "pollnow") {
		t.Fatalf("allowlist matching is wrong: %+v", s.InvokeAllow)
	}
}

func TestExpandMacros(t *testing.T) {
	out, used, err := ExpandMacros("SELECT c.DateTime FROM Orion.CPULoad c WHERE $__timeFilter(c.DateTime) AND c.NodeID = 1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "c.DateTime >= @__timeFrom AND c.DateTime <= @__timeTo") || !used[paramTimeFrom] || !used[paramTimeTo] {
		t.Fatalf("timeFilter not expanded: %s %v", out, used)
	}
	out, used, err = ExpandMacros("SELECT x FROM y WHERE a > $__timeFrom()")
	if err != nil || out != "SELECT x FROM y WHERE a > @__timeFrom" || used[paramTimeTo] {
		t.Fatalf("timeFrom: %q %v %v", out, used, err)
	}
	if _, _, err := ExpandMacros("SELECT x FROM y WHERE $__timeFilter(c.DateTime; DROP)"); err == nil {
		t.Fatal("a malformed macro argument must be refused, not pasted into the statement")
	}
	if _, _, err := ExpandMacros("SELECT x FROM y WHERE $__interval > 1"); err == nil {
		t.Fatal("an unknown macro must be an error")
	}
}

func TestQueryBindsTimeRangeAndTypesColumns(t *testing.T) {
	stub := newStub(t)
	stub.results = `[
	  {"Caption":"core-sw-01","Status":1,"CPULoad":12.5,"UnManaged":false,"LastSync":"2026-09-16T09:58:12.1234567","Detail":null,"Nested":{"a":1}},
	  {"Caption":"edge-rtr-02","Status":2,"CPULoad":null,"UnManaged":true,"LastSync":"2026-09-16T09:57:00","Detail":"x","Nested":[1,2]}
	]`
	ds := newDS(t, stub, nil)
	resp := runQuery(t, ds, QueryModel{SWQL: "SELECT n.Caption FROM Orion.Nodes n WHERE $__timeFilter(n.LastSync) -- $__timeFrom() in a comment is ignored"})
	if resp.Error != nil {
		t.Fatalf("query failed: %v", resp.Error)
	}
	if stub.lastParams["__timeFrom"] != "2026-09-15T08:00:00Z" || stub.lastParams["__timeTo"] != "2026-09-16T10:00:00Z" {
		t.Fatalf("time range not bound as UTC ISO: %v", stub.lastParams)
	}
	if strings.Contains(stub.lastQuery, "$__") || strings.Contains(stub.lastQuery, "--") {
		t.Fatalf("macros or comments reached SWIS: %s", stub.lastQuery)
	}
	frame := resp.Frames[0]
	want := map[string]data.FieldType{
		"Caption": data.FieldTypeNullableString, "Status": data.FieldTypeNullableFloat64, "CPULoad": data.FieldTypeNullableFloat64,
		"UnManaged": data.FieldTypeNullableBool, "LastSync": data.FieldTypeNullableTime, "Detail": data.FieldTypeNullableString,
		"Nested": data.FieldTypeNullableString,
	}
	if len(frame.Fields) != len(want) {
		t.Fatalf("expected %d fields, got %d", len(want), len(frame.Fields))
	}
	for i, name := range []string{"Caption", "Status", "CPULoad", "UnManaged", "LastSync", "Detail", "Nested"} {
		if frame.Fields[i].Name != name {
			t.Fatalf("column order lost: field %d is %s, want %s", i, frame.Fields[i].Name, name)
		}
		if frame.Fields[i].Type() != want[name] {
			t.Fatalf("%s: type %v, want %v", name, frame.Fields[i].Type(), want[name])
		}
	}
	ts := frame.Fields[4].At(0).(*time.Time)
	if ts.Location() != time.UTC || ts.Hour() != 9 || ts.Minute() != 58 || ts.Nanosecond() != 123456700 {
		t.Fatalf("SWIS bare timestamp must be read as UTC with its fraction: %v", ts)
	}
	if frame.Fields[2].At(1).(*float64) != nil {
		t.Fatal("null number should stay nil")
	}
	if *(frame.Fields[6].At(0).(*string)) != `{"a":1}` {
		t.Fatalf("nested JSON should be rendered as text, got %v", frame.Fields[6].At(0))
	}
	if frame.Meta.ExecutedQueryString == "" {
		t.Fatal("the expanded statement should be reported in frame meta for the query inspector")
	}
}

func TestMaxRowsTruncatesWithNotice(t *testing.T) {
	stub := newStub(t)
	stub.results = `[{"N":1},{"N":2},{"N":3}]`
	ds := newDS(t, stub, map[string]any{"maxRows": 2})
	resp := runQuery(t, ds, QueryModel{SWQL: "SELECT n.NodeID AS N FROM Orion.Nodes n"})
	if resp.Error != nil || resp.Frames[0].Rows() != 2 || len(resp.Frames[0].Meta.Notices) != 1 {
		t.Fatalf("expected 2 rows and a notice: err=%v rows=%d", resp.Error, resp.Frames[0].Rows())
	}
}

func TestTimeSeriesPivotsLongFormat(t *testing.T) {
	stub := newStub(t)
	stub.results = `[
	  {"DateTime":"2026-09-16T09:05:00","Caption":"a","AvgLoad":3},
	  {"DateTime":"2026-09-16T09:00:00","Caption":"a","AvgLoad":1},
	  {"DateTime":"2026-09-16T09:05:00","Caption":"b","AvgLoad":4},
	  {"DateTime":"2026-09-16T09:00:00","Caption":"b","AvgLoad":2}
	]`
	ds := newDS(t, stub, nil)
	resp := runQuery(t, ds, QueryModel{SWQL: "SELECT c.DateTime FROM Orion.CPULoad c", Format: "timeseries"})
	if resp.Error != nil {
		t.Fatal(resp.Error)
	}
	frame := resp.Frames[0]
	if frame.Rows() != 2 || len(frame.Fields) != 3 {
		t.Fatalf("expected 2 timestamps and time + 2 series, got rows=%d fields=%d", frame.Rows(), len(frame.Fields))
	}
	if frame.Fields[1].Labels["Caption"] != "a" || frame.Fields[2].Labels["Caption"] != "b" {
		t.Fatalf("series labels missing: %v %v", frame.Fields[1].Labels, frame.Fields[2].Labels)
	}
	if first := frame.Fields[0].At(0).(time.Time); first.Minute() != 0 {
		t.Fatalf("rows should be sorted by time before the pivot, first is %v", first)
	}
	if *(frame.Fields[1].At(1).(*float64)) != 3 {
		t.Fatalf("series a at 09:05 should be 3, got %v", frame.Fields[1].At(1))
	}
	stub.results = `[{"AvgLoad":1}]`
	resp = runQuery(t, ds, QueryModel{SWQL: "SELECT c.AvgLoad FROM Orion.CPULoad c", Format: "timeseries"})
	if resp.Error == nil || !strings.Contains(resp.Error.Error(), "DateTime column") {
		t.Fatalf("time series without a time column should explain itself, got %v", resp.Error)
	}
}

func TestSwisErrorsSurfaceTheServerMessage(t *testing.T) {
	stub := newStub(t)
	stub.status, stub.failBody = http.StatusBadRequest, `{"Message":"Orion.Nodez has no such entity"}`
	ds := newDS(t, stub, nil)
	resp := runQuery(t, ds, QueryModel{SWQL: "SELECT n.Caption FROM Orion.Nodez n"})
	if resp.Error == nil || !strings.Contains(resp.Error.Error(), "Orion.Nodez has no such entity") || resp.Status != backend.StatusBadRequest {
		t.Fatalf("expected the SWIS message with a 400 status, got %v (%v)", resp.Error, resp.Status)
	}
}

func TestHealthCheck(t *testing.T) {
	stub := newStub(t)
	stub.results = `[{"EngineID":1,"ServerName":"ORION-MAIN","EngineVersion":"2026.2.0"}]`
	ds := newDS(t, stub, nil)
	res, _ := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if res.Status != backend.HealthStatusOk || !strings.Contains(res.Message, "ORION-MAIN") || !strings.Contains(res.Message, "2026.2.0") {
		t.Fatalf("unexpected health result: %+v", res)
	}
	if !strings.Contains(stub.lastQuery, "FROM Orion.Engines") {
		t.Fatalf("health check should query Orion.Engines, got %s", stub.lastQuery)
	}

	stub.results = `[]`
	res, _ = ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if res.Status != backend.HealthStatusError || !strings.Contains(res.Message, "account limitation") {
		t.Fatalf("an empty engines result should point at account limitations: %+v", res)
	}

	bad := newDS(t, stub, nil)
	bad.settings.Password = "wrong"
	bad.swis.password = "wrong"
	res, _ = bad.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if res.Status != backend.HealthStatusError || !strings.Contains(res.Message, "credentials") {
		t.Fatalf("bad credentials should be reported as such: %+v", res)
	}
}

func TestTLSVerificationIsOnByDefault(t *testing.T) {
	stub := newStub(t)
	settings := stub.settings(t, nil)
	delete(settings.DecryptedSecureJSONData, "caCert")
	inst, err := NewDatasource(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := inst.(*Datasource).CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if res.Status != backend.HealthStatusError || !strings.Contains(res.Message, "could not reach") {
		t.Fatalf("a self-signed certificate with no CA and no skip flag must fail verification: %+v", res)
	}
	skip := newDS(t, stub, map[string]any{"tlsSkipVerify": true})
	skip.settings.CACert = ""
	stub.results = `[{"EngineID":1,"ServerName":"x","EngineVersion":"y"}]`
	if res, _ := skip.CheckHealth(context.Background(), &backend.CheckHealthRequest{}); res.Status != backend.HealthStatusOk {
		t.Fatalf("tlsSkipVerify should connect: %+v", res)
	}
}

type captureSender struct {
	status int
	body   []byte
}

func (c *captureSender) Send(r *backend.CallResourceResponse) error {
	c.status, c.body = r.Status, r.Body
	return nil
}

func callInvoke(t *testing.T, ds *Datasource, role, entity, verb, body string) *captureSender {
	t.Helper()
	out := &captureSender{}
	err := ds.CallResource(context.Background(), &backend.CallResourceRequest{
		PluginContext: backend.PluginContext{User: &backend.User{Login: "sean", Role: role}},
		Method:        http.MethodPost,
		Path:          "invoke/" + entity + "/" + verb,
		Body:          []byte(body),
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInvokeIsGatedByAllowlistAndRole(t *testing.T) {
	stub := newStub(t)
	ds := newDS(t, stub, map[string]any{"invokeAllow": []string{"Orion.Nodes.PollNow"}})

	if out := callInvoke(t, ds, "Viewer", "Orion.Nodes", "PollNow", `["N:42"]`); out.status != http.StatusForbidden {
		t.Fatalf("a Viewer must not invoke: %d %s", out.status, out.body)
	}
	if out := callInvoke(t, ds, "Editor", "Orion.Nodes", "Unmanage", `["N:42","2026-09-16T00:00:00Z","2026-09-17T00:00:00Z",false]`); out.status != http.StatusForbidden || stub.lastInvoke != "" {
		t.Fatalf("a verb off the allowlist must be refused before reaching SWIS: %d %s", out.status, out.body)
	}
	if out := callInvoke(t, ds, "Editor", "Orion.Nodes", "PollNow", `{"netObjectId":"N:42"}`); out.status != http.StatusBadRequest {
		t.Fatalf("named arguments are not the SWIS contract and must be refused: %d %s", out.status, out.body)
	}
	out := callInvoke(t, ds, "Editor", "Orion.Nodes", "PollNow", `["N:42"]`)
	if out.status != http.StatusOK || stub.lastInvoke != "Orion.Nodes/PollNow" || len(stub.lastArgs) != 1 || stub.lastArgs[0] != "N:42" {
		t.Fatalf("allowed verb should reach SWIS positionally: %d %s invoke=%s args=%v", out.status, out.body, stub.lastInvoke, stub.lastArgs)
	}
	if out := callInvoke(t, ds, "Admin", "Orion.Nodes", "PollNow", ``); out.status != http.StatusOK || len(stub.lastArgs) != 0 {
		t.Fatalf("an empty body should invoke with no arguments: %d %s", out.status, out.body)
	}

	list := &captureSender{}
	_ = ds.CallResource(context.Background(), &backend.CallResourceRequest{Method: http.MethodGet, Path: "verbs"}, list)
	if list.status != http.StatusOK || !strings.Contains(string(list.body), "Orion.Nodes.PollNow") {
		t.Fatalf("GET verbs should list the allowlist: %d %s", list.status, list.body)
	}
}

func TestNoInvokeAllowlistMeansNoInvoke(t *testing.T) {
	stub := newStub(t)
	ds := newDS(t, stub, nil)
	if out := callInvoke(t, ds, "Admin", "Orion.Nodes", "PollNow", `["N:1"]`); out.status != http.StatusForbidden {
		t.Fatalf("with an empty allowlist even an Admin must be refused: %d %s", out.status, out.body)
	}
}

func TestPinnedStockStyleCertificate(t *testing.T) {
	cert, certPEM := selfSignedNoSAN(t, "SolarWinds-Orion")
	stub := newStubWithCert(t, cert)
	stub.results = `[{"EngineID":1,"ServerName":"x","EngineVersion":"y"}]`

	// Pinned, name check on: the chain is fine but the name is not, and the message says
	// which switch fixes it.
	settings := stub.settings(t, nil)
	settings.DecryptedSecureJSONData["caCert"] = string(certPEM)
	inst, err := NewDatasource(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	res, _ := inst.(*Datasource).CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if res.Status != backend.HealthStatusError || !strings.Contains(res.Message, "Ignore certificate name") {
		t.Fatalf("a pinned certificate with the wrong name should fail and point at the switch: %+v", res)
	}

	// Pinned, name check off: connects.
	settings = stub.settings(t, map[string]any{"tlsIgnoreHostname": true})
	settings.DecryptedSecureJSONData["caCert"] = string(certPEM)
	inst, _ = NewDatasource(context.Background(), settings)
	if res, _ := inst.(*Datasource).CheckHealth(context.Background(), &backend.CheckHealthRequest{}); res.Status != backend.HealthStatusOk {
		t.Fatalf("pinning with the name check off should connect: %+v", res)
	}

	// A different certificate pinned, name check off: still refused. This is what keeps
	// the mode from being "verification off".
	_, otherPEM := selfSignedNoSAN(t, "SolarWinds-Orion")
	settings = stub.settings(t, map[string]any{"tlsIgnoreHostname": true})
	settings.DecryptedSecureJSONData["caCert"] = string(otherPEM)
	inst, _ = NewDatasource(context.Background(), settings)
	if res, _ := inst.(*Datasource).CheckHealth(context.Background(), &backend.CheckHealthRequest{}); res.Status != backend.HealthStatusError || !strings.Contains(res.Message, "not the pinned one") {
		t.Fatalf("a certificate other than the pinned one must be refused even with the name check off: %+v", res)
	}

	// Nothing pinned, nothing skipped: refused, as before.
	settings = stub.settings(t, map[string]any{"tlsIgnoreHostname": true})
	delete(settings.DecryptedSecureJSONData, "caCert")
	inst, _ = NewDatasource(context.Background(), settings)
	if res, _ := inst.(*Datasource).CheckHealth(context.Background(), &backend.CheckHealthRequest{}); res.Status != backend.HealthStatusError {
		t.Fatalf("with no pinned certificate the name switch alone must not connect: %+v", res)
	}
}
