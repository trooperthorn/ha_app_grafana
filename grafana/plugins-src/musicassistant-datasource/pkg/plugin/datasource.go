package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"

	"github.com/trooperthorn/music-assistant/pkg/models"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// Datasource queries a Music Assistant server's HTTP API for player state:
// which players exist, their power/playback state and volume, and what
// each is currently playing.
type Datasource struct {
	client *musicAssistantClient
}

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	config, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	return &Datasource{
		client: newMusicAssistantClient(config.URL, config.Secrets.APIToken),
	}, nil
}

func (d *Datasource) Dispose() {}

// CheckHealth backs the "Save & test" button: it calls players/all, so a
// bad URL, an invalid token, and network failures all show up the same way
// "Save & test" would find a real query failing.
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

	client := newMusicAssistantClient(config.URL, config.Secrets.APIToken)
	if _, err := client.getPlayers(); err != nil {
		return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: "Successfully queried Music Assistant"}, nil
}

// queryModel is the JSON the frontend query editor sends per query row.
type queryModel struct {
	// Series selects which table this query returns: "players" (one row
	// per player with its power/playback/volume state) or "nowPlaying"
	// (one row per player currently playing something).
	Series string `json:"series"`
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
	if qm.Series == "" {
		qm.Series = "players"
	}

	players, err := d.client.getPlayers()
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	var frame *data.Frame
	switch qm.Series {
	case "players":
		frame = playersFrame(players)
	case "nowPlaying":
		frame = nowPlayingFrame(players)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown series %q", qm.Series))
	}

	return backend.DataResponse{Frames: data.Frames{frame}}
}

func playersFrame(players []playerState) *data.Frame {
	ids := make([]string, 0, len(players))
	names := make([]string, 0, len(players))
	available := make([]bool, 0, len(players))
	powered := make([]bool, 0, len(players))
	playbackState := make([]string, 0, len(players))
	volume := make([]int64, 0, len(players))
	elapsed := make([]float64, 0, len(players))

	for _, p := range players {
		ids = append(ids, p.PlayerID)
		names = append(names, p.Name)
		available = append(available, p.Available)
		powered = append(powered, p.Powered)
		playbackState = append(playbackState, p.PlaybackState)
		volume = append(volume, p.VolumeLevel)
		elapsed = append(elapsed, p.ElapsedTime)
	}

	frame := data.NewFrame("players",
		data.NewField("player_id", nil, ids),
		data.NewField("name", nil, names),
		data.NewField("available", nil, available),
		data.NewField("powered", nil, powered),
		data.NewField("playback_state", nil, playbackState),
		data.NewField("volume_level", nil, volume),
		data.NewField("elapsed_time", nil, elapsed),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}

func nowPlayingFrame(players []playerState) *data.Frame {
	ids := []string{}
	names := []string{}
	playbackState := []string{}
	titles := []string{}
	artists := []string{}
	albums := []string{}
	durations := []int64{}

	for _, p := range players {
		if p.CurrentMedia == nil {
			continue
		}
		ids = append(ids, p.PlayerID)
		names = append(names, p.Name)
		playbackState = append(playbackState, p.PlaybackState)
		titles = append(titles, p.CurrentMedia.Title)
		artists = append(artists, p.CurrentMedia.Artist)
		albums = append(albums, p.CurrentMedia.Album)
		durations = append(durations, p.CurrentMedia.Duration)
	}

	frame := data.NewFrame("now_playing",
		data.NewField("player_id", nil, ids),
		data.NewField("name", nil, names),
		data.NewField("playback_state", nil, playbackState),
		data.NewField("title", nil, titles),
		data.NewField("artist", nil, artists),
		data.NewField("album", nil, albums),
		data.NewField("duration", nil, durations),
	)
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisTypeTable}
	return frame
}
