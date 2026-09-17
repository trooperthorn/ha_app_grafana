package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/trooperthorn/unifi-protect/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource queries a Unifi Protect console's local Integration API for
// camera inventory and recording state.
//
// Protect 7.2.105 has no historical REST events/detections/alarms route
// (see docs/UNIFI-LOCAL-API-CONTRACT.md in trooperthorn/ha_int_soc, which
// searched all documented paths); live events exist only as the WebSocket
// subscription GET /subscribe/events. A Grafana backend plugin answers one
// HTTP request per query and holds no persistent connection between them,
// so it cannot honestly offer an events series without inventing an
// endpoint the API does not have. This plugin therefore surfaces cameras
// only.
type Datasource struct {
	client *unifiProtectClient
	origin string
}

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	client := newUnifiProtectClient(config.Host, config.Secrets.APIKey, config.VerifySSL)
	return &Datasource{client: client, origin: client.origin}, nil
}

func (d *Datasource) Dispose() {}

// CheckHealth backs the "Save & test" button: it fetches the camera list,
// so a bad host, an invalid key, and network failures all show up the same
// way "Save & test" would find a real query failing.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	config, err := models.LoadPluginSettings(*req.PluginContext.DataSourceInstanceSettings)
	if err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Unable to load settings"}, nil
	}
	if config.Host == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Console host is missing"}, nil
	}
	if config.Secrets.APIKey == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "API key is missing"}, nil
	}

	client := newUnifiProtectClient(config.Host, config.Secrets.APIKey, config.VerifySSL)
	if _, err := client.getCameras(ctx); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Successfully queried Unifi Protect"}, nil
}

// queryModel is the JSON the frontend query editor sends per query row.
// Series has one value ("cameras") today, kept as a field rather than a
// hardcoded assumption so a future series (should Protect ever document a
// REST events route) is an additive change, not a breaking one.
type queryModel struct {
	Series string `json:"series"`
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
		qm.Series = "cameras"
	}
	if qm.Series != "cameras" {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown series %q", qm.Series))
	}

	cameras, err := d.client.getCameras(ctx)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	return backend.DataResponse{Frames: data.Frames{camerasFrame(cameras, d.origin)}}
}

func camerasFrame(cameraRows []row, origin string) *data.Frame {
	ids := make([]string, 0, len(cameraRows))
	names := make([]string, 0, len(cameraRows))
	ips := make([]string, 0, len(cameraRows))
	macs := make([]string, 0, len(cameraRows))
	recording := make([]bool, 0, len(cameraRows))
	lastRing := make([]time.Time, 0, len(cameraRows))
	channelCount := make([]int64, 0, len(cameraRows))
	states := make([]string, 0, len(cameraRows))
	online := make([]bool, 0, len(cameraRows))
	links := make([]string, 0, len(cameraRows))

	for _, r := range cameraRows {
		c := normalizeCamera(r, origin)
		ids = append(ids, c.ID)
		names = append(names, c.Name)
		ips = append(ips, c.IP)
		macs = append(macs, c.MAC)
		recording = append(recording, c.IsRecording)
		lastRing = append(lastRing, epochOrZero(c.LastRing))
		channelCount = append(channelCount, c.ChannelCount)
		states = append(states, c.State)
		online = append(online, c.Online)
		links = append(links, c.Link)
	}

	frame := data.NewFrame("cameras",
		data.NewField("id", nil, ids),
		data.NewField("name", nil, names),
		data.NewField("ip", nil, ips),
		data.NewField("mac", nil, macs),
		data.NewField("is_recording", nil, recording),
		data.NewField("last_ring", nil, lastRing),
		data.NewField("channel_count", nil, channelCount),
		data.NewField("state", nil, states),
		data.NewField("online", nil, online),
		data.NewField("link", nil, links),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}

func epochOrZero(epoch int64) time.Time {
	if epoch <= 0 {
		return time.Time{}
	}
	return time.Unix(epoch, 0).UTC()
}
