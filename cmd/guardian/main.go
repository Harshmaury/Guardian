// @guardian-project: guardian
// @guardian-path: cmd/guardian/main.go
// guardian is the platform policy observer daemon (ADR-013).
//
// Phase 2 (audit fixes):
//   - Each evaluation cycle carries a unique gd-<hex> trace ID (audit #3)
//   - Collectors log WARNING on upstream failure (audit #4)
//
// Startup sequence:
//  1. Config
//  2. Collectors (Forge, Navigator, Nexus)
//  3. Policy engine + report store
//  4. Initial evaluation
//  5. HTTP server (:8085)
//  6. Polling loop
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Harshmaury/Guardian/internal/api"
	"github.com/Harshmaury/Guardian/internal/api/handler"
	"github.com/Harshmaury/Guardian/internal/collector"
	"github.com/Harshmaury/Guardian/internal/config"
	"github.com/Harshmaury/Guardian/internal/policy"
)

const guardianVersion = "0.2.0"

func main() {
	logger := log.New(os.Stdout, "[guardian] ", log.LstdFlags)
	logger.Printf("Guardian v%s starting", guardianVersion)
	if err := run(logger); err != nil {
		logger.Fatalf("fatal: %v", err)
	}
	logger.Println("Guardian stopped cleanly")
}

func run(logger *log.Logger) error {
	// ── 1. CONFIG ────────────────────────────────────────────────────────────
	httpAddr      := config.EnvOrDefault("GUARDIAN_HTTP_ADDR", config.DefaultHTTPAddr)
	nexusAddr     := config.EnvOrDefault("NEXUS_HTTP_ADDR", config.DefaultNexusAddr)
	forgeAddr     := config.EnvOrDefault("FORGE_HTTP_ADDR", config.DefaultForgeAddr)
	navigatorAddr := config.EnvOrDefault("NAVIGATOR_HTTP_ADDR", config.DefaultNavigatorAddr)
	serviceToken  := config.EnvOrDefault("GUARDIAN_SERVICE_TOKEN", "")
	if serviceToken == "" {
		logger.Println("WARNING: GUARDIAN_SERVICE_TOKEN not set — upstream auth disabled")
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// ── 2. COLLECTORS ────────────────────────────────────────────────────────
	forgeColl     := collector.NewForgeCollector(forgeAddr, serviceToken, logger)
	navigatorColl := collector.NewNavigatorCollector(navigatorAddr, logger)
	nexusColl     := collector.NewNexusCollector(nexusAddr, serviceToken, logger)

	// ── 3. POLICY ENGINE + REPORT STORE ──────────────────────────────────────
	engine      := policy.NewEngine()
	reportStore := handler.NewReportStore()

	// ── 4. INITIAL EVALUATION ────────────────────────────────────────────────
	evaluate(ctx, engine, forgeColl, navigatorColl, nexusColl, reportStore, logger)
	logger.Printf("✓ Guardian ready — http=%s nexus=%s forge=%s navigator=%s",
		httpAddr, nexusAddr, forgeAddr, navigatorAddr)

	// ── 5. HTTP SERVER ───────────────────────────────────────────────────────
	srv := api.NewServer(httpAddr, reportStore, logger)

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("api server: %w", err)
		}
	}()

	// ── 6. POLLING LOOP ───────────────────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				evaluate(ctx, engine, forgeColl, navigatorColl, nexusColl, reportStore, logger)
			}
		}
	}()

	select {
	case sig := <-sigCh:
		logger.Printf("received %s — shutting down", sig)
	case err := <-errCh:
		logger.Printf("component error: %v — shutting down", err)
	}

	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	<-done
	return nil
}

// evaluate runs one collection + policy cycle with a unique trace ID.
// The trace ID propagates to all upstream calls via X-Trace-ID (audit #3).
func evaluate(
	ctx context.Context,
	engine *policy.Engine,
	forgeColl *collector.ForgeCollector,
	navColl *collector.NavigatorCollector,
	nexusColl *collector.NexusCollector,
	store *handler.ReportStore,
	logger *log.Logger,
) {
	traceID := newCycleTraceID()

	execs    := forgeColl.Collect(ctx, traceID)
	nodes    := navColl.Collect(ctx, traceID)
	events   := nexusColl.Collect(ctx, traceID)
	services := nexusColl.CollectServices(ctx, traceID)
	projects := nexusColl.CollectProjects(ctx, traceID)

	report := engine.Evaluate(execs, nodes, events, services, projects)
	store.Set(report)
	logger.Printf("evaluated trace=%s — %d finding(s) [%d warnings, %d errors]",
		traceID, report.Summary.Total, report.Summary.Warnings, report.Summary.Errors)
}

// newCycleTraceID generates a unique gd-<hex> trace ID for one evaluation cycle.
func newCycleTraceID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("gd-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("gd-%x", b)
}
