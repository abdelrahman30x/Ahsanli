package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ahsanly/go_platform/internal/catalog"
	"ahsanly/go_platform/internal/worker"
)

func main() {
	apply := flag.Bool("apply", false, "enable PostgreSQL offer writes; defaults to shadow candidates")
	once := flag.Bool("once", false, "process at most one due Amazon offer job")
	discover := flag.Bool("discover", false, "process enabled Amazon category pages; new listings start paused")
	capture := flag.Bool("capture", false, "capture one qualifying candidate; requires retailer publication gate")
	liveCandidates := flag.Bool("live-candidates", false, "create publishable candidates; requires both publication environment gates")
	browser := flag.String("browser", "", "headless Chromium executable for --capture")
	captureDir := flag.String("capture-dir", "", "evidence directory for --capture")
	flag.Parse()
	if !*apply {
		log.Fatal("worker is disabled by default; pass --apply only for an approved shadow run")
	}
	if (*discover && *capture) || (*liveCandidates && (*discover || *capture)) {
		log.Fatal("choose one offer, discovery, or capture mode")
	}
	if *liveCandidates && (os.Getenv("DEAL_PUBLISH_ENABLED") != "1" || os.Getenv("DEAL_GO_PUBLISH_ENABLED") != "1") {
		log.Fatal("live candidates require both publication environment gates")
	}
	if *capture && (os.Getenv("DEAL_PUBLISH_ENABLED") != "1" || os.Getenv("DEAL_GO_PUBLISH_ENABLED") != "1") {
		log.Fatal("candidate capture requires both publication environment gates; use cmd/capture for read-only diagnostics")
	}
	if *capture && (!*once || *browser == "" || *captureDir == "") {
		log.Fatal("--capture requires --once, --browser, and --capture-dir")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := catalog.New(ctx, catalog.DatabaseURL())
	if err != nil {
		log.Fatal("database connection failed: ", err)
	}
	defer store.DB.Close()
	runner := worker.New(store.DB)
	if *liveCandidates {
		runner.CandidateState = "new"
	}
	for {
		var worked bool
		if *capture {
			worked, err = runner.RunCaptureOnce(ctx, *browser, *captureDir)
		} else if *discover {
			worked, err = runner.RunDiscoverOnce(ctx)
		} else {
			worked, err = runner.RunOnce(ctx)
		}
		if err != nil {
			log.Printf("worker job failed: %v", err)
		}
		if *once || ctx.Err() != nil {
			break
		}
		if !worked {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}
}
