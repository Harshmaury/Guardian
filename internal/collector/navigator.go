// @guardian-project: guardian
// @guardian-path: internal/collector/navigator.go
package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Harshmaury/Guardian/internal/policy"
)

// NavigatorCollector polls Navigator GET /topology/graph.
type NavigatorCollector struct {
	baseURL    string
	httpClient *http.Client
}

// NewNavigatorCollector creates a NavigatorCollector.
// Navigator requires no auth on topology endpoints (ADR-012).
func NewNavigatorCollector(baseURL string) *NavigatorCollector {
	return &NavigatorCollector{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Collect fetches topology nodes from Navigator.
func (c *NavigatorCollector) Collect(ctx context.Context) []policy.TopologyNode {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/topology/graph", nil)
	if err != nil {
		return nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Nodes []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil
	}

	nodes := make([]policy.TopologyNode, 0, len(envelope.Data.Nodes))
	for _, n := range envelope.Data.Nodes {
		nodes = append(nodes, policy.TopologyNode{
			ID:     n.ID,
			Status: n.Status,
		})
	}
	return nodes
}
