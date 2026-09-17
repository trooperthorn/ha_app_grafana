package plugin

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// basePath is the SWIS REST/JSON contract root, verified against SolarWinds Platform
// 2026.2. See docs/swis/rest-api.md in OrionGuides.
const basePath = "/SolarWinds/InformationService/v3/Json"

// SwisClient speaks the small REST contract that every SolarWinds client wraps: POST
// /Query with a bound-parameter body, and POST /Invoke/{Entity}/{Verb} with a positional
// argument array.
type SwisClient struct {
	base     string
	username string
	password string
	http     *http.Client
}

// SwisError carries the message SWIS returns in its JSON error envelope, which is the
// text a user needs to see (a bad column name, a missing right, a malformed verb call).
type SwisError struct {
	Status  int
	Message string
}

func (e *SwisError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("SWIS returned HTTP %d: %s", e.Status, e.Message)
}

// NewSwisClient builds a client from the data source settings. TLS verification stays on
// unless the administrator turned it off; the right fix for the self-signed certificate
// SWIS ships with is to paste that certificate into the CA field, and this honours it.
func NewSwisClient(s *Settings) (*SwisClient, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	switch {
	case s.TLSSkipVerify:
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // an explicit, logged, per-data-source choice
	case strings.TrimSpace(s.CACert) != "":
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(s.CACert)) {
			return nil, fmt.Errorf("the CA certificate field does not contain a PEM certificate")
		}
		// Verification is done by hand so the name check can be dropped on its own. The
		// chain must still lead to the pasted certificate, so a different certificate on
		// the same host, which is what an interception looks like, is still refused.
		// Go's default verifier is bypassed (InsecureSkipVerify) only so that this one
		// runs instead; it is not skipped.
		host := s.Host
		ignoreName := s.TLSIgnoreHostname
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // replaced by VerifyPeerCertificate below
		tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("SWIS presented no certificate")
			}
			leaf, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("could not parse the SWIS certificate: %w", err)
			}
			intermediates := x509.NewCertPool()
			for _, raw := range rawCerts[1:] {
				if c, err := x509.ParseCertificate(raw); err == nil {
					intermediates.AddCert(c)
				}
			}
			opts := x509.VerifyOptions{Roots: pool, Intermediates: intermediates}
			if !ignoreName {
				opts.DNSName = host
			}
			if _, err := leaf.Verify(opts); err != nil {
				if ignoreName {
					return fmt.Errorf("the SWIS certificate is not the pinned one: %w", err)
				}
				return fmt.Errorf("%w (if the chain is right and only the name differs, which is the case with the stock SWIS certificate, turn on 'Ignore certificate name')", err)
			}
			return nil
		}
	}
	transport := &http.Transport{
		TLSClientConfig:     tlsCfg,
		MaxIdleConns:        4,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	return &SwisClient{
		base:     fmt.Sprintf("https://%s:%d%s", s.Host, s.Port, basePath),
		username: s.Username,
		password: s.Password,
		http:     &http.Client{Transport: transport, Timeout: time.Duration(s.TimeoutSecs) * time.Second},
	}, nil
}

// Close releases idle connections when Grafana disposes the instance.
func (c *SwisClient) Close() {
	if t, ok := c.http.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
}

func (c *SwisClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/"+strings.TrimPrefix(path, "/"), payload)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		var uerr *url.Error
		if ok := asURLError(err, &uerr); ok && uerr.Timeout() {
			return nil, &SwisError{Message: "the request to SWIS timed out; raise the timeout or narrow the query"}
		}
		return nil, &SwisError{Message: fmt.Sprintf("could not reach SWIS at %s: %v (check the host, that port is open, and that it is 17774 rather than the deprecated 17778)", c.base, err)}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(raw))
		var envelope struct {
			Message string `json:"Message"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Message != "" {
			msg = envelope.Message
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			msg = "SWIS rejected the credentials, or the account lacks the right this call needs (" + msg + ")"
		}
		return nil, &SwisError{Status: resp.StatusCode, Message: msg}
	}
	return raw, nil
}

func asURLError(err error, target **url.Error) bool {
	e, ok := err.(*url.Error)
	if ok {
		*target = e
	}
	return ok
}

// Query runs SWQL with bound parameters and returns the raw JSON results array, untouched,
// so the caller can recover column order (a Go map would lose it).
func (c *SwisClient) Query(ctx context.Context, swql string, parameters map[string]any) (json.RawMessage, error) {
	body := map[string]any{"query": swql}
	if len(parameters) > 0 {
		body["parameters"] = parameters
	}
	raw, err := c.do(ctx, http.MethodPost, "Query", body)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("SWIS returned a response that is not the expected JSON envelope: %w", err)
	}
	if len(envelope.Results) == 0 {
		return json.RawMessage("[]"), nil
	}
	return envelope.Results, nil
}

// Invoke calls a verb. Arguments are positional: the array order is the whole contract.
func (c *SwisClient) Invoke(ctx context.Context, entity, verb string, args []any) (json.RawMessage, error) {
	if args == nil {
		args = []any{}
	}
	raw, err := c.do(ctx, http.MethodPost, "Invoke/"+url.PathEscape(entity)+"/"+url.PathEscape(verb), args)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage("null"), nil
	}
	return json.RawMessage(raw), nil
}
