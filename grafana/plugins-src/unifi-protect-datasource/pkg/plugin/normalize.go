package plugin

import "strings"

// normalizedCamera is one Protect camera row, field candidates and the
// deep-link shape taken from unifi.py's _normalize_camera in
// trooperthorn/ha_int_soc (see docs/UNIFI-LOCAL-API-CONTRACT.md there for
// which fields were actually verified against the Protect 7.2.105 OpenAPI
// spec).
type normalizedCamera struct {
	ID            string
	Name          string
	IP            string
	MAC           string
	IsRecording   bool
	IsRecordingOK bool
	LastRing      int64
	ChannelCount  int64
	State         string
	Online        bool
	OnlineOK      bool
	Link          string
}

func normalizeCamera(r row, origin string) normalizedCamera {
	id := first(r, "id", "_id", "deviceId")
	name := first(r, "name", "displayName", "modelKey", "model")
	mac := strings.ToLower(first(r, "mac", "macAddress"))
	if name == "" {
		name = mac
	}
	if name == "" {
		name = "unknown"
	}

	// A boolean on some Protect firmwares, recordingSettings.mode on others.
	var isRecording, isRecordingOK bool
	if v, ok := firstBool(r, "isRecording", "recording"); ok {
		isRecording, isRecordingOK = v, true
	} else if rs, ok := r["recordingSettings"].(map[string]any); ok {
		mode := strings.ToLower(first(row(rs), "mode"))
		if mode != "" {
			isRecordingOK = true
			isRecording = mode != "never" && mode != "off" && mode != "disabled"
		}
	}

	var lastRing int64
	if v, ok := firstNumber(r, "lastRing", "last_ring"); ok {
		lastRing = asEpoch(v)
	}

	var channelCount int64
	if channels, ok := r["channels"].([]any); ok {
		channelCount = int64(len(channels))
	}

	state := strings.ToUpper(first(r, "state", "status"))
	var online, onlineOK bool
	if state != "" {
		online, onlineOK = isOnlineState(state), true
	} else if v, ok := firstBool(r, "isConnected", "connected"); ok {
		online, onlineOK = v, true
	}

	link := ""
	if id != "" {
		link = origin + "/protect/dashboard/devices/" + id
	}

	return normalizedCamera{
		ID:            id,
		Name:          name,
		IP:            first(r, "host", "ip", "ipAddress", "lastSeenIp", "address"),
		MAC:           mac,
		IsRecording:   isRecording,
		IsRecordingOK: isRecordingOK,
		LastRing:      lastRing,
		ChannelCount:  channelCount,
		State:         state,
		Online:        online,
		OnlineOK:      onlineOK,
		Link:          link,
	}
}

func isOnlineState(state string) bool {
	switch state {
	case "CONNECTED", "ONLINE", "TRUE", "1":
		return true
	default:
		return false
	}
}
