// @guardian-project: guardian
// @guardian-path: internal/collector/nexus.go
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Harshmaury/Canon/identity"
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
// traceID is the collection-cycle trace ID for X-Trace-ID propagation (FEAT-002).
func (c *NexusCollector) Collect(ctx context.Context, traceID string) []policy.NexusEvent {
	path := fmt.Sprintf("/events?since=%d&limit=100", c.lastEventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil
	}
	if c.serviceToken != "" {
		req.Header.Set(identity.ServiceTokenHeader, c.serviceToken)
	}
	if traceID != "" {
		req.Header.Set(identity.TraceIDHeader, traceID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	return c.decodeNexusResponse(resp)
}

// decodeNexusResponse parses the Nexus /events response body and advances lastEventID.
func (c *NexusCollector) decodeNexusResponse(resp *http.Response) []policy.NexusEvent {
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
		ts, err := time.Parse(time.RFC3339Nano, e.CreatedAt)
		if err != nil {
			ts = time.Time{} // zero time on malformed timestamp — safe default
		}
		events = append(events, policy.NexusEvent{
			Type:      e.Type,
			CreatedAt: ts,
		})
	}
	return events
}
