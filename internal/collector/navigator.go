// @guardian-project: guardian
// @guardian-path: internal/collector/navigator.go
// ADR-039 gap closure: Navigator topology calls now use Herald typed client.
// Replaces: raw http.NewRequestWithContext + local navigatorGraphResponse struct.
// Herald v0.1.6+ exposes c.Navigator().Graph(ctx) returning accord.NavigatorGraphDTO.
// No auth required on Navigator topology endpoints (ADR-012) — token passed but
// Navigator's middleware accepts it without requiring presence.
package collector

import (
	"context"
	"log"

	herald "github.com/Harshmaury/Herald/client"
	"github.com/Harshmaury/Guardian/internal/policy"
)

// NavigatorCollector polls Navigator GET /topology/graph via Herald.
type NavigatorCollector struct {
	navigator *herald.Client
	logger    *log.Logger
}

// NewNavigatorCollector creates a NavigatorCollector.
// serviceToken is passed for consistency — Navigator accepts but does not require it.
func NewNavigatorCollector(baseURL, serviceToken string, logger *log.Logger) *NavigatorCollector {
	return &NavigatorCollector{
		navigator: herald.NewForService(baseURL, serviceToken),
		logger:    logger,
	}
}

// Collect fetches topology nodes from Navigator.
// Returns nil and logs a WARNING if Navigator is unreachable.
func (c *NavigatorCollector) Collect(ctx context.Context, _ string) []policy.TopologyNode {
	graph, err := c.navigator.Navigator().Graph(ctx)
	if err != nil {
		c.logger.Printf("WARNING: navigator collector: %v", err)
		return nil
	}
	nodes := make([]policy.TopologyNode, 0, len(graph.Nodes))
	for _, n := range graph.Nodes {
		nodes = append(nodes, policy.TopologyNode{
			ID:     n.ID,
			Status: n.Status,
		})
	}
	return nodes
}
