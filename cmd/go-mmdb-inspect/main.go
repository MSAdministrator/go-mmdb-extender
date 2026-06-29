// Command go-mmdb-inspect inspects an MMDB file: it prints the database
// metadata, iterates every network (reporting a total and showing a few sample
// records), and optionally performs a direct IP lookup. It is a quick way to
// verify the output of go-mmdb-extender.
//
// Usage:
//
//	go-mmdb-inspect [--samples N] [--lookup IP] <database.mmdb>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

func main() {
	samples := flag.Int("samples", 3, "number of sample records to print while iterating networks")
	lookup := flag.String("lookup", "", "look up a single IP address and print its record")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [--samples N] [--lookup IP] <database.mmdb>\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := flag.Arg(0)

	db, err := maxminddb.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", path, err)
		os.Exit(1)
	}
	defer db.Close()

	printMetadata(path, db.Metadata)

	if err := iterateNetworks(db, *samples); err != nil {
		fmt.Fprintf(os.Stderr, "Error iterating networks: %v\n", err)
		os.Exit(1)
	}

	if *lookup != "" {
		if err := lookupIP(db, *lookup); err != nil {
			fmt.Fprintf(os.Stderr, "Error looking up %s: %v\n", *lookup, err)
			os.Exit(1)
		}
	}
}

// printMetadata reports the database-level metadata.
func printMetadata(path string, m maxminddb.Metadata) {
	fmt.Printf("Database:        %s\n", path)
	fmt.Printf("Type:            %s\n", m.DatabaseType)
	if len(m.Description) > 0 {
		fmt.Printf("Description:     %v\n", m.Description)
	}
	if len(m.Languages) > 0 {
		fmt.Printf("Languages:       %v\n", m.Languages)
	}
	fmt.Printf("IP version:      IPv%d\n", m.IPVersion)
	fmt.Printf("Record size:     %d bits\n", m.RecordSize)
	fmt.Printf("Node count:      %d\n", m.NodeCount)
	fmt.Printf("Binary format:   %d.%d\n", m.BinaryFormatMajorVersion, m.BinaryFormatMinorVersion)
	if m.BuildEpoch > 0 {
		built := time.Unix(int64(m.BuildEpoch), 0).UTC()
		fmt.Printf("Build epoch:     %d (%s)\n", m.BuildEpoch, built.Format(time.RFC3339))
	}
	fmt.Println()
}

// iterateNetworks walks every (non-aliased) network in the database, counts
// them, and prints the first `samples` records as indented JSON.
func iterateNetworks(db *maxminddb.Reader, samples int) error {
	fmt.Println("Networks:")
	var count int
	nets := db.Networks(maxminddb.SkipAliasedNetworks)
	for nets.Next() {
		var record any
		network, err := nets.Network(&record)
		if err != nil {
			return err
		}
		if count < samples {
			fmt.Printf("  %s\n%s\n", network, indentJSON(record, "    "))
		}
		count++
	}
	if err := nets.Err(); err != nil {
		return err
	}
	fmt.Printf("\nTotal networks: %d\n\n", count)
	return nil
}

// lookupIP performs a direct lookup of a single IP and prints its record.
func lookupIP(db *maxminddb.Reader, ipStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("invalid IP address %q", ipStr)
	}
	var record any
	if err := db.Lookup(ip, &record); err != nil {
		return err
	}
	fmt.Printf("Lookup %s:\n", ipStr)
	if record == nil {
		fmt.Println("  (no record found)")
		return nil
	}
	fmt.Println(indentJSON(record, "  "))
	return nil
}

// indentJSON renders v as indented JSON, each line prefixed with `prefix`.
// On marshal failure it falls back to Go's default formatting so we never drop
// the record entirely.
func indentJSON(v any, prefix string) string {
	b, err := json.MarshalIndent(v, prefix, "  ")
	if err != nil {
		return fmt.Sprintf("%s%v", prefix, v)
	}
	return string(b)
}
