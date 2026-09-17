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

	"github.com/trooperthorn/ha-soc/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource queries HA SOC (trooperthorn/ha_int_soc), a Home Assistant
// custom integration with no REST API of its own: it registers its
// ha_soc/* commands on Home Assistant's own core WebSocket API, so this
// plugin talks to Home Assistant directly, the same way the
// trooperthorn-homeassistant-datasource plugin does, and authenticates
// with an access token for an HA SOC admin user.
type Datasource struct {
	client *haSOCClient
}

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	return &Datasource{
		client: newHASOCClient(config.URL, config.Secrets.AccessToken, config.VerifySSL),
	}, nil
}

func (d *Datasource) Dispose() {}

// CheckHealth backs the "Save & test" button: it fetches the risk posture,
// so a bad URL, an invalid token, and network failures all show up the
// same way "Save & test" would find a real query failing. A "not
// authorized" error here usually means the token belongs to a non-admin
// user, or access_level is owner_only and the token's user is not the
// owner - see require_soc_access in HA SOC's own websocket_api.py.
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

	client := newHASOCClient(config.URL, config.Secrets.AccessToken, config.VerifySSL)
	if _, err := client.riskPosture(ctx); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Successfully queried HA SOC"}, nil
}

// queryModel is the JSON the frontend query editor sends per query row.
type queryModel struct {
	// Series selects "posture" (one row, the whole-install score),
	// "risk" (one row per user), or "audit" (one row per audit log event,
	// bounded to the dashboard's own time range).
	Series string `json:"series"`
	// AuditLimit caps the "audit" series (HA SOC's own default is 200).
	AuditLimit int `json:"auditLimit"`
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
		qm.Series = "posture"
	}

	switch qm.Series {
	case "posture":
		return d.queryPosture(ctx)
	case "risk":
		return d.queryRisk(ctx)
	case "audit":
		return d.queryAudit(ctx, qm, query.TimeRange)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown series %q", qm.Series))
	}
}

func (d *Datasource) queryPosture(ctx context.Context) backend.DataResponse {
	p, err := d.client.riskPosture(ctx)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	frame := data.NewFrame("posture",
		data.NewField("time", nil, []time.Time{time.Now()}),
		data.NewField("score", nil, []int64{int64(p.Score)}),
		data.NewField("grade", nil, []string{p.Grade}),
		data.NewField("provisional", nil, []bool{p.Provisional}),
		data.NewField("missing_terms", nil, []string{strings.Join(p.MissingTerms, ",")}),
		data.NewField("p_user", nil, []float64{p.Breakdown.PUser}),
		data.NewField("p_vuln", nil, []float64{p.Breakdown.PVuln}),
		data.NewField("p_misconfig", nil, []float64{p.Breakdown.PMisconfig}),
		data.NewField("p_integration", nil, []float64{p.Breakdown.PIntegration}),
		data.NewField("p_detection", nil, []float64{p.Breakdown.PDetection}),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return backend.DataResponse{Frames: data.Frames{frame}}
}

func (d *Datasource) queryRisk(ctx context.Context) backend.DataResponse {
	users, err := d.client.riskList(ctx)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	userIDs := make([]string, 0, len(users))
	scores := make([]int64, 0, len(users))
	bands := make([]string, 0, len(users))
	factorCounts := make([]int64, 0, len(users))
	topFactors := make([]string, 0, len(users))

	for _, u := range users {
		userIDs = append(userIDs, u.UserID)
		scores = append(scores, int64(u.Score))
		bands = append(bands, u.Band)
		factorCounts = append(factorCounts, int64(len(u.Factors)))
		topFactors = append(topFactors, u.topFactor())
	}

	frame := data.NewFrame("risk",
		data.NewField("user_id", nil, userIDs),
		data.NewField("score", nil, scores),
		data.NewField("band", nil, bands),
		data.NewField("factor_count", nil, factorCounts),
		data.NewField("top_factor", nil, topFactors),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return backend.DataResponse{Frames: data.Frames{frame}}
}

func (d *Datasource) queryAudit(ctx context.Context, qm queryModel, tr backend.TimeRange) backend.DataResponse {
	limit := qm.AuditLimit
	if limit <= 0 {
		limit = 200
	}

	events, err := d.client.auditQuery(ctx, tr.From.UTC().Format(time.RFC3339), tr.To.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	times := make([]time.Time, 0, len(events))
	categories := make([]string, 0, len(events))
	userIDs := make([]string, 0, len(events))
	domains := make([]string, 0, len(events))
	services := make([]string, 0, len(events))
	entityIDs := make([]string, 0, len(events))
	ips := make([]string, 0, len(events))
	seqs := make([]int64, 0, len(events))

	for _, e := range events {
		ts, err := time.Parse(time.RFC3339, e.TS)
		if err != nil {
			ts = time.Time{}
		}
		times = append(times, ts.UTC())
		categories = append(categories, e.Category)
		userIDs = append(userIDs, e.UserID)
		domains = append(domains, e.Domain)
		services = append(services, e.Service)
		entityIDs = append(entityIDs, strings.Join(e.EntityIDs, ","))
		ips = append(ips, e.IP)
		seqs = append(seqs, e.Seq)
	}

	frame := data.NewFrame("audit",
		data.NewField("time", nil, times),
		data.NewField("category", nil, categories),
		data.NewField("user_id", nil, userIDs),
		data.NewField("domain", nil, domains),
		data.NewField("service", nil, services),
		data.NewField("entity_ids", nil, entityIDs),
		data.NewField("ip", nil, ips),
		data.NewField("seq", nil, seqs),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return backend.DataResponse{Frames: data.Frames{frame}}
}
