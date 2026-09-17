package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

// defaultQueryLogsAppName is the store-listed name of the "Query Logs
// (Sqlite)" DNS app: https://github.com/TechnitiumSoftware/DnsServer/tree/master/Apps/QueryLogsSqliteApp.
// A server can install it under a different name, hence the override in
// PluginSettings, but this is what most installs use.
const defaultQueryLogsAppName = "Query Logs (Sqlite)"

// queryLogsClassPath is the app's fully qualified class name, fixed by its
// own source (namespace QueryLogsSqlite, class App) regardless of the name
// it was installed under.
const queryLogsClassPath = "QueryLogsSqlite.App"

// queryLogsResponse mirrors GET /api/logs/query's response, served by any
// installed DNS app that implements IDnsQueryLogs (Query Logs Sqlite,
// MySQL, PostgreSQL, SQL Server all share this same API shape).
type queryLogsResponse struct {
	Status       string          `json:"status"`
	ErrorMsg     string          `json:"errorMessage"`
	PageNumber   int64           `json:"pageNumber"`
	TotalPages   int64           `json:"totalPages"`
	TotalEntries int64           `json:"totalEntries"`
	Entries      []queryLogEntry `json:"entries"`
}

type queryLogEntry struct {
	RowNumber       int64    `json:"rowNumber"`
	Timestamp       string   `json:"timestamp"`
	ClientIPAddress string   `json:"clientIpAddress"`
	Protocol        string   `json:"protocol"`
	ResponseType    string   `json:"responseType"`
	ResponseRtt     *float64 `json:"responseRtt"`
	RCode           string   `json:"rcode"`
	QName           string   `json:"qname"`
	QType           string   `json:"qtype"`
	QClass          string   `json:"qclass"`
	Answer          string   `json:"answer"`
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

// queryLogsFilter is the optional subset of GET /api/logs/query's filter
// parameters this plugin exposes. Empty fields are left off the request.
type queryLogsFilter struct {
	appName         string
	start, end      time.Time
	clientIPAddress string
	qname           string
}

// getQueryLogsPage calls GET /api/logs/query for a single page of a DNS
// app's stored query log (e.g. "Query Logs (Sqlite)"). Entries come back
// oldest-first within the page when descending is false, which is what
// getQueryLogs relies on to build a chronological frame across pages.
func (c *technitiumClient) getQueryLogsPage(f queryLogsFilter, pageNumber int64, entriesPerPage int) (*queryLogsResponse, error) {
	appName := f.appName
	if appName == "" {
		appName = defaultQueryLogsAppName
	}

	q := url.Values{}
	q.Set("token", c.apiToken)
	q.Set("name", appName)
	q.Set("classPath", queryLogsClassPath)
	q.Set("pageNumber", strconv.FormatInt(pageNumber, 10))
	q.Set("entriesPerPage", strconv.Itoa(entriesPerPage))
	q.Set("descendingOrder", "false")
	if !f.start.IsZero() {
		q.Set("start", f.start.UTC().Format(time.RFC3339))
	}
	if !f.end.IsZero() {
		q.Set("end", f.end.UTC().Format(time.RFC3339))
	}
	if f.clientIPAddress != "" {
		q.Set("clientIpAddress", f.clientIPAddress)
	}
	if f.qname != "" {
		q.Set("qname", f.qname)
	}

	reqURL := fmt.Sprintf("%s/api/logs/query?%s", c.baseURL, q.Encode())
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

	var out queryLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if out.Status != "" && out.Status != "ok" {
		msg := out.ErrorMsg
		if msg == "" {
			msg = out.Status
		}
		return nil, fmt.Errorf("technitium api error: %s", msg)
	}

	return &out, nil
}

// maxQueryLogEntries caps how many rows getQueryLogs will pull across
// pages for one Grafana query, so a wide dashboard time range against a
// busy resolver cannot pull the whole log into one response.
const maxQueryLogEntries = 10000

// queryLogsPageSize is the page size requested from Technitium per HTTP
// call while paging toward maxQueryLogEntries.
const queryLogsPageSize = 1000

// getQueryLogs pages through GET /api/logs/query until it has covered the
// requested time range or hit maxQueryLogEntries, whichever comes first,
// and returns the entries in chronological order.
func (c *technitiumClient) getQueryLogs(f queryLogsFilter) ([]queryLogEntry, error) {
	var all []queryLogEntry

	for page := int64(1); ; page++ {
		out, err := c.getQueryLogsPage(f, page, queryLogsPageSize)
		if err != nil {
			return nil, err
		}

		all = append(all, out.Entries...)
		if len(all) >= maxQueryLogEntries || page >= out.TotalPages || len(out.Entries) == 0 {
			break
		}
	}

	if len(all) > maxQueryLogEntries {
		all = all[:maxQueryLogEntries]
	}

	return all, nil
}
