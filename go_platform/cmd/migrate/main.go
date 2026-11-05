package main

import (
	"context"
	"flag"
	"log"

	"ahsanly/go_platform/internal/catalog"
	"ahsanly/go_platform/internal/schema"
)

func main() {
	apply := flag.Bool("apply", false, "apply pending Go schema migrations; default is read-only")
	flag.Parse()
	ctx := context.Background()
	store, err := catalog.New(ctx, catalog.DatabaseURL())
	if err != nil {
		log.Fatal("database unavailable")
	}
	defer store.DB.Close()
	pending, err := schema.Apply(ctx, store.DB, *apply)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Go schema pending/applied: %v", pending)
}
