package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// dashboardStatsResponse mirrors the fields of Technitium DNS Server's
// /api/dashboard/stats/get response that this plugin turns into frames.
// See https://github.com/TechnitiumSoftware/DnsServer/blob/master/APIDOCS.md.
type dashboardStatsResponse struct {
	Status   string `json:"status"`
	ErrorMsg string `json:"errorMessage"`
	Response struct {
		Stats struct {
			TotalQueries       float64 `json:"totalQueries"`
			TotalNoError       float64 `json:"totalNoError"`
			TotalServerFailure float64 `json:"totalServerFailure"`
			TotalNxDomain      float64 `json:"totalNxDomain"`
			TotalRefused       float64 `json:"totalRefused"`
			TotalAuthoritative float64 `json:"totalAuthoritative"`
			TotalRecursive     float64 `json:"totalRecursive"`
			TotalCached        float64 `json:"totalCached"`
			TotalBlocked       float64 `json:"totalBlocked"`
			TotalDropped       float64 `json:"totalDropped"`
		} `json:"stats"`
		MainChartData struct {
			LabelFormat string   `json:"labelFormat"`
			Labels      []string `json:"labels"`
			Datasets    []struct {
				Label string    `json:"label"`
				Data  []float64 `json:"data"`
			} `json:"datasets"`
		} `json:"mainChartData"`
		TopClients        []topRow `json:"topClients"`
		TopDomains        []topRow `json:"topDomains"`
		TopBlockedDomains []topRow `json:"topBlockedDomains"`
	} `json:"response"`
}

// topRow is the common name/hits shape of Technitium's three "top N"
// dashboard lists. topClients also carries domain/rateLimited fields this
// plugin does not surface.
type topRow struct {
	Name string  `json:"name"`
	Hits float64 `json:"hits"`
}

// technitiumClient talks to a Technitium DNS Server's HTTP API using a
// long-lived API token (Administration > Sessions > Create API Token),
// never the short-lived login session the web console itself uses.
type technitiumClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func newTechnitiumClient(baseURL, apiToken string) *technitiumClient {
	return &technitiumClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// getDashboardStats calls GET /api/dashboard/stats/get?type=<statType>&token=<token>.
// statType is one of LastHour, LastDay, LastWeek, LastMonth, LastYear, as the
// API documents; this plugin does not support its Custom start/end variant.
func (c *technitiumClient) getDashboardStats(statType string) (*dashboardStatsResponse, error) {
	q := url.Values{}
	q.Set("type", statType)
	q.Set("token", c.apiToken)

	reqURL := fmt.Sprintf("%s/api/dashboard/stats/get?%s", c.baseURL, q.Encode())
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling technitium: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("technitium returned HTTP %d", resp.StatusCode)
	}

	var out dashboardStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if out.Status != "ok" {
		msg := out.ErrorMsg
		if msg == "" {
			msg = out.Status
		}
		return nil, fmt.Errorf("technitium api error: %s", msg)
	}

	return &out, nil
}
