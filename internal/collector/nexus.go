// @guardian-project: guardian
// @guardian-path: internal/collector/nexus.go
// Phase 2: injected *log.Logger, traceID on all methods, WARNING on failure (audit #3, #4).
// Canon migration already applied in Wave 0 (X-Service-Token → canon.ServiceTokenHeader).
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

// NexusCollector polls Nexus GET /events for Guardian rule evaluation.
type NexusCollector struct {
	baseURL      string
	serviceToken string
	httpClient   *http.Client
	logger       *log.Logger
	lastEventID  int64
}

// NewNexusCollector creates a NexusCollector.
func NewNexusCollector(baseURL, serviceToken string, logger *log.Logger) *NexusCollector {
	return &NexusCollector{
		baseURL:      baseURL,
		serviceToken: serviceToken,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		logger:       logger,
	}
}

// Collect fetches recent events from Nexus.
// Returns nil and logs a WARNING if Nexus is unreachable (audit #4).
func (c *NexusCollector) Collect(ctx context.Context, traceID string) []policy.NexusEvent {
	path := fmt.Sprintf("/events?since=%d&limit=100", c.lastEventID)
	resp, err := c.get(ctx, path, traceID)
	if err != nil {
		c.logger.Printf("WARNING: nexus collector (events) unreachable: %v", err)
		return nil
	}
	defer resp.Body.Close()
	return c.decodeEvents(resp)
}

// CollectServices fetches service records from Nexus GET /services.
func (c *NexusCollector) CollectServices(ctx context.Context, traceID string) []policy.ServiceRecord {
	resp, err := c.get(ctx, "/services", traceID)
	if err != nil {
		c.logger.Printf("WARNING: nexus collector (services) unreachable: %v", err)
		return nil
	}
	defer resp.Body.Close()
	return c.decodeServices(resp)
}

// CollectProjects fetches registered projects from Nexus GET /projects.
func (c *NexusCollector) CollectProjects(ctx context.Context, traceID string) []policy.ProjectRecord {
	resp, err := c.get(ctx, "/projects", traceID)
	if err != nil {
		c.logger.Printf("WARNING: nexus collector (projects) unreachable: %v", err)
		return nil
	}
	defer resp.Body.Close()
	return c.decodeProjects(resp)
}

// get issues an authenticated GET and returns the response or an error.
func (c *NexusCollector) get(ctx context.Context, path, traceID string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.serviceToken != "" {
		req.Header.Set(canon.ServiceTokenHeader, c.serviceToken)
	}
	if traceID != "" {
		req.Header.Set(canon.TraceIDHeader, traceID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return resp, nil
}

// decodeEvents parses the /events response.
func (c *NexusCollector) decodeEvents(resp *http.Response) []policy.NexusEvent {
	var envelope struct {
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
		events = append(events, policy.NexusEvent{Type: e.Type, CreatedAt: ts})
	}
	return events
}

// decodeServices parses the /services response.
func (c *NexusCollector) decodeServices(resp *http.Response) []policy.ServiceRecord {
	var env struct {
		Data []struct {
			ID           string `json:"id"`
			Project      string `json:"project"`
			DesiredState string `json:"desired_state"`
			ActualState  string `json:"actual_state"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil
	}
	out := make([]policy.ServiceRecord, 0, len(env.Data))
	for _, s := range env.Data {
		out = append(out, policy.ServiceRecord{
			ID:           s.ID,
			ProjectID:    s.Project,
			DesiredState: s.DesiredState,
			ActualState:  s.ActualState,
		})
	}
	return out
}

// decodeProjects parses the /projects response.
func (c *NexusCollector) decodeProjects(resp *http.Response) []policy.ProjectRecord {
	var env struct {
		Data []struct {
			ProjectID string `json:"ProjectID"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil
	}
	out := make([]policy.ProjectRecord, 0, len(env.Data))
	for _, p := range env.Data {
		if p.ProjectID != "" {
			out = append(out, policy.ProjectRecord{ID: p.ProjectID})
		}
	}
	return out
}
