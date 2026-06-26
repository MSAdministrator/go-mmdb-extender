package ipinfo

import (
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

// sampleMMDB is a small real IPinfo location-sample database, embedded so the
// test binary is self-contained (no dependency on the working directory or an
// on-disk file).
//
//go:embed testdata/sample.mmdb
var sampleMMDB []byte

// geoLite2ASNSample is a small synthetic GeoLite2-ASN fixture (schema-faithful:
// autonomous_system_number + autonomous_system_organization) used as a base
// database to enrich. Regenerate with: go run testdata/gen.go
//
//go:embed testdata/geolite2-asn-sample.mmdb
var geoLite2ASNSample []byte

// writeSampleMMDB materializes the embedded IPinfo sample to a temp file and
// returns its path. Our source APIs take a file path, so the embedded bytes are
// written out before use.
func writeSampleMMDB(t *testing.T) string {
	return writeEmbedded(t, "sample.mmdb", sampleMMDB)
}

// writeGeoLiteSample materializes the embedded GeoLite2-ASN fixture to a temp
// file and returns its path.
func writeGeoLiteSample(t *testing.T) string {
	return writeEmbedded(t, "geolite2-asn-sample.mmdb", geoLite2ASNSample)
}

func writeEmbedded(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
