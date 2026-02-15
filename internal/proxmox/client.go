package proxmox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ClientConfig holds the parameters for creating a new Client.
type ClientConfig struct {
	BaseURL       string
	Node          string
	TokenID       string
	TokenSecret   string
	TLSSkipVerify bool
	TLSCACertPath string
}

// Client is an HTTP client for the Proxmox REST API.
type Client struct {
	baseURL     string
	node        string
	tokenID     string
	tokenSecret string
	httpClient  *http.Client
}

// NewClient creates a new Proxmox API client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("proxmox base URL is required")
	}
	if cfg.Node == "" {
		return nil, fmt.Errorf("proxmox node name is required")
	}

	tlsCfg := &tls.Config{}

	if cfg.TLSCACertPath != "" {
		caCert, err := os.ReadFile(cfg.TLSCACertPath)
		if err != nil {
			return nil, fmt.Errorf("reading CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert from %s", cfg.TLSCACertPath)
		}
		tlsCfg.RootCAs = pool
	} else if cfg.TLSSkipVerify {
		tlsCfg.InsecureSkipVerify = true
	}

	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		node:        cfg.Node,
		tokenID:     cfg.TokenID,
		tokenSecret: cfg.TokenSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsCfg,
			},
		},
	}, nil
}

// StorageInfo holds configuration details for a Proxmox storage from GET /storage/{id}.
type StorageInfo struct {
	ID         string `json:"storage"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	Path       string `json:"path,omitempty"`       // dir, nfs, cifs
	Mountpoint string `json:"mountpoint,omitempty"` // zfspool
	Pool       string `json:"pool,omitempty"`       // zfspool
}

// ListStorages returns all storages from the Proxmox cluster.
func (c *Client) ListStorages(ctx context.Context) ([]StorageInfo, error) {
	var storages []StorageInfo
	if err := c.doRequest(ctx, "GET", "/storage", nil, &storages); err != nil {
		return nil, fmt.Errorf("listing storages: %w", err)
	}
	return storages, nil
}

// GetStorageInfo returns the configuration for a single Proxmox storage.
func (c *Client) GetStorageInfo(ctx context.Context, storage string) (*StorageInfo, error) {
	// Use the list endpoint (works with broader permissions) and filter
	storages, err := c.ListStorages(ctx)
	if err != nil {
		return nil, err
	}
	for _, si := range storages {
		if si.ID == storage {
			return &si, nil
		}
	}
	return nil, fmt.Errorf("storage %q not found", storage)
}

// NodeStorageStatus holds per-node storage status from GET /nodes/{node}/storage.
// Unlike StorageInfo (from GET /storage), this includes capacity/usage data.
type NodeStorageStatus struct {
	ID      string `json:"storage"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Active  int    `json:"active"`
	Enabled int    `json:"enabled"`
	Total   int64  `json:"total"` // bytes
	Used    int64  `json:"used"`  // bytes
	Avail   int64  `json:"avail"` // bytes
}

// ListNodeStorages returns all storages for this node with capacity data.
func (c *Client) ListNodeStorages(ctx context.Context) ([]NodeStorageStatus, error) {
	var storages []NodeStorageStatus
	if err := c.doRequest(ctx, "GET", "/nodes/"+c.node+"/storage", nil, &storages); err != nil {
		return nil, fmt.Errorf("listing node storages: %w", err)
	}
	return storages, nil
}

// NodeNetworkIface holds network interface info from GET /nodes/{node}/network.
type NodeNetworkIface struct {
	Iface          string `json:"iface"`
	Type           string `json:"type"`
	Address        string `json:"address"`
	Netmask        string `json:"netmask"`
	CIDR           string `json:"cidr"`
	Gateway        string `json:"gateway"`
	BridgePorts    string `json:"bridge_ports"`
	Active         int    `json:"active"`
	Comments       string `json:"comments"`
	BridgeVLANAware int   `json:"bridge_vlan_aware"`
	BridgeVIDs     string `json:"bridge_vids"`
}

// ListNodeNetworks returns all network interfaces for this node.
func (c *Client) ListNodeNetworks(ctx context.Context) ([]NodeNetworkIface, error) {
	var ifaces []NodeNetworkIface
	if err := c.doRequest(ctx, "GET", "/nodes/"+c.node+"/network", nil, &ifaces); err != nil {
		return nil, fmt.Errorf("listing node networks: %w", err)
	}
	return ifaces, nil
}

// apiResponse wraps the standard Proxmox {"data": ...} envelope.
type apiResponse struct {
	Data   json.RawMessage        `json:"data"`
	Errors map[string]string      `json:"errors,omitempty"`
}

// doRequest performs an HTTP request against the Proxmox API.
// For GET requests, params are added as query string.
// For POST/PUT/DELETE, params are form-encoded in the body.
func (c *Client) doRequest(ctx context.Context, method, path string, params url.Values, result interface{}) error {
	reqURL := c.baseURL + "/api2/json" + path

	var body io.Reader
	if method == http.MethodGet {
		if len(params) > 0 {
			reqURL += "?" + params.Encode()
		}
	} else {
		if params != nil {
			body = strings.NewReader(params.Encode())
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenID, c.tokenSecret))
	if method != http.MethodGet && body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &ProxmoxError{StatusCode: resp.StatusCode}
		var envelope apiResponse
		if json.Unmarshal(respBody, &envelope) == nil {
			apiErr.Errors = envelope.Errors
			// Try to extract a message from data
			if len(envelope.Data) > 0 {
				var msg string
				if json.Unmarshal(envelope.Data, &msg) == nil {
					apiErr.Message = msg
				}
			}
		}
		if apiErr.Message == "" {
			apiErr.Message = string(respBody)
		}
		return apiErr
	}

	if result != nil {
		var envelope apiResponse
		if err := json.Unmarshal(respBody, &envelope); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return fmt.Errorf("decoding data: %w", err)
		}
	}

	return nil
}
