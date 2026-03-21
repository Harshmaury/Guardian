// @guardian-project: guardian
// @guardian-path: internal/collector/nexus.go
// ADR-039: Herald migration — replaces raw http.Get calls with typed herald client.
// Bug fix: decodeProjects was using json:"ProjectID" — Nexus returns json:"id".
//   G-007 and G-008 rules were never firing because projects slice was always empty.
// Retry on transient failures is now handled by herald (3 attempts, backoff).
package collector

import (
	"context"
	"log"
	"time"

	"github.com/Harshmaury/Guardian/internal/policy"
	herald "github.com/Harshmaury/Herald/client"
	accord "github.com/Harshmaury/Accord/api"
)

// NexusCollector polls Nexus for Guardian rule evaluation.
// Uses herald for typed, retried, version-checked API calls.
type NexusCollector struct {
	client      *herald.Client
	logger      *log.Logger
	lastEventID int64
}

// NewNexusCollector creates a NexusCollector.
func NewNexusCollector(baseURL, serviceToken string, logger *log.Logger) *NexusCollector {
	return &NexusCollector{
		client: herald.New(baseURL, herald.WithToken(serviceToken)),
		logger: logger,
	}
}

// Collect fetches recent events from Nexus since the last known event ID.
func (c *NexusCollector) Collect(ctx context.Context, traceID string) []policy.NexusEvent {
	events, err := c.client.Events().Since(ctx, c.lastEventID, 100)
	if err != nil {
		c.logger.Printf("WARNING: nexus collector (events): %v", err)
		return nil
	}
	result := make([]policy.NexusEvent, 0, len(events))
	for _, e := range events {
		if e.ID > c.lastEventID {
			c.lastEventID = e.ID
		}
		ts := parseTime(e.CreatedAt)
		result = append(result, policy.NexusEvent{Type: e.Type, CreatedAt: ts})
	}
	return result
}

// CollectServices fetches all registered services from Nexus.
func (c *NexusCollector) CollectServices(ctx context.Context, _ string) []policy.ServiceRecord {
	svcs, err := c.client.Services().List(ctx)
	if err != nil {
		c.logger.Printf("WARNING: nexus collector (services): %v", err)
		return nil
	}
	out := make([]policy.ServiceRecord, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, policy.ServiceRecord{
			ID:           s.ID,
			ProjectID:    s.Project,
			DesiredState: s.DesiredState,
			ActualState:  s.ActualState,
		})
	}
	return out
}

// CollectProjects fetches all registered projects from Nexus.
// FIX: previously used json:"ProjectID" — Nexus returns json:"id".
// accord.ProjectDTO has the correct field mapping.
func (c *NexusCollector) CollectProjects(ctx context.Context, _ string) []policy.ProjectRecord {
	projs, err := c.client.Projects().List(ctx)
	if err != nil {
		c.logger.Printf("WARNING: nexus collector (projects): %v", err)
		return nil
	}
	out := make([]policy.ProjectRecord, 0, len(projs))
	for _, p := range projs {
		if p.ID != "" {
			out = append(out, policy.ProjectRecord{ID: p.ID})
		}
	}
	return out
}

// parseTime parses RFC3339/RFC3339Nano timestamps, returning zero on failure.
func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// toServiceRecords converts accord DTOs to policy records.
// Kept for potential future use — currently inlined in CollectServices.
func toServiceRecords(svcs []accord.ServiceDTO) []policy.ServiceRecord {
	out := make([]policy.ServiceRecord, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, policy.ServiceRecord{
			ID:           s.ID,
			ProjectID:    s.Project,
			DesiredState: s.DesiredState,
			ActualState:  s.ActualState,
		})
	}
	return out
}
