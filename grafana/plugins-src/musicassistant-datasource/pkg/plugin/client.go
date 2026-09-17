package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// commandMessage is the request envelope for Music Assistant's HTTP
// JSON-RPC endpoint, POST /api. See
// music_assistant_models.api.CommandMessage in
// https://github.com/music-assistant/models.
type commandMessage struct {
	MessageID int64          `json:"message_id"`
	Command   string         `json:"command"`
	Args      map[string]any `json:"args"`
}

// resultEnvelope covers both music_assistant_models.api.SuccessResultMessage
// (Result populated, ErrorCode empty) and ErrorResultMessage (ErrorCode set).
type resultEnvelope struct {
	MessageID int64           `json:"message_id"`
	Result    json.RawMessage `json:"result"`
	ErrorCode string          `json:"error_code"`
	Details   string          `json:"details"`
}

// currentMedia is music_assistant_models.player.PlayerMedia, trimmed to the
// fields this plugin surfaces.
type currentMedia struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int64  `json:"duration"`
}

// playerState is music_assistant_models.player.Player ("PlayerState" in the
// players/all response), trimmed to the fields this plugin surfaces.
type playerState struct {
	PlayerID      string        `json:"player_id"`
	Name          string        `json:"name"`
	Available     bool          `json:"available"`
	Powered       bool          `json:"powered"`
	PlaybackState string        `json:"playback_state"`
	VolumeLevel   int64         `json:"volume_level"`
	ElapsedTime   float64       `json:"elapsed_time"`
	CurrentMedia  *currentMedia `json:"current_media"`
}

// musicAssistantClient talks to a Music Assistant server's HTTP JSON-RPC
// endpoint (POST /api) using a long-lived API token, never a short-lived
// login session token.
type musicAssistantClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
	nextID     atomic.Int64
}

func newMusicAssistantClient(baseURL, apiToken string) *musicAssistantClient {
	return &musicAssistantClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// call invokes one API command and decodes its result into out (nil to
// discard it).
func (c *musicAssistantClient) call(command string, out any) error {
	body, err := json.Marshal(commandMessage{
		MessageID: c.nextID.Add(1),
		Command:   command,
		Args:      map[string]any{},
	})
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling music assistant: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("music assistant returned HTTP %d", resp.StatusCode)
	}

	var envelope resultEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if envelope.ErrorCode != "" {
		msg := envelope.Details
		if msg == "" {
			msg = envelope.ErrorCode
		}
		return fmt.Errorf("music assistant api error (%s): %s", envelope.ErrorCode, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return fmt.Errorf("decoding result: %w", err)
	}
	return nil
}

// getPlayers calls the "players/all" command.
func (c *musicAssistantClient) getPlayers() ([]playerState, error) {
	var players []playerState
	if err := c.call("players/all", &players); err != nil {
		return nil, err
	}
	return players, nil
}
