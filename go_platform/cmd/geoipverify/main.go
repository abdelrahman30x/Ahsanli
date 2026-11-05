package main

import (
	"fmt"
	"log"
	"net/netip"
	"os"

	"github.com/oschwald/maxminddb-golang/v2"
)

func main() {
	if len(os.Args) != 2 { log.Fatal("usage: ahsanly-geoipverify FILE.mmdb") }
	db, err := maxminddb.Open(os.Args[1])
	if err != nil { log.Fatal(err) }
	defer db.Close()
	if err = db.Verify(); err != nil { log.Fatal(err) }
	var code string
	if err = db.Lookup(netip.MustParseAddr("8.8.8.8")).DecodePath(&code, "country", "iso_code"); err != nil || code != "US" {
		log.Fatalf("country database failed known-address check: %v", err)
	}
	fmt.Println("MMDB verified")
}
