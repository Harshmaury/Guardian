// @guardian-project: guardian
// @guardian-path: internal/collector/navigator.go
// Phase 2: injected *log.Logger, WARNING on upstream failure (audit #4).
package collector

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/Harshmaury/Canon/identity"
	"github.com/Harshmaury/Guardian/internal/policy"
)

// NavigatorCollector polls Navigator GET /topology/graph.
type NavigatorCollector struct {
	baseURL    string
	httpClient *http.Client
	logger     *log.Logger
}

// NewNavigatorCollector creates a NavigatorCollector.
// Navigator requires no auth on topology endpoints (ADR-012).
func NewNavigatorCollector(baseURL string, logger *log.Logger) *NavigatorCollector {
	return &NavigatorCollector{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// Collect fetches topology nodes from Navigator.
// traceID propagates the cycle trace ID via X-Trace-ID (ADR-015).
// Returns nil and logs a WARNING if Navigator is unreachable (audit #4).
func (c *NavigatorCollector) Collect(ctx context.Context, traceID string) []policy.TopologyNode {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/topology/graph", nil)
	if err != nil {
		c.logger.Printf("WARNING: navigator collector: build request: %v", err)
		return nil
	}
	if traceID != "" {
		req.Header.Set(identity.TraceIDHeader, traceID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Printf("WARNING: navigator collector unreachable: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.logger.Printf("WARNING: navigator collector: HTTP %d from %s/topology/graph",
			resp.StatusCode, c.baseURL)
		return nil
	}

	return decodeNavigatorResponse(resp)
}

// decodeNavigatorResponse parses the Navigator topology response.
func decodeNavigatorResponse(resp *http.Response) []policy.TopologyNode {
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
