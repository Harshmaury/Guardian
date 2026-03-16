// @guardian-project: guardian
// @guardian-path: internal/collector/nexus.go
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Harshmaury/Guardian/internal/policy"
)

// NexusCollector polls Nexus GET /events for Guardian rule evaluation.
type NexusCollector struct {
	baseURL      string
	serviceToken string
	httpClient   *http.Client
	lastEventID  int64
}

// NewNexusCollector creates a NexusCollector.
func NewNexusCollector(baseURL, serviceToken string) *NexusCollector {
	return &NexusCollector{
		baseURL:      baseURL,
		serviceToken: serviceToken,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Collect fetches recent events from Nexus.
func (c *NexusCollector) Collect(ctx context.Context) []policy.NexusEvent {
	path := fmt.Sprintf("/events?since=%d&limit=100", c.lastEventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil
	}
	if c.serviceToken != "" {
		req.Header.Set("X-Service-Token", c.serviceToken)
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
		Data []struct {
			ID        int64  `json:"id"`
			Type      string `json:"type"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil
	}

	events := make([]policy.NexusEvent, 0, len(envelope.Data))
	for _, e := range envelope.Data {
		if e.ID > c.lastEventID {
			c.lastEventID = e.ID
		}
		ts, _ := time.Parse(time.RFC3339Nano, e.CreatedAt)
		events = append(events, policy.NexusEvent{
			Type:      e.Type,
			CreatedAt: ts,
		})
	}
	return events
}
