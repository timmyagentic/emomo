package configcenter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxErrorBodyBytes = 4096

// Client fetches runtime configuration from the config center endpoint.
type Client struct {
	url        string
	token      string
	httpClient *http.Client
}

// ClientConfig configures a config center client.
type ClientConfig struct {
	URL     string
	Token   string
	Timeout time.Duration
}

// RemoteConfig is the JSON document returned by the config center.
type RemoteConfig struct {
	Version        string                     `json:"version,omitempty"`
	UpdatedAt      string                     `json:"updated_at,omitempty"`
	Config         map[string]any             `json:"config,omitempty"`
	QueryExpansion RemoteQueryExpansionConfig `json:"query_expansion,omitempty"`
}

// RemoteQueryExpansionConfig contains optional runtime overrides.
type RemoteQueryExpansionConfig struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Model   *string `json:"model,omitempty"`
	APIKey  *string `json:"api_key,omitempty"`
	BaseURL *string `json:"base_url,omitempty"`
}

// IsZero reports whether no query expansion override fields were provided.
func (c RemoteQueryExpansionConfig) IsZero() bool {
	return c.Enabled == nil && c.Model == nil && c.APIKey == nil && c.BaseURL == nil
}

// RuntimeConfig returns the full application config override map. It also maps
// the legacy top-level query_expansion shape into search.query_expansion.
func (c *RemoteConfig) RuntimeConfig() map[string]any {
	if c == nil {
		return nil
	}

	runtimeConfig := deepCopyMap(c.Config)
	if runtimeConfig == nil {
		runtimeConfig = make(map[string]any)
	}

	if !c.QueryExpansion.IsZero() {
		search, _ := runtimeConfig["search"].(map[string]any)
		if search == nil {
			search = make(map[string]any)
			runtimeConfig["search"] = search
		}
		qe, _ := search["query_expansion"].(map[string]any)
		if qe == nil {
			qe = make(map[string]any)
			search["query_expansion"] = qe
		}
		if c.QueryExpansion.Enabled != nil {
			qe["enabled"] = *c.QueryExpansion.Enabled
		}
		if c.QueryExpansion.Model != nil {
			qe["model"] = *c.QueryExpansion.Model
		}
		if c.QueryExpansion.APIKey != nil {
			qe["api_key"] = *c.QueryExpansion.APIKey
		}
		if c.QueryExpansion.BaseURL != nil {
			qe["base_url"] = *c.QueryExpansion.BaseURL
		}
	}

	if len(runtimeConfig) == 0 {
		return nil
	}
	return runtimeConfig
}

func deepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}

	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = deepCopyValue(value)
	}
	return dst
}

func deepCopyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCopyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = deepCopyValue(item)
		}
		return out
	default:
		return typed
	}
}

// NewClient creates a config center client.
func NewClient(cfg ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &Client{
		url:   strings.TrimSpace(cfg.URL),
		token: strings.TrimSpace(cfg.Token),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Fetch retrieves and decodes the current remote runtime configuration.
func (c *Client) Fetch(ctx context.Context) (*RemoteConfig, error) {
	if c.url == "" {
		return nil, fmt.Errorf("config center URL is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create config center request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch config center: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("config center returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var remote RemoteConfig
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&remote); err != nil {
		return nil, fmt.Errorf("decode config center response: %w", err)
	}

	return &remote, nil
}
