// @guardian-project: guardian
// @guardian-path: internal/collector/nexus.go
package collector

import (
	"context"
	"encoding/json"
	"fmt"
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
		req.Header.Set(canon.ServiceTokenHeader, c.serviceToken)
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

// CollectServices fetches service records from Nexus GET /services.
func (c *NexusCollector) CollectServices(ctx context.Context) []policy.ServiceRecord {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/services", nil)
	if err != nil {
		return nil
	}
	if c.serviceToken != "" {
		req.Header.Set(canon.ServiceTokenHeader, c.serviceToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()
	var env struct {
		OK   bool `json:"ok"`
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

// CollectProjects fetches registered projects from Nexus GET /projects.
func (c *NexusCollector) CollectProjects(ctx context.Context) []policy.ProjectRecord {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/projects", nil)
	if err != nil {
		return nil
	}
	if c.serviceToken != "" {
		req.Header.Set(canon.ServiceTokenHeader, c.serviceToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()
	var env struct {
		OK   bool `json:"ok"`
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
