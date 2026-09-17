package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/trooperthorn/home-assistant/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource queries a Home Assistant instance's own core WebSocket API for
// long-term statistics (recorder/statistics_during_period) and system
// health (system_health/info). Both are WebSocket-only commands with no
// REST equivalent, unlike entity state, which HA's REST /api/states
// already serves and which this plugin does not duplicate.
type Datasource struct {
	client *haClient
}

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	return &Datasource{
		client: newHAClient(config.URL, config.Secrets.AccessToken, config.VerifySSL),
	}, nil
}

func (d *Datasource) Dispose() {}

// CheckHealth backs the "Save & test" button: it fetches system health, so
// a bad URL, an invalid token, and network failures all show up the same
// way "Save & test" would find a real query failing.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	config, err := models.LoadPluginSettings(*req.PluginContext.DataSourceInstanceSettings)
	if err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Unable to load settings"}, nil
	}
	if config.URL == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Instance URL is missing"}, nil
	}
	if config.Secrets.AccessToken == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Access token is missing"}, nil
	}

	client := newHAClient(config.URL, config.Secrets.AccessToken, config.VerifySSL)
	if _, err := client.systemHealthInfo(ctx); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Successfully queried Home Assistant"}, nil
}

// queryModel is the JSON the frontend query editor sends per query row.
type queryModel struct {
	// Series selects "statistics" (long-term statistics for one or more
	// statistic ids) or "system_health" (one row per domain/key health fact).
	Series string `json:"series"`
	// StatisticIDs, comma-separated (e.g. "sensor.outdoor_temperature"),
	// used only by the "statistics" series.
	StatisticIDs string `json:"statisticIds"`
	// Period is one of Home Assistant's own bucket sizes: 5minute, hour,
	// day, week, month, year.
	Period string `json:"period"`
}

func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		response.Responses[q.RefID] = d.query(ctx, q)
	}

	return response, nil
}

func (d *Datasource) query(ctx context.Context, query backend.DataQuery) backend.DataResponse {
	var qm queryModel
	if err := json.Unmarshal(query.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("json unmarshal: %v", err))
	}
	if qm.Series == "" {
		qm.Series = "statistics"
	}

	switch qm.Series {
	case "statistics":
		return d.queryStatistics(ctx, qm, query.TimeRange)
	case "system_health":
		return d.querySystemHealth(ctx)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown series %q", qm.Series))
	}
}

func (d *Datasource) queryStatistics(ctx context.Context, qm queryModel, tr backend.TimeRange) backend.DataResponse {
	ids := splitNonEmpty(qm.StatisticIDs)
	if len(ids) == 0 {
		return backend.ErrDataResponse(backend.StatusBadRequest, "at least one statistic id is required")
	}
	period := qm.Period
	if period == "" {
		period = "hour"
	}

	params := map[string]any{
		"statistic_ids": ids,
		"start_time":    tr.From.UTC().Format(time.RFC3339),
		"end_time":      tr.To.UTC().Format(time.RFC3339),
		"period":        period,
		"types":         []string{"mean", "min", "max", "state", "sum"},
	}
	raw, err := d.client.call(ctx, "recorder/statistics_during_period", params)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	var result map[string][]statPoint
	if err := json.Unmarshal(raw, &result); err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("decoding statistics result: %v", err))
	}

	frames := make(data.Frames, 0, len(ids))
	for _, id := range ids {
		frames = append(frames, statisticFrame(id, result[id]))
	}
	return backend.DataResponse{Frames: frames}
}

// statPoint is one entry of recorder/statistics_during_period's per-statistic
// array. start/end arrive as epoch milliseconds; fields the request did not
// ask for, or that this bucket has none of, are simply absent.
type statPoint struct {
	Start float64  `json:"start"`
	Mean  *float64 `json:"mean"`
	Min   *float64 `json:"min"`
	Max   *float64 `json:"max"`
	State *float64 `json:"state"`
	Sum   *float64 `json:"sum"`
}

func statisticFrame(statisticID string, points []statPoint) *data.Frame {
	times := make([]time.Time, 0, len(points))
	mean := make([]*float64, 0, len(points))
	min := make([]*float64, 0, len(points))
	max := make([]*float64, 0, len(points))
	state := make([]*float64, 0, len(points))
	sum := make([]*float64, 0, len(points))

	for _, p := range points {
		times = append(times, time.UnixMilli(int64(p.Start)).UTC())
		mean = append(mean, p.Mean)
		min = append(min, p.Min)
		max = append(max, p.Max)
		state = append(state, p.State)
		sum = append(sum, p.Sum)
	}

	frame := data.NewFrame(statisticID,
		data.NewField("time", nil, times),
		data.NewField("mean", nil, mean),
		data.NewField("min", nil, min),
		data.NewField("max", nil, max),
		data.NewField("state", nil, state),
		data.NewField("sum", nil, sum),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeGraph}
	return frame
}

func (d *Datasource) querySystemHealth(ctx context.Context) backend.DataResponse {
	info, err := d.client.systemHealthInfo(ctx)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	domains := []string{}
	keys := []string{}
	values := []string{}
	for domain, facts := range info {
		for key, value := range facts {
			domains = append(domains, domain)
			keys = append(keys, key)
			values = append(values, fmt.Sprint(value))
		}
	}

	frame := data.NewFrame("system_health",
		data.NewField("domain", nil, domains),
		data.NewField("key", nil, keys),
		data.NewField("value", nil, values),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return backend.DataResponse{Frames: data.Frames{frame}}
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
