// Package mmdbmerge provides a reusable engine for the class of enrichment
// sources whose upstream data is itself an MMDB file (e.g. IPinfo Lite,
// GeoLite2, and most commercial IP datasets). It iterates every network in a
// source MMDB and merges its record into a writable tree under a single
// top-level key, preserving any existing data.
package mmdbmerge

import (
	"fmt"
	"strings"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/oschwald/maxminddb-golang"
)

// Merge iterates every (non-aliased) network in the MMDB at srcPath and merges
// its record into tree under the top-level key `key`, preserving existing data
// via inserter.TopLevelMergeWith. It returns the number of networks merged.
func Merge(tree *mmdbwriter.Tree, srcPath, key string) (int, error) {
	db, err := maxminddb.Open(srcPath)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", srcPath, err)
	}
	defer db.Close()

	var merged int
	nets := db.Networks(maxminddb.SkipAliasedNetworks)
	for nets.Next() {
		// Decode into `any` rather than a map: MMDB records can have any
		// top-level type (map, slice, or scalar). toMMDBType handles the shape.
		var raw any
		network, err := nets.Network(&raw)
		if err != nil {
			return merged, fmt.Errorf("decoding network from %s: %w", srcPath, err)
		}
		if raw == nil {
			continue
		}

		record := mmdbtype.Map{
			mmdbtype.String(key): toMMDBType(raw),
		}

		if err := tree.InsertFunc(network, inserter.TopLevelMergeWith(record)); err != nil {
			errMsg := err.Error()
			// Aliased/reserved networks can't be inserted; skip them rather
			// than aborting the whole merge.
			if strings.Contains(errMsg, "aliased network") || strings.Contains(errMsg, "reserved network") {
				continue
			}
			return merged, fmt.Errorf("merging %s: %w", network, err)
		}
		merged++
	}
	if err := nets.Err(); err != nil {
		return merged, fmt.Errorf("iterating %s: %w", srcPath, err)
	}
	return merged, nil
}

// toMMDBType converts a value decoded from an MMDB (into Go's `any`) back into
// the corresponding mmdbtype value so it can be written to a tree. The
// maxminddb reader decodes into the standard Go types enumerated here.
func toMMDBType(v any) mmdbtype.DataType {
	switch t := v.(type) {
	case map[string]any:
		m := make(mmdbtype.Map, len(t))
		for k, vv := range t {
			m[mmdbtype.String(k)] = toMMDBType(vv)
		}
		return m
	case []any:
		s := make(mmdbtype.Slice, 0, len(t))
		for _, vv := range t {
			s = append(s, toMMDBType(vv))
		}
		return s
	case string:
		return mmdbtype.String(t)
	case bool:
		return mmdbtype.Bool(t)
	case float64:
		return mmdbtype.Float64(t)
	case float32:
		return mmdbtype.Float32(t)
	case int:
		return mmdbtype.Int32(int32(t))
	case int32:
		return mmdbtype.Int32(t)
	case uint16:
		return mmdbtype.Uint16(t)
	case uint32:
		return mmdbtype.Uint32(t)
	case uint64:
		return mmdbtype.Uint64(t)
	case uint:
		return mmdbtype.Uint64(uint64(t))
	case []byte:
		return mmdbtype.Bytes(t)
	default:
		// Fall back to a string representation rather than dropping data.
		return mmdbtype.String(fmt.Sprintf("%v", t))
	}
}
