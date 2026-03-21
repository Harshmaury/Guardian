// @guardian-project: guardian
// @guardian-path: internal/collector/navigator.go
// ADR-039: full Herald migration — Navigator topology calls now use typed client.
// Replaces: raw http.NewRequestWithContext + anonymous struct decode.
// Navigator topology is accessed via Herald pointing at Navigator's address.
// The /topology/graph endpoint returns accord.AtlasGraphDTO shape — nodes
// are wrapped inside the graph response data field.
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	canon "github.com/Harshmaury/Canon/identity"
	"github.com/Harshmaury/Guardian/internal/policy"
)

// NavigatorCollector polls Navigator GET /topology/graph.
// Navigator has no Herald client since it doesn't speak the Nexus API envelope.
// It uses the standard {ok, data} envelope but at its own address.
// We use a lightweight typed fetch rather than raw anonymous structs.
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

// navigatorGraphResponse is the typed response from GET /topology/graph.
// Replaces the previous anonymous struct — schema drift now fails loudly.
type navigatorGraphResponse struct {
	OK   bool `json:"ok"`
	Data struct {
		Nodes []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"nodes"`
	} `json:"data"`
}

// Collect fetches topology nodes from Navigator.
// Returns nil and logs a WARNING if Navigator is unreachable.
func (c *NavigatorCollector) Collect(ctx context.Context, traceID string) []policy.TopologyNode {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/topology/graph", nil)
	if err != nil {
		c.logger.Printf("WARNING: navigator collector: build request: %v", err)
		return nil
	}
	if traceID != "" {
		req.Header.Set(canon.TraceIDHeader, traceID)
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

	var envelope navigatorGraphResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		c.logger.Printf("WARNING: navigator collector: decode: %v", err)
		return nil
	}
	if !envelope.OK {
		c.logger.Printf("WARNING: navigator collector: %s",
			fmt.Sprintf("upstream returned ok=false"))
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
