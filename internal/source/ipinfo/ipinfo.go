// Package ipinfo enriches an MMDB with data from an IPinfo MMDB database, such
// as the free IPinfo Lite database (https://ipinfo.io/developers/ipinfo-lite-database).
//
// IPinfo ships its datasets as MMDB files, so enrichment is a straight
// MMDB-to-MMDB merge: every network in the IPinfo database is merged into the
// tree under the top-level "ipinfo" key, preserving existing data.
package ipinfo

import (
	"errors"
	"fmt"
	"log"

	"github.com/maxmind/mmdbwriter"
	"github.com/msadministrator/go-mmdb-extender/internal/mmdbmerge"
	"github.com/msadministrator/go-mmdb-extender/internal/source"
)

func init() {
	source.Register("ipinfo", func(cfg map[string]any) (source.Source, error) {
		path, _ := cfg["path"].(string)
		if path == "" {
			return nil, errors.New("'path' to an IPinfo MMDB file is required")
		}
		return &Source{path: path}, nil
	})
}

// Source enriches an MMDB by merging the records of an IPinfo MMDB file (e.g.
// IPinfo Lite) under the "ipinfo" key.
type Source struct {
	path string // path to the IPinfo MMDB file
}

// New creates an IPinfo source that reads the MMDB at path.
func New(path string) *Source { return &Source{path: path} }

func (s *Source) Name() string { return "ipinfo" }

func (s *Source) Enrich(tree *mmdbwriter.Tree) error {
	log.Printf("ipinfo: merging records from %s", s.path)
	n, err := mmdbmerge.Merge(tree, s.path, s.Name())
	if err != nil {
		return fmt.Errorf("merging IPinfo data: %w", err)
	}
	log.Printf("ipinfo: merged %d networks into MMDB", n)
	return nil
}
