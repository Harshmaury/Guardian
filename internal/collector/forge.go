// @guardian-project: guardian
// @guardian-path: internal/collector/forge.go
// ADR-039: full Herald migration — Forge history calls now use typed client.
// Replaces: raw http.NewRequestWithContext + anonymous struct decode.
package collector

import (
	"context"
	"log"
	"time"

	herald "github.com/Harshmaury/Herald/client"
	"github.com/Harshmaury/Guardian/internal/policy"
)

// ForgeCollector polls Forge GET /history via Herald.
type ForgeCollector struct {
	forge  *herald.Client
	logger *log.Logger
}

// NewForgeCollector creates a ForgeCollector.
func NewForgeCollector(baseURL, serviceToken string, logger *log.Logger) *ForgeCollector {
	return &ForgeCollector{
		forge:  herald.NewForService(baseURL, serviceToken),
		logger: logger,
	}
}

// Collect fetches recent execution records from Forge.
// Returns nil and logs a WARNING if Forge is unreachable.
func (c *ForgeCollector) Collect(ctx context.Context, traceID string) []policy.ExecutionRecord {
	records, err := c.forge.Forge().History(ctx, 100)
	if err != nil {
		c.logger.Printf("WARNING: forge collector: %v", err)
		return nil
	}

	out := make([]policy.ExecutionRecord, 0, len(records))
	for _, r := range records {
		ts := r.StartedAt
		if ts.IsZero() {
			ts = time.Time{}
		}
		out = append(out, policy.ExecutionRecord{
			Target:    r.Target,
			Status:    r.Status,
			ActorSub:  r.ActorSub,
			StartedAt: ts,
		})
	}
	return out
}
