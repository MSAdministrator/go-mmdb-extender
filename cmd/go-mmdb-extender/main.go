package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/maxmind/mmdbwriter"
	"github.com/msadministrator/go-mmdb-extender/internal/config"
	"github.com/msadministrator/go-mmdb-extender/internal/source"

	// Register the available enrichment sources. Adding a new source is just
	// a new package with an init() that calls source.Register, plus a blank
	// import here -- main itself never names a source.
	_ "github.com/msadministrator/go-mmdb-extender/internal/source/czds"
	_ "github.com/msadministrator/go-mmdb-extender/internal/source/ipinfo"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		fmt.Fprintf(os.Stderr, "\nRegistered sources: %v\n", source.Registered())
		os.Exit(1)
	}

	// Build the configured enrichment sources from the registry.
	sources, err := source.Build(cfg.Sources)
	if err != nil {
		log.Fatalf("Failed to build sources: %v", err)
	}
	if len(sources) == 0 {
		log.Fatal("No enrichment sources configured (check credentials / config).")
	}

	// Load existing MMDB into a writable tree.
	log.Printf("Loading MMDB from %s", cfg.Input)
	tree, err := mmdbwriter.Load(cfg.Input, mmdbwriter.Options{RecordSize: 28})
	if err != nil {
		log.Fatalf("Failed to load MMDB: %v", err)
	}

	// Apply each source in turn.
	for _, s := range sources {
		log.Printf("Applying source: %s", s.Name())
		if err := s.Enrich(tree); err != nil {
			log.Fatalf("Source %s failed: %v", s.Name(), err)
		}
	}

	// Write the extended MMDB.
	f, err := os.Create(cfg.Output)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	if _, err := tree.WriteTo(f); err != nil {
		log.Fatalf("Failed to write MMDB: %v", err)
	}

	log.Printf("Extended MMDB written to %s", cfg.Output)
}
