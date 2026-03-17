// @guardian-project: guardian
// @guardian-path: internal/collector/forge.go
package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Harshmaury/Canon/identity"
	"github.com/Harshmaury/Guardian/internal/policy"
)

// ForgeCollector polls Forge GET /history.
type ForgeCollector struct {
	baseURL      string
	serviceToken string
	httpClient   *http.Client
}

// NewForgeCollector creates a ForgeCollector.
func NewForgeCollector(baseURL, serviceToken string) *ForgeCollector {
	return &ForgeCollector{
		baseURL:      baseURL,
		serviceToken: serviceToken,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Collect fetches recent execution records from Forge.
// traceID is the collection-cycle trace ID for X-Trace-ID propagation (FEAT-002).
func (c *ForgeCollector) Collect(ctx context.Context, traceID string) []policy.ExecutionRecord {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/history?limit=100", nil)
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

	return decodeForgeResponse(resp)
}

// decodeForgeResponse parses the Forge /history response body.
func decodeForgeResponse(resp *http.Response) []policy.ExecutionRecord {
	var envelope struct {
		OK   bool `json:"ok"`
		Data []struct {
			Target    string `json:"target"`
			Status    string `json:"status"`
			StartedAt string `json:"started_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil
	}

	records := make([]policy.ExecutionRecord, 0, len(envelope.Data))
	for _, d := range envelope.Data {
		ts, err := time.Parse(time.RFC3339Nano, d.StartedAt)
		if err != nil {
			ts = time.Time{} // zero time on malformed timestamp — safe default
		}
		records = append(records, policy.ExecutionRecord{
			Target:    d.Target,
			Status:    d.Status,
			StartedAt: ts,
		})
	}
	return records
}

