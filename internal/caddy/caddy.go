// Package caddy provides a client for the Caddy Admin API (localhost:2019).
package caddy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the Caddy Admin API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Client pointed at adminAPI (e.g. "http://localhost:2019").
func NewClient(adminAPI string) *Client {
	return &Client{
		baseURL: adminAPI,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Ping verifies the Caddy Admin API is reachable.
func (c *Client) Ping() error {
	resp, err := c.httpClient.Get(c.baseURL + "/config/")
	if err != nil {
		return fmt.Errorf("caddy unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("caddy returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Route describes a reverse-proxy route to register in Caddy.
type Route struct {
	AppName      string // used as the @id: "davit_<name>"
	Domain       string
	UpstreamAddr string // "127.0.0.1:42001"
}

// CertInfo summarises a TLS certificate managed by Caddy.
type CertInfo struct {
	Domain      string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	NotAfter    time.Time `json:"not_after"`
	AutoManaged bool      `json:"managed"`
}

// AddRoute registers (or replaces) a reverse-proxy route for an application.
// It is idempotent: a second call for the same app replaces the existing route.
func (c *Client) AddRoute(r Route) error {
	id := "davit_" + r.AppName

	// First ensure the HTTP server and routes array exist.
	if err := c.ensureHTTPServer(); err != nil {
		return fmt.Errorf("ensure http server: %w", err)
	}

	// Remove any existing route with this ID (idempotent).
	_ = c.RemoveRoute(r.AppName)

	route := map[string]any{
		"@id":      id,
		"match":    []map[string]any{{"host": []string{r.Domain}}},
		"handle":   []map[string]any{{"handler": "reverse_proxy", "upstreams": []map[string]any{{"dial": r.UpstreamAddr}}}},
		"terminal": true,
	}

	body, err := json.Marshal(route)
	if err != nil {
		return err
	}

	// Append to routes array
	resp, err := c.do("POST",
		"/config/apps/http/servers/srv0/routes",
		body,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy rejected route (HTTP %d): %s", resp.StatusCode, b)
	}
	return nil
}

// RemoveRoute removes the route with @id "davit_<appName>" from Caddy.
func (c *Client) RemoveRoute(appName string) error {
	id := "davit_" + appName
	resp, err := c.do("DELETE", "/id/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 404 means already gone — treat as success.
	if resp.StatusCode >= 300 && resp.StatusCode != 404 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy remove route (HTTP %d): %s", resp.StatusCode, b)
	}
	return nil
}

// RouteExists returns true if a route with @id "davit_<appName>" exists in Caddy.
func (c *Client) RouteExists(appName string) bool {
	id := "davit_" + appName
	resp, err := c.do("GET", "/id/"+id, nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// GetCerts returns the list of TLS certificates Caddy is managing.
func (c *Client) GetCerts() ([]CertInfo, error) {
	resp, err := c.do("GET", "/config/apps/tls/certificates/automate", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, nil
	}
	var certs []CertInfo
	if err := json.NewDecoder(resp.Body).Decode(&certs); err != nil {
		return nil, err
	}
	return certs, nil
}

// ensureHTTPServer bootstraps the minimal Caddy config if the HTTP server
// section doesn't exist yet (fresh Caddy install).
func (c *Client) ensureHTTPServer() error {
	// Check if the server block exists
	resp, err := c.do("GET", "/config/apps/http/servers/srv0", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return nil // already exists
	}

	// Create the minimal HTTP server config
	serverConfig := map[string]any{
		"listen": []string{":80", ":443"},
		"routes": []any{},
	}
	body, err := json.Marshal(serverConfig)
	if err != nil {
		return err
	}

	resp2, err := c.do("PUT", "/config/apps/http/servers/srv0", body)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode >= 300 {
		b, _ := io.ReadAll(resp2.Body)
		return fmt.Errorf("init caddy http server (HTTP %d): %s", resp2.StatusCode, b)
	}
	return nil
}

func (c *Client) do(method, path string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}
