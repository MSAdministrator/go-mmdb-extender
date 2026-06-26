package ipinfo

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/msadministrator/go-mmdb-extender/internal/source"
	"github.com/oschwald/maxminddb-golang"
)

func TestFactory(t *testing.T) {
	// Recover the registered factory from the registry rather than reaching
	// into package internals, so this also exercises self-registration.
	registered := false
	for _, n := range source.Registered() {
		if n == "ipinfo" {
			registered = true
		}
	}
	if !registered {
		t.Fatal("ipinfo source did not self-register")
	}

	// Missing path -> error.
	if _, err := source.Build(map[string]map[string]any{"ipinfo": {}}); err == nil {
		t.Error("expected error when path is missing")
	}

	// Valid path -> a source named "ipinfo".
	srcs, err := source.Build(map[string]map[string]any{"ipinfo": {"path": "x.mmdb"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(srcs) != 1 || srcs[0].Name() != "ipinfo" {
		t.Fatalf("got %v, want single source named ipinfo", srcs)
	}
}

func TestEnrich_MergesUnderIPinfoKey(t *testing.T) {
	dir := t.TempDir()

	// Build a stand-in IPinfo MMDB.
	ipinfoPath := filepath.Join(dir, "ipinfo.mmdb")
	srcTree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test", RecordSize: 28})
	if err != nil {
		t.Fatal(err)
	}
	_, network, _ := net.ParseCIDR("8.8.8.0/24")
	if err := srcTree.Insert(network, mmdbtype.Map{
		"country_code": mmdbtype.String("US"),
		"asn":          mmdbtype.String("AS15169"),
	}); err != nil {
		t.Fatal(err)
	}
	writeTree(t, srcTree, ipinfoPath)

	// Destination tree with pre-existing data on the same network.
	tree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test", RecordSize: 28})
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Insert(network, mmdbtype.Map{"existing": mmdbtype.Bool(true)}); err != nil {
		t.Fatal(err)
	}

	if err := New(ipinfoPath).Enrich(tree); err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	outPath := filepath.Join(dir, "out.mmdb")
	writeTree(t, tree, outPath)

	db, err := maxminddb.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var result map[string]any
	if err := db.Lookup(net.ParseIP("8.8.8.8"), &result); err != nil {
		t.Fatal(err)
	}

	if result["existing"] != true {
		t.Errorf("expected pre-existing data preserved, got %v", result["existing"])
	}
	ipinfo, ok := result["ipinfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected ipinfo key, got %v", result)
	}
	if ipinfo["country_code"] != "US" {
		t.Errorf("ipinfo.country_code = %v, want US", ipinfo["country_code"])
	}
	if ipinfo["asn"] != "AS15169" {
		t.Errorf("ipinfo.asn = %v, want AS15169", ipinfo["asn"])
	}
}

func TestEnrich_RealSample(t *testing.T) {
	// Merge the embedded real IPinfo location sample into a fresh tree and
	// verify a known record round-trips under the "ipinfo" key.
	tree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test", RecordSize: 28})
	if err != nil {
		t.Fatal(err)
	}

	if err := New(writeSampleMMDB(t)).Enrich(tree); err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "out.mmdb")
	writeTree(t, tree, outPath)

	db, err := maxminddb.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var result map[string]any
	if err := db.Lookup(net.ParseIP("1.0.0.1"), &result); err != nil {
		t.Fatal(err)
	}
	ipinfo, ok := result["ipinfo"].(map[string]any)
	if !ok {
		t.Fatalf("expected ipinfo key for 1.0.0.1, got %v", result)
	}
	if ipinfo["country_code"] != "AU" {
		t.Errorf("ipinfo.country_code = %v, want AU", ipinfo["country_code"])
	}
	if ipinfo["city"] != "Brisbane" {
		t.Errorf("ipinfo.city = %v, want Brisbane", ipinfo["city"])
	}
}

func TestEnrich_OntoGeoLiteBase(t *testing.T) {
	// The real-world flow: load a GeoLite2-ASN base DB, then enrich it with
	// IPinfo data. Both data sets should coexist on overlapping networks.
	tree, err := mmdbwriter.Load(writeGeoLiteSample(t), mmdbwriter.Options{RecordSize: 28})
	if err != nil {
		t.Fatalf("Load base: %v", err)
	}

	if err := New(writeSampleMMDB(t)).Enrich(tree); err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "out.mmdb")
	writeTree(t, tree, outPath)

	db, err := maxminddb.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 1.0.0.1 exists in the GeoLite base (CLOUDFLARENET) and in the IPinfo
	// sample, so the merged record must carry both the base ASN fields and the
	// namespaced ipinfo block.
	var result map[string]any
	if err := db.Lookup(net.ParseIP("1.0.0.1"), &result); err != nil {
		t.Fatal(err)
	}

	if org, _ := result["autonomous_system_organization"].(string); org != "CLOUDFLARENET" {
		t.Errorf("base autonomous_system_organization = %v, want CLOUDFLARENET", result["autonomous_system_organization"])
	}
	if asn, _ := result["autonomous_system_number"].(uint64); asn != 13335 {
		t.Errorf("base autonomous_system_number = %v, want 13335", result["autonomous_system_number"])
	}
	if _, ok := result["ipinfo"].(map[string]any); !ok {
		t.Errorf("expected ipinfo enrichment alongside base ASN data, got %v", result)
	}
}

func TestEnrich_MissingFile(t *testing.T) {
	tree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test", RecordSize: 28})
	if err != nil {
		t.Fatal(err)
	}
	if err := New("/nonexistent/path.mmdb").Enrich(tree); err == nil {
		t.Fatal("expected error for missing IPinfo file, got nil")
	}
}

func writeTree(t *testing.T, tree *mmdbwriter.Tree, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := tree.WriteTo(f); err != nil {
		t.Fatal(err)
	}
}
