//go:build ignore

// Command gen builds the small synthetic GeoLite2-ASN fixture used in tests.
// It mirrors the real GeoLite2-ASN schema (autonomous_system_number +
// autonomous_system_organization) but with a handful of well-known networks so
// tests can assert against stable values without shipping the full ~10MB DB.
//
// Regenerate with:
//
//	go run gen.go
package main

import (
	"log"
	"net"
	"os"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func main() {
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "GeoLite2-ASN",
		RecordSize:   24,
	})
	if err != nil {
		log.Fatal(err)
	}

	entries := []struct {
		cidr string
		asn  uint32
		org  string
	}{
		// 1.0.0.0/24 overlaps the IPinfo location sample, so tests can assert
		// base ASN data and IPinfo enrichment coexist on the same network.
		{"1.0.0.0/24", 13335, "CLOUDFLARENET"},
		{"8.8.8.0/24", 15169, "GOOGLE"},
		{"9.9.9.0/24", 19281, "QUAD9-AS-1"},
	}

	for _, e := range entries {
		_, network, err := net.ParseCIDR(e.cidr)
		if err != nil {
			log.Fatal(err)
		}
		rec := mmdbtype.Map{
			"autonomous_system_number":       mmdbtype.Uint32(e.asn),
			"autonomous_system_organization": mmdbtype.String(e.org),
		}
		if err := tree.Insert(network, rec); err != nil {
			log.Fatal(err)
		}
	}

	f, err := os.Create("geolite2-asn-sample.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if _, err := tree.WriteTo(f); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote geolite2-asn-sample.mmdb (%d networks)", len(entries))
}
