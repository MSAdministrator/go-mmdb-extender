package source

import (
	"fmt"
	"sort"
	"sync"

	"github.com/maxmind/mmdbwriter"
)

// Source represents a data source that can enrich an MMDB database.
// Implement this interface to add new enrichment sources.
//
// Each source MUST namespace its data under a single top-level key matching
// Name() and insert with inserter.TopLevelMergeWith so that multiple sources
// compose without clobbering each other's data or the input database.
type Source interface {
	// Name returns a short identifier for the source (e.g., "czds").
	// It is also the top-level MMDB key the source writes under.
	Name() string

	// Enrich adds data from this source into the MMDB tree.
	Enrich(tree *mmdbwriter.Tree) error
}

// Factory builds a configured Source from its config block (the decoded YAML
// map for that source). It returns (nil, nil) when the source is present in
// config but should be skipped (e.g. optional and not fully configured).
type Factory func(cfg map[string]any) (Source, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register associates a source name with its factory. It is intended to be
// called from a source package's init() function. It panics on duplicate
// registration, which can only happen as a programming error.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := factories[name]; dup {
		panic("source: duplicate registration for " + name)
	}
	factories[name] = f
}

// Registered returns the names of all registered sources, sorted.
func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Build constructs a Source for every entry in configs, using the registered
// factory for each. Sources are returned in stable (sorted) name order so runs
// are deterministic. An unknown source name is an error.
func Build(configs map[string]map[string]any) ([]Source, error) {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(configs))
	for n := range configs {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []Source
	for _, name := range names {
		f, ok := factories[name]
		if !ok {
			return nil, fmt.Errorf("unknown source %q (registered: %v)", name, registeredLocked())
		}
		s, err := f(configs[name])
		if err != nil {
			return nil, fmt.Errorf("configuring source %q: %w", name, err)
		}
		if s != nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// registeredLocked returns sorted source names. Caller must hold mu.
func registeredLocked() []string {
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
