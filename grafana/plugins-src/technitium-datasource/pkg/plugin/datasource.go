package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/trooperthorn/technitium-dns/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource queries a Technitium DNS Server's HTTP API for dashboard
// statistics: query volume over time, response totals, and the top
// clients/domains/blocked-domains tables.
type Datasource struct {
	client           *technitiumClient
	queryLogsAppName string
}

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	return &Datasource{
		client:           newTechnitiumClient(config.URL, config.Secrets.APIToken),
		queryLogsAppName: config.QueryLogsAppName,
	}, nil
}

func (d *Datasource) Dispose() {}

// CheckHealth backs the "Save & test" button on the datasource config page.
// It calls the same stats endpoint QueryData does, over the shortest
// window, so a bad URL, an invalid token, and network failures all show up
// the same way "Save & test" would find a real query failing.
func (d *Datasource) CheckHealth(_ context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	config, err := models.LoadPluginSettings(*req.PluginContext.DataSourceInstanceSettings)
	if err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Unable to load settings"}, nil
	}
	if config.URL == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Server URL is missing"}, nil
	}
	if config.Secrets.APIToken == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "API token is missing"}, nil
	}

	client := newTechnitiumClient(config.URL, config.Secrets.APIToken)
	if _, err := client.getDashboardStats("LastHour"); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Successfully queried Technitium DNS Server"}, nil
}

// queryModel is the JSON the frontend query editor sends per query row.
type queryModel struct {
	// StatType is one of LastHour, LastDay, LastWeek, LastMonth, LastYear,
	// as Technitium's /api/dashboard/stats/get documents.
	StatType string `json:"statType"`
	// Series selects which part of the response becomes this query's
	// frame(s): "volume" (the time series), "topClients", "topDomains",
	// "topBlockedDomains" (each a table of name+hits), or "queryLogs" (the
	// installed Query Logs app's stored request/response log, over the
	// dashboard's own time range rather than StatType).
	Series string `json:"series"`
	// QName and ClientIPAddress filter the "queryLogs" series; both are
	// optional and passed through to the Query Logs app's own filtering.
	QName           string `json:"qname"`
	ClientIPAddress string `json:"clientIpAddress"`
}

func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		response.Responses[q.RefID] = d.query(ctx, q)
	}

	return response, nil
}

func (d *Datasource) query(_ context.Context, query backend.DataQuery) backend.DataResponse {
	var qm queryModel
	if err := json.Unmarshal(query.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("json unmarshal: %v", err))
	}
	if qm.StatType == "" {
		qm.StatType = "LastDay"
	}
	if qm.Series == "" {
		qm.Series = "volume"
	}

	if qm.Series == "queryLogs" {
		entries, err := d.client.getQueryLogs(queryLogsFilter{
			appName:         d.queryLogsAppName,
			start:           query.TimeRange.From,
			end:             query.TimeRange.To,
			clientIPAddress: qm.ClientIPAddress,
			qname:           qm.QName,
		})
		if err != nil {
			return backend.ErrDataResponse(backend.StatusInternal, err.Error())
		}
		return backend.DataResponse{Frames: data.Frames{queryLogsFrame(entries)}}
	}

	stats, err := d.client.getDashboardStats(qm.StatType)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	var frame *data.Frame
	switch qm.Series {
	case "volume":
		frame = volumeFrame(stats)
	case "topClients":
		frame = topNFrame("top_clients", stats.Response.TopClients)
	case "topDomains":
		frame = topNFrame("top_domains", stats.Response.TopDomains)
	case "topBlockedDomains":
		frame = topNFrame("top_blocked_domains", stats.Response.TopBlockedDomains)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown series %q", qm.Series))
	}

	return backend.DataResponse{Frames: data.Frames{frame}}
}

// volumeFrame turns mainChartData's labels/datasets into a wide time series
// frame: one "time" field plus one numeric field per dataset (e.g. Total
// Queries, No Error, Server Failure, ...).
func volumeFrame(stats *dashboardStatsResponse) *data.Frame {
	chart := stats.Response.MainChartData
	frame := data.NewFrame("dashboard_stats",
		data.NewField("time", nil, chart.Labels),
	)
	for _, ds := range chart.Datasets {
		frame.Fields = append(frame.Fields, data.NewField(ds.Label, nil, ds.Data))
	}
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeGraph}
	return frame
}

// queryLogsFrame turns a page of Query Logs app entries into a table frame:
// one row per DNS request/response, oldest first. A timestamp this plugin
// cannot parse is left as the zero time rather than dropping the row, since
// Technitium's own format has changed across app versions.
func queryLogsFrame(entries []queryLogEntry) *data.Frame {
	times := make([]time.Time, 0, len(entries))
	clients := make([]string, 0, len(entries))
	protocols := make([]string, 0, len(entries))
	responseTypes := make([]string, 0, len(entries))
	rcodes := make([]string, 0, len(entries))
	qnames := make([]string, 0, len(entries))
	qtypes := make([]string, 0, len(entries))
	qclasses := make([]string, 0, len(entries))
	answers := make([]string, 0, len(entries))
	rtts := make([]*float64, 0, len(entries))

	for _, e := range entries {
		ts, _ := time.Parse(time.RFC3339Nano, e.Timestamp)
		times = append(times, ts)
		clients = append(clients, e.ClientIPAddress)
		protocols = append(protocols, e.Protocol)
		responseTypes = append(responseTypes, e.ResponseType)
		rcodes = append(rcodes, e.RCode)
		qnames = append(qnames, e.QName)
		qtypes = append(qtypes, e.QType)
		qclasses = append(qclasses, e.QClass)
		answers = append(answers, e.Answer)
		rtts = append(rtts, e.ResponseRtt)
	}

	frame := data.NewFrame("query_logs",
		data.NewField("time", nil, times),
		data.NewField("clientIpAddress", nil, clients),
		data.NewField("protocol", nil, protocols),
		data.NewField("responseType", nil, responseTypes),
		data.NewField("rcode", nil, rcodes),
		data.NewField("qname", nil, qnames),
		data.NewField("qtype", nil, qtypes),
		data.NewField("qclass", nil, qclasses),
		data.NewField("answer", nil, answers),
		data.NewField("responseRttMs", nil, rtts),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}

func topNFrame(name string, rows []topRow) *data.Frame {
	names := make([]string, 0, len(rows))
	hits := make([]float64, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Name)
		hits = append(hits, r.Hits)
	}

	frame := data.NewFrame(name,
		data.NewField("name", nil, names),
		data.NewField("hits", nil, hits),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}
