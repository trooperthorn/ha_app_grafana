package plugin

import "strings"

// normalizedClient is a connected client row, field candidates taken from
// unifi.py's _normalize_client in trooperthorn/ha_int_soc (see
// docs/UNIFI-LOCAL-API-CONTRACT.md there for which controller field names
// were actually verified against the Network 10.4.57 OpenAPI spec).
type normalizedClient struct {
	Name     string
	MAC      string
	IPv4     string
	VLAN     string
	SSID     string
	Wired    bool
	UptimeS  int64
	LastSeen int64
	RXBytes  int64
	TXBytes  int64
}

func normalizeClient(r row, nowUnix int64) normalizedClient {
	name := first(r, "name", "hostname", "displayName", "alias", "note")
	mac := strings.ToLower(first(r, "macAddress", "mac"))
	if name == "" {
		name = mac
	}
	if name == "" {
		name = "unknown"
	}

	vlan := first(r, "vlan", "vlanId", "networkVlanId", "vlan_id")
	if vlan == "" {
		if access, ok := r["access"].(map[string]any); ok {
			vlan = first(row(access), "vlanId", "vlan")
		}
	}

	ssid := first(r, "ssid", "essid", "wifiNetworkName", "networkName", "network_name")
	connType := strings.ToUpper(first(r, "type", "connectionType"))
	wiredFlag, _ := firstBool(r, "wired", "isWired")
	wired := connType == "WIRED" || (connType != "WIRELESS" && ssid == "" && wiredFlag)

	var uptime int64
	if v, ok := firstNumber(r, "uptime", "uptimeSeconds", "uptime_seconds"); ok {
		uptime = int64(v)
	} else if v, ok := firstNumber(r, "connectedAt", "connected_at", "associationTime", "assocTime"); ok {
		connected := asEpoch(v)
		if nowUnix >= connected {
			uptime = nowUnix - connected
		}
	}

	var lastSeen int64
	if v, ok := firstNumber(r, "lastSeen", "last_seen", "lastConnectionAt", "connectedAt", "connected_at"); ok {
		lastSeen = asEpoch(v)
	}

	rx, tx := bandwidthOf(r)

	return normalizedClient{
		Name:     name,
		MAC:      mac,
		IPv4:     first(r, "ipAddress", "ip", "ipv4", "lastIp", "last_ip", "fixed_ip"),
		VLAN:     vlan,
		SSID:     ssid,
		Wired:    wired,
		UptimeS:  uptime,
		LastSeen: lastSeen,
		RXBytes:  rx,
		TXBytes:  tx,
	}
}

// normalizedDevice is a network infrastructure device (gateway, switch, AP).
type normalizedDevice struct {
	Name                   string
	MAC                    string
	IPv4                   string
	Model                  string
	State                  string
	FirmwareUpdatable      bool
	FirmwareUpdatableKnown bool
	LastSeen               int64
	RXBytes                int64
	TXBytes                int64
}

func normalizeDevice(r row) normalizedDevice {
	name := first(r, "name", "hostname", "displayName", "model")
	mac := strings.ToLower(first(r, "macAddress", "mac"))
	if name == "" {
		name = mac
	}
	if name == "" {
		name = "unknown"
	}

	fwUpdatable, fwKnown := firstBool(r, "firmwareUpdatable", "updateAvailable", "update_available")
	if !fwKnown {
		if fw, ok := r["firmware"].(map[string]any); ok {
			fwUpdatable, fwKnown = firstBool(row(fw), "updatable", "updateAvailable")
		}
	}

	var lastSeen int64
	if v, ok := firstNumber(r, "lastSeen", "last_seen", "lastHeartbeatAt", "startupTimestamp"); ok {
		lastSeen = asEpoch(v)
	} else if stats, ok := r["statistics"].(map[string]any); ok {
		if v, ok := firstNumber(row(stats), "lastHeartbeatAt", "lastSeen"); ok {
			lastSeen = asEpoch(v)
		}
	}

	rx, tx := bandwidthOf(r)

	return normalizedDevice{
		Name:                   name,
		MAC:                    mac,
		IPv4:                   first(r, "ipAddress", "ip", "ipv4", "lastIp"),
		Model:                  first(r, "model", "modelName", "shortname"),
		State:                  strings.ToUpper(first(r, "state", "status")),
		FirmwareUpdatable:      fwUpdatable,
		FirmwareUpdatableKnown: fwKnown,
		LastSeen:               lastSeen,
		RXBytes:                rx,
		TXBytes:                tx,
	}
}

// bandwidthOf finds cumulative rx/tx byte counters at the top level or
// under the "statistics"/"stats"/"uplink" containers, the same search
// unifi.py's _bandwidth_of does.
func bandwidthOf(r row) (rx, tx int64) {
	containers := []row{r}
	for _, key := range []string{"statistics", "stats", "uplink"} {
		if node, ok := r[key].(map[string]any); ok {
			containers = append(containers, row(node))
			if nested, ok := node["uplink"].(map[string]any); ok {
				containers = append(containers, row(nested))
			}
		}
	}
	for _, c := range containers {
		rxV, rxOK := firstNumber(c, "rxBytes", "rx_bytes", "wired-rx_bytes", "rx")
		txV, txOK := firstNumber(c, "txBytes", "tx_bytes", "wired-tx_bytes", "tx")
		if !rxOK && !txOK {
			continue
		}
		return int64(rxV), int64(txV)
	}
	return 0, 0
}

var gatewayTokens = []string{"gateway", "udm", "uxg", "usg", "ugw", "ucg", "udr", "uxr", "dream", "console"}

// isGateway mirrors unifi.py's _is_gateway: a declared role wins; a name
// token match is a fallback for a device that never declares one.
func isGateway(r row) bool {
	role := strings.ToLower(first(r, "type", "deviceType", "role"))
	if role == "gateway" || role == "console" || role == "ugw" {
		return true
	}
	blob := strings.ToLower(first(r, "type") + " " + first(r, "model") + " " + first(r, "shortname") + " " +
		first(r, "name") + " " + first(r, "deviceType") + " " + first(r, "role"))
	for _, tok := range gatewayTokens {
		if strings.Contains(blob, tok) {
			return true
		}
	}
	return false
}

// selectGateway mirrors unifi.py's _select_gateway.
func selectGateway(devices []row) row {
	for _, d := range devices {
		role := strings.ToLower(first(d, "type", "deviceType", "role"))
		if role == "gateway" || role == "console" || role == "ugw" {
			return d
		}
	}
	for _, d := range devices {
		if isGateway(d) {
			return d
		}
	}
	return nil
}

type wanStatus struct {
	Port  string
	Up    bool
	UpOK  bool
	RXBps int64
	TXBps int64
	IP    string
}

// derivedWAN is a trimmed form of unifi.py's _derive_wan: it checks the
// gateway's own uplink/wan/wan1/wan2/internet objects (top level and under
// "statistics"), not the fuller interfaces/ports-array search the reference
// client also does, since those shapes remain on ha_int_soc's own
// unverified backlog.
func derivedWAN(devices []row) wanStatus {
	var status wanStatus
	gateway := selectGateway(devices)
	if gateway == nil {
		return status
	}

	var nodes []row
	addNamed := func(container row) {
		for _, key := range []string{"uplink", "wan1", "wan", "internet", "wan2"} {
			if node, ok := container[key].(map[string]any); ok {
				nodes = append(nodes, row(node))
			}
		}
	}
	addNamed(gateway)
	if stats, ok := gateway["statistics"].(map[string]any); ok {
		addNamed(row(stats))
		nodes = append(nodes, row(stats))
	}

	for _, node := range nodes {
		rx, rxOK := firstNumber(node, "rxRateBps", "rx_bytes-r", "rx_rate", "rxRate", "rxBps", "download")
		tx, txOK := firstNumber(node, "txRateBps", "tx_bytes-r", "tx_rate", "txRate", "txBps", "upload")
		up, upOK := firstBool(node, "up", "enable", "enabled", "isUp", "plugged", "connected")
		ip := first(node, "ip", "ipAddress", "wan_ip", "wanIp")
		name := first(node, "name", "ifname")
		if !rxOK && !txOK && !upOK && ip == "" {
			continue
		}
		if name != "" {
			status.Port = name
		}
		if upOK {
			status.Up, status.UpOK = up, true
		}
		if rxOK {
			status.RXBps = int64(rx)
		}
		if txOK {
			status.TXBps = int64(tx)
		}
		if ip != "" {
			status.IP = ip
		}
		break
	}
	return status
}
