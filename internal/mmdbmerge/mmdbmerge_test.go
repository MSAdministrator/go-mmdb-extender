package mmdbmerge

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/oschwald/maxminddb-golang"
)

// writeMMDB builds a small MMDB at path containing the given network->record
// entries and returns the path.
func writeMMDB(t *testing.T, path string, entries map[string]mmdbtype.Map) {
	t.Helper()
	tree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test", RecordSize: 28})
	if err != nil {
		t.Fatal(err)
	}
	for cidr, rec := range entries {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		if err := tree.Insert(network, rec); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := tree.WriteTo(f); err != nil {
		t.Fatal(err)
	}
}

func TestMerge_NamespacesAndPreserves(t *testing.T) {
	dir := t.TempDir()

	// Source MMDB to merge in.
	srcPath := filepath.Join(dir, "src.mmdb")
	writeMMDB(t, srcPath, map[string]mmdbtype.Map{
		"1.2.3.0/24": {
			"country_code": mmdbtype.String("US"),
			"asn":          mmdbtype.Uint32(15169),
		},
	})

	// Destination tree with pre-existing data at an overlapping network.
	tree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test", RecordSize: 28})
	if err != nil {
		t.Fatal(err)
	}
	_, network, _ := net.ParseCIDR("1.2.3.0/24")
	if err := tree.Insert(network, mmdbtype.Map{"existing": mmdbtype.String("keep me")}); err != nil {
		t.Fatal(err)
	}

	n, err := Merge(tree, srcPath, "vendor")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if n != 1 {
		t.Errorf("merged = %d, want 1", n)
	}

	// Read back.
	outPath := filepath.Join(dir, "out.mmdb")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.WriteTo(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	db, err := maxminddb.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var result map[string]any
	if err := db.Lookup(net.ParseIP("1.2.3.4"), &result); err != nil {
		t.Fatal(err)
	}

	// Pre-existing top-level data is preserved.
	if result["existing"] != "keep me" {
		t.Errorf("expected existing data preserved, got %v", result["existing"])
	}

	// Merged data is namespaced under "vendor".
	vendor, ok := result["vendor"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'vendor' key, got %v", result)
	}
	if vendor["country_code"] != "US" {
		t.Errorf("vendor.country_code = %v, want US", vendor["country_code"])
	}
	if asn, _ := vendor["asn"].(uint64); asn != 15169 {
		t.Errorf("vendor.asn = %v, want 15169", vendor["asn"])
	}
}
