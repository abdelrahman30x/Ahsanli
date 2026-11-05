// Command publish is the gated Go photo outbox. It is not enabled by default.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"ahsanly/go_platform/internal/catalog"
	"ahsanly/go_platform/internal/publishing"
)

func main() {
	apply := flag.Bool("apply", false, "allow one gated outbox operation")
	prepare := flag.Bool("prepare", false, "prepare one editorially approved candidate for routed channels")
	send := flag.Bool("send", false, "send one pending verified photo")
	recoverUncertain := flag.Bool("recover-uncertain", false, "hold expired sending records for manual review")
	captureDir := flag.String("capture-dir", "", "directory containing verified evidence PNGs")
	allowLive := flag.Bool("allow-live-channels", false, "include public channels after controlled cutover; default is private test channels only")
	flag.Parse()
	modes := 0
	for _, v := range []bool{*prepare, *send, *recoverUncertain} {
		if v {
			modes++
		}
	}
	if !*apply || modes != 1 || *captureDir == "" {
		log.Fatal("requires --apply, --capture-dir, and exactly one operation")
	}
	if os.Getenv("DEAL_PUBLISH_ENABLED") != "1" || os.Getenv("DEAL_GO_PUBLISH_ENABLED") != "1" {
		log.Fatal("Go publisher is disabled by environment gates")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := catalog.New(ctx, catalog.DatabaseURL())
	if err != nil {
		log.Fatal("database connection failed")
	}
	defer store.DB.Close()
	box := publishing.Outbox{DB: store.DB, CaptureDir: *captureDir, Enabled: true, AllowLiveChannels: *allowLive}
	if *recoverUncertain {
		n, e := box.RecoverUncertain(ctx)
		if e != nil {
			log.Fatal(e)
		}
		log.Printf("uncertain records requiring review: %d", n)
		return
	}
	var worked bool
	if *prepare {
		worked, err = box.PrepareOne(ctx)
	} else {
		worked, err = box.SendOne(ctx)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("operation completed: worked=%t", worked)
}
