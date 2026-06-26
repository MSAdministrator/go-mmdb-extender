// Package config loads the YAML configuration that drives go-mmdb-extender:
// the input/output MMDB paths and a per-source config block for each
// enrichment source to apply.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
//
// Example config.yaml:
//
//	input: GeoLite2-ASN.mmdb
//	output: extended.mmdb
//	sources:
//	  ipinfo:
//	    path: ipinfo_lite.mmdb
//	  czds:
//	    zones: [com, net, org]
//	    data_dir: ./czds-data
//	    local_only: true
type Config struct {
	Input   string                    `yaml:"input"`
	Output  string                    `yaml:"output"`
	Sources map[string]map[string]any `yaml:"sources"`
}

// Load reads and parses the YAML config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	if c.Input == "" {
		return nil, fmt.Errorf("config %s: 'input' is required", path)
	}
	if c.Output == "" {
		return nil, fmt.Errorf("config %s: 'output' is required", path)
	}
	if len(c.Sources) == 0 {
		return nil, fmt.Errorf("config %s: at least one entry under 'sources' is required", path)
	}

	return &c, nil
}
