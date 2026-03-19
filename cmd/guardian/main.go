// @guardian-project: guardian
// @guardian-path: cmd/guardian/main.go
// guardian is the platform policy observer daemon (ADR-013).
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

const guardianVersion = "0.1.0"

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
	forgeColl     := collector.NewForgeCollector(forgeAddr, serviceToken)
	navigatorColl := collector.NewNavigatorCollector(navigatorAddr)
	nexusColl     := collector.NewNexusCollector(nexusAddr, serviceToken)

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

// evaluate runs all collectors and policy engine, updates the report store.
func evaluate(
	ctx context.Context,
	engine *policy.Engine,
	forgeColl *collector.ForgeCollector,
	navColl *collector.NavigatorCollector,
	nexusColl *collector.NexusCollector,
	store *handler.ReportStore,
	logger *log.Logger,
) {
	execs    := forgeColl.Collect(ctx, "")
	nodes    := navColl.Collect(ctx, "")
	events   := nexusColl.Collect(ctx)
	services := nexusColl.CollectServices(ctx)
	projects := nexusColl.CollectProjects(ctx)

	report := engine.Evaluate(execs, nodes, events, services, projects)
	store.Set(report)
	logger.Printf("evaluated — %d finding(s) [%d warnings, %d errors]",
		report.Summary.Total, report.Summary.Warnings, report.Summary.Errors)
}
