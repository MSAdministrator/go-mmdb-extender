package source

import "github.com/maxmind/mmdbwriter"

// Source represents a data source that can enrich an MMDB database.
// Implement this interface to add new enrichment sources beyond CZDS.
type Source interface {
	// Name returns a short identifier for the source (e.g., "czds").
	Name() string

	// Enrich adds data from this source into the MMDB tree.
	// It should use inserter.TopLevelMergeWith to preserve existing data.
	Enrich(tree *mmdbwriter.Tree) error
}
