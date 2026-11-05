// Command pipeline captures verified Amazon deals and sends their photos.
// Routine offer checks run in the separate Go worker without a browser.
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
	"ahsanly/go_platform/internal/publishing"
	"ahsanly/go_platform/internal/worker"
)

func main() {
	apply := flag.Bool("apply", false, "enable the verified photo pipeline")
	allowLive := flag.Bool("allow-live-channels", false, "include public Telegram channels")
	browser := flag.String("browser", "", "headless Chromium executable")
	captureDir := flag.String("capture-dir", "", "directory for verified screenshots")
	flag.Parse()
	if !*apply || !*allowLive || *browser == "" || *captureDir == "" {
		log.Fatal("requires --apply, --allow-live-channels, --browser and --capture-dir")
	}
	if os.Getenv("DEAL_PUBLISH_ENABLED") != "1" || os.Getenv("DEAL_GO_PUBLISH_ENABLED") != "1" {
		log.Fatal("Go photo pipeline disabled by environment gates")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := catalog.New(ctx, catalog.DatabaseURL())
	if err != nil {
		log.Fatal("database connection failed: ", err)
	}
	defer store.DB.Close()
	runner := worker.New(store.DB)
	box := publishing.Outbox{DB: store.DB, CaptureDir: *captureDir, Enabled: true, AllowLiveChannels: true}
	lastRecovery := time.Time{}
	for ctx.Err() == nil {
		if time.Since(lastRecovery) >= time.Minute {
			if n, e := box.RecoverUncertain(ctx); e != nil {
				log.Printf("delivery recovery failed: %v", e)
			} else if n > 0 {
				log.Printf("uncertain deliveries held for review: %d", n)
			}
			lastRecovery = time.Now()
		}
		worked := false
		if yes, e := runner.RunCaptureOnce(ctx, *browser, *captureDir); e != nil {
			log.Printf("candidate capture failed: %v", e)
			worked = true
		} else if yes {
			worked = true
		}
		if yes, e := box.PrepareOne(ctx); e != nil {
			log.Printf("publication preparation failed: %v", e)
			worked = true
		} else if yes {
			worked = true
		}
		if yes, e := box.SendOne(ctx); e != nil {
			log.Printf("photo delivery failed: %v", e)
			worked = true
		} else if yes {
			log.Printf("one verified photo delivery processed")
			worked = true
		}
		pause := 15 * time.Second
		if worked {
			pause = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}
}
