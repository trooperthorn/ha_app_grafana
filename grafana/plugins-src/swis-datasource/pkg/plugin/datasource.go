// Package plugin implements a Grafana backend data source for the SolarWinds Information
// Service (SWIS), the API behind SolarWinds Observability Self-Hosted (Orion).
//
// The query editor takes SWQL. The backend expands the Grafana time-range macros into
// bound parameters, runs the statement over the REST/JSON contract on port 17774, and
// returns a typed data frame. Verbs are exposed as a plugin resource, gated by an
// allowlist the administrator sets per data source, so a dashboard can offer "poll now"
// or "acknowledge" without the data source being able to unmanage the estate.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource is one configured SWIS connection. Grafana creates one per data source
// instance and disposes it when the settings change.
type Datasource struct {
	settings *Settings
	swis     *SwisClient
}

// NewDatasource is the instance factory registered in main.
func NewDatasource(_ context.Context, src backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	settings, err := LoadSettings(src)
	if err != nil {
		return nil, err
	}
	client, err := NewSwisClient(settings)
	if err != nil {
		return nil, err
	}
	return &Datasource{settings: settings, swis: client}, nil
}

// Dispose releases the HTTP connections held for this instance.
func (d *Datasource) Dispose() {
	if d.swis != nil {
		d.swis.Close()
	}
}

// QueryModel is the JSON the query editor stores on a panel.
type QueryModel struct {
	SWQL       string         `json:"swql"`
	Format     string         `json:"format"` // "table" (default) or "timeseries"
	Parameters map[string]any `json:"parameters,omitempty"`
}

// QueryData runs each panel query and returns one frame per RefID.
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()
	for _, q := range req.Queries {
		response.Responses[q.RefID] = d.query(ctx, q)
	}
	return response, nil
}

func (d *Datasource) query(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	var model QueryModel
	if err := json.Unmarshal(q.JSON, &model); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, "could not read the query: "+err.Error())
	}
	swql := stripComments(model.SWQL)
	if swql == "" {
		return backend.DataResponse{}
	}

	expanded, used, err := ExpandMacros(swql)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, err.Error())
	}
	params := map[string]any{}
	for k, v := range model.Parameters {
		params[k] = v
	}
	if used[paramTimeFrom] {
		params[paramTimeFrom] = swisTime(q.TimeRange.From)
	}
	if used[paramTimeTo] {
		params[paramTimeTo] = swisTime(q.TimeRange.To)
	}

	started := time.Now()
	raw, err := d.swis.Query(ctx, expanded, params)
	if err != nil {
		status := backend.StatusInternal
		if se, ok := err.(*SwisError); ok && se.Status == http.StatusBadRequest {
			status = backend.StatusBadRequest
		}
		return backend.ErrDataResponse(status, err.Error())
	}

	frame, truncated, err := ToFrame(q.RefID, raw, d.settings.MaxRows)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}
	frame.Meta = &data.FrameMeta{
		ExecutedQueryString:    expanded,
		PreferredVisualization: data.VisTypeTable,
	}
	if truncated {
		frame.AppendNotices(data.Notice{
			Severity: data.NoticeSeverityWarning,
			Text:     fmt.Sprintf("SWIS returned more than %d rows; the rest were dropped. Add TOP n or a tighter WHERE clause.", d.settings.MaxRows),
		})
	}
	log.DefaultLogger.Debug("swql query", "refId", q.RefID, "rows", frame.Rows(), "ms", time.Since(started).Milliseconds())

	if model.Format == "timeseries" {
		frame.Meta.PreferredVisualization = data.VisTypeGraph
		if wide, err := toTimeSeries(frame); err == nil {
			frame = wide
		} else {
			return backend.ErrDataResponse(backend.StatusBadRequest, err.Error())
		}
	}
	return backend.DataResponse{Frames: data.Frames{frame}}
}

// toTimeSeries makes a frame that the time series panel accepts. A frame with a time
// column plus numeric columns already is one. A frame that also carries string columns
// is "long" format (one row per timestamp per label), which LongToWide pivots so each
// distinct label combination becomes its own series.
func toTimeSeries(frame *data.Frame) (*data.Frame, error) {
	hasTime, hasString := false, false
	for _, f := range frame.Fields {
		switch f.Type() {
		case data.FieldTypeNullableTime, data.FieldTypeTime:
			hasTime = true
		case data.FieldTypeNullableString, data.FieldTypeString:
			hasString = true
		}
	}
	if !hasTime {
		return nil, fmt.Errorf("time series format needs a DateTime column in the SELECT list (for example c.DateTime)")
	}
	if !hasString || frame.Rows() == 0 {
		return frame, nil
	}
	// LongToWide requires the time field first and rows in ascending time order. The
	// order is enforced here rather than trusted, because a panel author who forgets
	// ORDER BY should get a chart rather than an error about frame layout.
	reordered := data.NewFrame(frame.Name)
	reordered.Meta = frame.Meta
	var timeField *data.Field
	for _, f := range frame.Fields {
		if timeField == nil && (f.Type() == data.FieldTypeNullableTime || f.Type() == data.FieldTypeTime) {
			timeField = f
		}
	}
	reordered.Fields = append(reordered.Fields, timeField)
	for _, f := range frame.Fields {
		if f != timeField {
			reordered.Fields = append(reordered.Fields, f)
		}
	}
	reordered = sortRowsByTime(reordered)
	wide, err := data.LongToWide(reordered, nil)
	if err != nil {
		return nil, fmt.Errorf("could not pivot rows into series: %w (order the query by the time column)", err)
	}
	wide.Meta = frame.Meta
	return wide, nil
}

// CheckHealth backs the "Save & test" button. It runs the smallest query that proves the
// credentials work and the account can see something: the polling engines.
func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	raw, err := d.swis.Query(ctx, "SELECT TOP 1 e.EngineID, e.ServerName, e.EngineVersion FROM Orion.Engines e ORDER BY e.EngineID", nil)
	if err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}
	rows, err := decodeRows(raw)
	if err != nil || len(rows) == 0 {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "connected and authenticated, but the account cannot see Orion.Engines; check its account limitation",
		}, nil
	}
	var server, version string
	_ = json.Unmarshal(rows[0].values["ServerName"], &server)
	_ = json.Unmarshal(rows[0].values["EngineVersion"], &version)
	msg := fmt.Sprintf("Connected to SWIS on %s (engine %s, platform %s).", d.settings.Host, server, version)
	if len(d.settings.InvokeAllow) > 0 {
		msg += fmt.Sprintf(" Invoke is enabled for %d verb(s).", len(d.settings.InvokeAllow))
	}
	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: msg}, nil
}

// Resources: the verb surface. Grafana routes
//
//	GET  /api/datasources/uid/<uid>/resources/verbs
//	POST /api/datasources/uid/<uid>/resources/invoke/<Entity>/<Verb>   body: positional JSON array
//
// to CallResource. The frontend's DataSource.invoke() helper and any panel that can POST
// to a data source resource (a form or button panel) use these.

var namePartRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.]*$`)

type resourceError struct {
	Error string `json:"error"`
}

func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	path := strings.Trim(req.Path, "/")
	parts := strings.Split(path, "/")

	switch {
	case req.Method == http.MethodGet && path == "verbs":
		return sendJSON(sender, http.StatusOK, map[string]any{"invokeAllow": d.settings.InvokeAllow})

	case req.Method == http.MethodPost && len(parts) == 3 && parts[0] == "invoke":
		return d.invoke(ctx, req, sender, parts[1], parts[2])

	default:
		return sendJSON(sender, http.StatusNotFound, resourceError{"unknown resource; use GET verbs or POST invoke/<Entity>/<Verb>"})
	}
}

func (d *Datasource) invoke(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender, entity, verb string) error {
	user := "unknown"
	role := ""
	if req.PluginContext.User != nil {
		user = req.PluginContext.User.Login
		role = req.PluginContext.User.Role
	}
	full := entity + "." + verb

	if !namePartRE.MatchString(entity) || !namePartRE.MatchString(verb) {
		return sendJSON(sender, http.StatusBadRequest, resourceError{"malformed entity or verb name"})
	}
	// Viewers can look; changing the monitored estate needs an editor or an admin. The
	// role comes from Grafana, which authenticated the user, not from the request body.
	if role != "Admin" && role != "Editor" {
		log.DefaultLogger.Warn("invoke refused: insufficient Grafana role", "verb", full, "user", user, "role", role)
		return sendJSON(sender, http.StatusForbidden, resourceError{"invoking a verb needs the Editor or Admin role in Grafana"})
	}
	if !d.settings.InvokeAllowed(entity, verb) {
		log.DefaultLogger.Warn("invoke refused: verb not on the data source allowlist", "verb", full, "user", user)
		return sendJSON(sender, http.StatusForbidden, resourceError{full + " is not on this data source's invoke allowlist"})
	}
	var args []any
	if len(strings.TrimSpace(string(req.Body))) > 0 {
		if err := json.Unmarshal(req.Body, &args); err != nil {
			return sendJSON(sender, http.StatusBadRequest, resourceError{"the body must be a JSON array of positional arguments"})
		}
	}
	// Every change made through a dashboard is logged with who made it. This is the
	// audit line, so it carries the arguments too.
	log.DefaultLogger.Info("invoke", "verb", full, "user", user, "args", string(req.Body))

	result, err := d.swis.Invoke(ctx, entity, verb, args)
	if err != nil {
		status := http.StatusBadGateway
		if se, ok := err.(*SwisError); ok && se.Status != 0 {
			status = se.Status
		}
		return sendJSON(sender, status, resourceError{err.Error()})
	}
	return sendJSON(sender, http.StatusOK, map[string]any{"invoked": full, "result": result})
}

func sendJSON(sender backend.CallResourceResponseSender, status int, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return sender.Send(&backend.CallResourceResponse{
		Status:  status,
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body:    body,
	})
}

// sortRowsByTime returns a copy of a frame whose first field is time, with the rows in
// ascending time order and nil times last. It is a stable sort, so rows sharing a
// timestamp keep their query order.
func sortRowsByTime(frame *data.Frame) *data.Frame {
	n := frame.Rows()
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	at := func(i int) (time.Time, bool) {
		v := frame.Fields[0].At(i)
		switch t := v.(type) {
		case *time.Time:
			if t == nil {
				return time.Time{}, false
			}
			return *t, true
		case time.Time:
			return t, true
		}
		return time.Time{}, false
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ta, oka := at(idx[a])
		tb, okb := at(idx[b])
		if oka != okb {
			return oka
		}
		return ta.Before(tb)
	})
	out := data.NewFrame(frame.Name)
	out.Meta = frame.Meta
	for _, f := range frame.Fields {
		nf := data.NewFieldFromFieldType(f.Type(), n)
		nf.Name, nf.Labels, nf.Config = f.Name, f.Labels, f.Config
		for i, src := range idx {
			nf.Set(i, f.At(src))
		}
		out.Fields = append(out.Fields, nf)
	}
	return out
}
