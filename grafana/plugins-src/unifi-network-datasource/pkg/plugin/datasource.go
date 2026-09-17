package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/trooperthorn/unifi-network/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource queries a Unifi Network controller's local Integration API for
// connected clients, network infrastructure devices, and derived WAN status.
type Datasource struct {
	client *unifiNetworkClient
}

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	return &Datasource{
		client: newUnifiNetworkClient(config.Host, config.Secrets.APIKey, config.VerifySSL),
	}, nil
}

func (d *Datasource) Dispose() {}

// CheckHealth backs the "Save & test" button: it resolves the site id and
// fetches the client list, so a bad host, an invalid key, and network
// failures all show up the same way "Save & test" would find a real query
// failing.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	config, err := models.LoadPluginSettings(*req.PluginContext.DataSourceInstanceSettings)
	if err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Unable to load settings"}, nil
	}
	if config.Host == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "Controller host is missing"}, nil
	}
	if config.Secrets.APIKey == "" {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: "API key is missing"}, nil
	}

	client := newUnifiNetworkClient(config.Host, config.Secrets.APIKey, config.VerifySSL)
	siteID, err := client.resolveSiteID(ctx)
	if err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}
	if _, err := client.getClients(ctx, siteID); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Successfully queried Unifi Network"}, nil
}

// queryModel is the JSON the frontend query editor sends per query row.
type queryModel struct {
	// Series selects which table this query returns: "clients", "devices",
	// or "wan" (a single-row WAN status derived from the gateway device).
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
		qm.Series = "clients"
	}

	siteID, err := d.client.resolveSiteID(ctx)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	var frame *data.Frame
	switch qm.Series {
	case "clients":
		clients, err := d.client.getClients(ctx, siteID)
		if err != nil {
			return backend.ErrDataResponse(backend.StatusInternal, err.Error())
		}
		frame = clientsFrame(clients)
	case "devices":
		devices, err := d.client.getDevices(ctx, siteID)
		if err != nil {
			return backend.ErrDataResponse(backend.StatusInternal, err.Error())
		}
		frame = devicesFrame(devices)
	case "wan":
		devices, err := d.client.getDevices(ctx, siteID)
		if err != nil {
			return backend.ErrDataResponse(backend.StatusInternal, err.Error())
		}
		frame = wanFrame(devices)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown series %q", qm.Series))
	}

	return backend.DataResponse{Frames: data.Frames{frame}}
}

func clientsFrame(clientRows []row) *data.Frame {
	now := time.Now().Unix()
	names := make([]string, 0, len(clientRows))
	macs := make([]string, 0, len(clientRows))
	ipv4s := make([]string, 0, len(clientRows))
	vlans := make([]string, 0, len(clientRows))
	ssids := make([]string, 0, len(clientRows))
	wired := make([]bool, 0, len(clientRows))
	uptime := make([]int64, 0, len(clientRows))
	lastSeen := make([]time.Time, 0, len(clientRows))
	rx := make([]int64, 0, len(clientRows))
	tx := make([]int64, 0, len(clientRows))

	for _, r := range clientRows {
		c := normalizeClient(r, now)
		names = append(names, c.Name)
		macs = append(macs, c.MAC)
		ipv4s = append(ipv4s, c.IPv4)
		vlans = append(vlans, c.VLAN)
		ssids = append(ssids, c.SSID)
		wired = append(wired, c.Wired)
		uptime = append(uptime, c.UptimeS)
		lastSeen = append(lastSeen, epochOrZero(c.LastSeen))
		rx = append(rx, c.RXBytes)
		tx = append(tx, c.TXBytes)
	}

	frame := data.NewFrame("clients",
		data.NewField("name", nil, names),
		data.NewField("mac", nil, macs),
		data.NewField("ipv4", nil, ipv4s),
		data.NewField("vlan", nil, vlans),
		data.NewField("ssid", nil, ssids),
		data.NewField("wired", nil, wired),
		data.NewField("uptime_s", nil, uptime),
		data.NewField("last_seen", nil, lastSeen),
		data.NewField("rx_bytes", nil, rx),
		data.NewField("tx_bytes", nil, tx),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}

func devicesFrame(deviceRows []row) *data.Frame {
	names := make([]string, 0, len(deviceRows))
	macs := make([]string, 0, len(deviceRows))
	ipv4s := make([]string, 0, len(deviceRows))
	models_ := make([]string, 0, len(deviceRows))
	states := make([]string, 0, len(deviceRows))
	fwUpdatable := make([]bool, 0, len(deviceRows))
	lastSeen := make([]time.Time, 0, len(deviceRows))
	rx := make([]int64, 0, len(deviceRows))
	tx := make([]int64, 0, len(deviceRows))

	for _, r := range deviceRows {
		dev := normalizeDevice(r)
		names = append(names, dev.Name)
		macs = append(macs, dev.MAC)
		ipv4s = append(ipv4s, dev.IPv4)
		models_ = append(models_, dev.Model)
		states = append(states, dev.State)
		fwUpdatable = append(fwUpdatable, dev.FirmwareUpdatable)
		lastSeen = append(lastSeen, epochOrZero(dev.LastSeen))
		rx = append(rx, dev.RXBytes)
		tx = append(tx, dev.TXBytes)
	}

	frame := data.NewFrame("devices",
		data.NewField("name", nil, names),
		data.NewField("mac", nil, macs),
		data.NewField("ipv4", nil, ipv4s),
		data.NewField("model", nil, models_),
		data.NewField("state", nil, states),
		data.NewField("firmware_updatable", nil, fwUpdatable),
		data.NewField("last_seen", nil, lastSeen),
		data.NewField("rx_bytes", nil, rx),
		data.NewField("tx_bytes", nil, tx),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}

func wanFrame(deviceRows []row) *data.Frame {
	wan := derivedWAN(deviceRows)
	frame := data.NewFrame("wan",
		data.NewField("time", nil, []time.Time{time.Now()}),
		data.NewField("port", nil, []string{wan.Port}),
		data.NewField("up", nil, []bool{wan.Up}),
		data.NewField("rx_rate_bps", nil, []int64{wan.RXBps}),
		data.NewField("tx_rate_bps", nil, []int64{wan.TXBps}),
		data.NewField("ip", nil, []string{wan.IP}),
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
