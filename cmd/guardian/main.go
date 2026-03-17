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
	"crypto/rand"
	"encoding/hex"
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

// guardianConfig holds resolved runtime configuration.
type guardianConfig struct {
	httpAddr      string
	nexusAddr     string
	forgeAddr     string
	navigatorAddr string
	serviceToken  string
}

// loadConfig reads all environment variables and logs warnings.
func loadConfig(logger *log.Logger) guardianConfig {
	cfg := guardianConfig{
		httpAddr:      config.EnvOrDefault("GUARDIAN_HTTP_ADDR", config.DefaultHTTPAddr),
		nexusAddr:     config.EnvOrDefault("NEXUS_HTTP_ADDR", config.DefaultNexusAddr),
		forgeAddr:     config.EnvOrDefault("FORGE_HTTP_ADDR", config.DefaultForgeAddr),
		navigatorAddr: config.EnvOrDefault("NAVIGATOR_HTTP_ADDR", config.DefaultNavigatorAddr),
		serviceToken:  config.EnvOrDefault("GUARDIAN_SERVICE_TOKEN", ""),
	}
	if cfg.serviceToken == "" {
		logger.Println("WARNING: GUARDIAN_SERVICE_TOKEN not set — upstream auth disabled")
	}
	return cfg
}

func run(logger *log.Logger) error {
	cfg := loadConfig(logger)

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// ── 2. COLLECTORS ────────────────────────────────────────────────────────
	forgeColl     := collector.NewForgeCollector(cfg.forgeAddr, cfg.serviceToken)
	navigatorColl := collector.NewNavigatorCollector(cfg.navigatorAddr)
	nexusColl     := collector.NewNexusCollector(cfg.nexusAddr, cfg.serviceToken)

	// ── 3. POLICY ENGINE + REPORT STORE ──────────────────────────────────────
	engine      := policy.NewEngine()
	reportStore := handler.NewReportStore()

	// ── 4. INITIAL EVALUATION ────────────────────────────────────────────────
	evaluate(ctx, engine, forgeColl, navigatorColl, nexusColl, reportStore, logger)
	logger.Printf("✓ Guardian ready — http=%s nexus=%s forge=%s navigator=%s",
		cfg.httpAddr, cfg.nexusAddr, cfg.forgeAddr, cfg.navigatorAddr)

	return serveAndWait(ctx, cancel, sigCh, cfg.httpAddr, reportStore,
		engine, forgeColl, navigatorColl, nexusColl, logger)
}

// serveAndWait starts the HTTP server and polling loop, blocks until shutdown.
func serveAndWait(
	ctx context.Context,
	cancel context.CancelFunc,
	sigCh <-chan os.Signal,
	httpAddr string,
	reportStore *handler.ReportStore,
	engine *policy.Engine,
	forgeColl *collector.ForgeCollector,
	navigatorColl *collector.NavigatorCollector,
	nexusColl *collector.NexusCollector,
	logger *log.Logger,
) error {
	srv  := api.NewServer(httpAddr, reportStore, logger)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("api server: %w", err)
		}
	}()

	wg.Add(1)
	go startPollingLoop(ctx, &wg, engine, forgeColl, navigatorColl, nexusColl, reportStore, logger)

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

// startPollingLoop runs the 30-second evaluation cycle until ctx is cancelled.
func startPollingLoop(
	ctx context.Context,
	wg *sync.WaitGroup,
	engine *policy.Engine,
	forgeColl *collector.ForgeCollector,
	navColl *collector.NavigatorCollector,
	nexusColl *collector.NexusCollector,
	store *handler.ReportStore,
	logger *log.Logger,
) {
	defer wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evaluate(ctx, engine, forgeColl, navColl, nexusColl, store, logger)
		}
	}
}

// evaluate runs all collectors and policy engine, updates the report store.
// A fresh trace ID is generated per cycle for X-Trace-ID propagation (FEAT-002).
func evaluate(
	ctx context.Context,
	engine *policy.Engine,
	forgeColl *collector.ForgeCollector,
	navColl *collector.NavigatorCollector,
	nexusColl *collector.NexusCollector,
	store *handler.ReportStore,
	logger *log.Logger,
) {
	traceID := newTraceID()

	execs  := forgeColl.Collect(ctx, traceID)
	nodes  := navColl.Collect(ctx, traceID)
	events := nexusColl.Collect(ctx, traceID)

	report := engine.Evaluate(execs, nodes, events)
	store.Set(report)
	logger.Printf("evaluated trace=%s — %d finding(s) [%d warnings, %d errors]",
		traceID, report.Summary.Total, report.Summary.Warnings, report.Summary.Errors)
}

// newTraceID generates a random 16-byte hex trace ID for collection cycles.
func newTraceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("gd-%d", time.Now().UnixNano())
	}
	return "gd-" + hex.EncodeToString(b)
}
