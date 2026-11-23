// Package client is a small HTTP client for the Artisan Virt control plane
// administrator API (/api/v1). Field names mirror the server's JSON exactly:
// the server decodes request bodies with DisallowUnknownFields.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one Artisan Virt control plane.
type Client struct {
	baseURL   *url.URL
	token     string
	http      *http.Client
	userAgent string
	// PollInterval is how often long-running operations are polled.
	PollInterval time.Duration
}

// Config configures a Client.
type Config struct {
	Endpoint           string
	Token              string
	InsecureSkipVerify bool
	CACertPEM          string
	RequestTimeout     time.Duration
	UserAgent          string
	PollInterval       time.Duration
}

// New builds a client. Endpoint is the control plane base URL, for example
// https://artisan.example.com (without /api/v1).
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("endpoint is required")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("token is required")
	}
	u, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid endpoint %q: expected a URL such as https://artisan.example.com", cfg.Endpoint)
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec // opt-in for lab installs
	if cfg.CACertPEM != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(cfg.CACertPEM)) {
			return nil, errors.New("ca_cert does not contain a valid PEM certificate")
		}
		tlsCfg.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 3 * time.Second
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = "terraform-provider-artisanvirt"
	}
	return &Client{
		baseURL:      u,
		token:        cfg.Token,
		http:         &http.Client{Timeout: timeout, Transport: transport},
		userAgent:    ua,
		PollInterval: poll,
	}, nil
}

// APIError is a non-2xx response. The body follows the server's Error schema.
type APIError struct {
	StatusCode int             `json:"-"`
	Code       string          `json:"code"`
	Message    string          `json:"message"`
	Details    json.RawMessage `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("artisan virt API: HTTP %d", e.StatusCode)
	if e.Code != "" {
		msg += " " + e.Code
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	if len(e.Details) > 0 && string(e.Details) != "null" {
		msg += "\ndetails: " + string(e.Details)
	}
	return msg
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusNotFound
}

// IsConflict reports whether err is a 409 from the API.
func IsConflict(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.StatusCode == http.StatusConflict
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := *c.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("%s %s: reading response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		ae := &APIError{StatusCode: resp.StatusCode}
		if json.Unmarshal(data, ae) != nil || (ae.Code == "" && ae.Message == "") {
			ae.Message = strings.TrimSpace(string(data))
		}
		ae.StatusCode = resp.StatusCode
		return ae
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
	}
	return nil
}

func esc(s string) string { return url.PathEscape(s) }

// Poll calls check every interval until it reports done, returns an error, or
// ctx expires.
func (c *Client) Poll(ctx context.Context, what string, check func(context.Context) (bool, error)) error {
	t := time.NewTicker(c.PollInterval)
	defer t.Stop()
	for {
		done, err := check(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// The deadline expired during the request itself.
				return fmt.Errorf("timed out waiting for %s: %w", what, ctx.Err())
			}
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s: %w", what, ctx.Err())
		case <-t.C:
		}
	}
}

// WaitTask waits for a queued task to finish and returns an error if it failed.
func (c *Client) WaitTask(ctx context.Context, id string) (*Task, error) {
	if id == "" {
		return nil, nil
	}
	var last *Task
	err := c.Poll(ctx, "task "+id, func(ctx context.Context) (bool, error) {
		t, err := c.GetTask(ctx, id)
		if err != nil {
			return false, err
		}
		last = t
		return t.Done(), nil
	})
	if err != nil {
		return last, err
	}
	if last.Status == TaskFailed {
		return last, fmt.Errorf("task %s (%s) failed: %s", last.ID, last.Type, last.Error)
	}
	return last, nil
}
