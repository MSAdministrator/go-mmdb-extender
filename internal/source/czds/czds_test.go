package czds

import (
	"compress/gzip"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/oschwald/maxminddb-golang"
)

// writeTestZoneFile creates a gzipped zone file at path with the given zone
// content. The content should be valid DNS zone file text.
func writeTestZoneFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestParseZoneFile_ARecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.zone.gz")
	zone := "example"

	content := `$ORIGIN example.
@ 3600 IN SOA ns1.example. admin.example. 2024010101 3600 900 604800 86400
@ 3600 IN NS ns1.example.
ns1   3600 IN A    93.184.216.34
www   3600 IN A    93.184.216.34
loop  3600 IN A    127.0.0.1
other 3600 IN AAAA 2606:4700:4700::1111
`
	writeTestZoneFile(t, path, content)

	enrichment := make(map[netip.Addr]*ipEnrichment)
	if err := parseZoneFile(path, zone, enrichment); err != nil {
		t.Fatalf("parseZoneFile: %v", err)
	}

	// 127.0.0.1 is loopback — filtered by IsGlobalUnicast.
	// 93.184.216.34 should have 2 records, 2606:4700:4700::1111 should have 1.
	addr := netip.MustParseAddr("93.184.216.34")
	data, ok := enrichment[addr]
	if !ok {
		t.Fatalf("expected enrichment for %s, got none", addr)
	}

	if data.domainCount != 2 {
		t.Errorf("domain_count = %d, want 2", data.domainCount)
	}
	if _, ok := data.zones["example"]; !ok {
		t.Errorf("zones should contain 'example', got %v", data.zones)
	}
	if len(data.domains) != 2 {
		t.Errorf("domains len = %d, want 2", len(data.domains))
	}

	// 127.0.0.1 is loopback — should be excluded
	loopAddr := netip.MustParseAddr("127.0.0.1")
	if _, ok := enrichment[loopAddr]; ok {
		t.Error("loopback address 127.0.0.1 should be filtered out")
	}
}

func TestParseZoneFile_AAAARecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "v6test.zone.gz")
	zone := "v6test"

	content := `$ORIGIN v6test.
@ 3600 IN SOA ns1.v6test. admin.v6test. 2024010101 3600 900 604800 86400
@ 3600 IN NS ns1.v6test.
ns1  3600 IN AAAA 2606:4700:4700::1111
www  3600 IN AAAA 2606:4700:4700::1111
`
	writeTestZoneFile(t, path, content)

	enrichment := make(map[netip.Addr]*ipEnrichment)
	if err := parseZoneFile(path, zone, enrichment); err != nil {
		t.Fatalf("parseZoneFile: %v", err)
	}

	addr := netip.MustParseAddr("2606:4700:4700::1111")
	data, ok := enrichment[addr]
	if !ok {
		t.Fatalf("expected enrichment for %s", addr)
	}
	if data.domainCount != 2 {
		t.Errorf("domain_count = %d, want 2", data.domainCount)
	}
}

func TestParseZoneFile_AccumulatesAcrossZones(t *testing.T) {
	dir := t.TempDir()
	ip := "93.184.216.34"

	zone1Content := `$ORIGIN zone1.
@ 3600 IN SOA ns.zone1. admin.zone1. 1 3600 900 604800 86400
@ 3600 IN NS ns.zone1.
app 3600 IN A ` + ip + `
`
	zone2Content := `$ORIGIN zone2.
@ 3600 IN SOA ns.zone2. admin.zone2. 1 3600 900 604800 86400
@ 3600 IN NS ns.zone2.
web 3600 IN A ` + ip + `
api 3600 IN A ` + ip + `
`
	path1 := filepath.Join(dir, "zone1.zone.gz")
	path2 := filepath.Join(dir, "zone2.zone.gz")
	writeTestZoneFile(t, path1, zone1Content)
	writeTestZoneFile(t, path2, zone2Content)

	enrichment := make(map[netip.Addr]*ipEnrichment)
	if err := parseZoneFile(path1, "zone1", enrichment); err != nil {
		t.Fatal(err)
	}
	if err := parseZoneFile(path2, "zone2", enrichment); err != nil {
		t.Fatal(err)
	}

	addr := netip.MustParseAddr(ip)
	data := enrichment[addr]
	if data.domainCount != 3 {
		t.Errorf("domain_count = %d, want 3", data.domainCount)
	}
	if len(data.zones) != 2 {
		t.Errorf("zones count = %d, want 2", len(data.zones))
	}
	if len(data.domains) != 3 {
		t.Errorf("domains len = %d, want 3", len(data.domains))
	}
}

func TestParseZoneFile_SkipsNonGlobalUnicast(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "special.zone.gz")

	content := `$ORIGIN special.
@ 3600 IN SOA ns.special. admin.special. 1 3600 900 604800 86400
@ 3600 IN NS ns.special.
loopback  3600 IN A    127.0.0.1
zero      3600 IN A    0.0.0.0
linklocal 3600 IN AAAA fe80::1
multicast 3600 IN AAAA ff02::1
`
	writeTestZoneFile(t, path, content)

	enrichment := make(map[netip.Addr]*ipEnrichment)
	if err := parseZoneFile(path, "special", enrichment); err != nil {
		t.Fatal(err)
	}

	// All of these are non-global-unicast and should be filtered out
	if len(enrichment) != 0 {
		for addr := range enrichment {
			t.Errorf("unexpected enrichment for non-global-unicast address %s", addr)
		}
	}
}

func TestEnrich_LocalOnly(t *testing.T) {
	dir := t.TempDir()

	zone1Content := `$ORIGIN testzone.
@ 3600 IN SOA ns.testzone. admin.testzone. 1 3600 900 604800 86400
@ 3600 IN NS ns.testzone.
www 3600 IN A 93.184.216.34
`
	zone2Content := `$ORIGIN other.
@ 3600 IN SOA ns.other. admin.other. 1 3600 900 604800 86400
@ 3600 IN NS ns.other.
app 3600 IN A 104.21.32.1
`
	writeTestZoneFile(t, filepath.Join(dir, "testzone.zone.gz"), zone1Content)
	writeTestZoneFile(t, filepath.Join(dir, "other.zone.gz"), zone2Content)
	// Write a non-zone file that should be ignored.
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644)

	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "Test",
		RecordSize:   28,
	})
	if err != nil {
		t.Fatal(err)
	}

	src := New("", "", nil, dir, true)
	if err := src.Enrich(tree); err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	// Write to a temp file and read back with the reader to verify records.
	mmdbPath := filepath.Join(dir, "test.mmdb")
	f, err := os.Create(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.WriteTo(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	db, err := maxminddb.Open(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Check 93.184.216.34 has czds data from testzone
	var result map[string]any
	if err := db.Lookup(net.ParseIP("93.184.216.34"), &result); err != nil {
		t.Fatal(err)
	}
	czdsData, ok := result["czds"].(map[string]any)
	if !ok {
		t.Fatalf("expected czds key in result, got %v", result)
	}
	if count, _ := czdsData["domain_count"].(uint64); count != 1 {
		t.Errorf("domain_count = %v, want 1", czdsData["domain_count"])
	}

	// Check 104.21.32.1 has czds data from other
	var result2 map[string]any
	if err := db.Lookup(net.ParseIP("104.21.32.1"), &result2); err != nil {
		t.Fatal(err)
	}
	czdsData2, ok := result2["czds"].(map[string]any)
	if !ok {
		t.Fatalf("expected czds key in result2, got %v", result2)
	}
	if count, _ := czdsData2["domain_count"].(uint64); count != 1 {
		t.Errorf("domain_count = %v, want 1", czdsData2["domain_count"])
	}
}

func TestEnrich_LocalOnlyWithZoneFilter(t *testing.T) {
	dir := t.TempDir()

	zone1Content := `$ORIGIN keep.
@ 3600 IN SOA ns.keep. admin.keep. 1 3600 900 604800 86400
@ 3600 IN NS ns.keep.
www 3600 IN A 93.184.216.34
`
	zone2Content := `$ORIGIN skip.
@ 3600 IN SOA ns.skip. admin.skip. 1 3600 900 604800 86400
@ 3600 IN NS ns.skip.
app 3600 IN A 104.21.32.1
`
	writeTestZoneFile(t, filepath.Join(dir, "keep.zone.gz"), zone1Content)
	writeTestZoneFile(t, filepath.Join(dir, "skip.zone.gz"), zone2Content)

	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "Test",
		RecordSize:   28,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Only process "keep" zone
	src := New("", "", []string{"keep"}, dir, true)
	if err := src.Enrich(tree); err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	mmdbPath := filepath.Join(dir, "test.mmdb")
	f, err := os.Create(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.WriteTo(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	db, err := maxminddb.Open(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 93.184.216.34 should have data (from "keep" zone)
	var result map[string]any
	if err := db.Lookup(net.ParseIP("93.184.216.34"), &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["czds"]; !ok {
		t.Error("expected czds data for 93.184.216.34")
	}

	// 104.21.32.1 should NOT have data (from "skip" zone)
	var result2 map[string]any
	if err := db.Lookup(net.ParseIP("104.21.32.1"), &result2); err != nil {
		t.Fatal(err)
	}
	if _, ok := result2["czds"]; ok {
		t.Error("104.21.32.1 should not have czds data when 'skip' zone is filtered out")
	}
}

func TestEnrich_MergesWithExistingData(t *testing.T) {
	dir := t.TempDir()

	zoneContent := `$ORIGIN merged.
@ 3600 IN SOA ns.merged. admin.merged. 1 3600 900 604800 86400
@ 3600 IN NS ns.merged.
www 3600 IN A 93.184.216.34
`
	writeTestZoneFile(t, filepath.Join(dir, "merged.zone.gz"), zoneContent)

	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "Test",
		RecordSize:   28,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Pre-insert existing data for this IP
	_, network, _ := net.ParseCIDR("93.184.216.34/32")
	existing := mmdbtype.Map{
		"asn": mmdbtype.Uint32(15133),
	}
	if err := tree.Insert(network, existing); err != nil {
		t.Fatal(err)
	}

	src := New("", "", nil, dir, true)
	if err := src.Enrich(tree); err != nil {
		t.Fatalf("Enrich: %v", err)
	}

	mmdbPath := filepath.Join(dir, "test.mmdb")
	f, err := os.Create(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.WriteTo(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	db, err := maxminddb.Open(mmdbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var result map[string]any
	if err := db.Lookup(net.ParseIP("93.184.216.34"), &result); err != nil {
		t.Fatal(err)
	}

	// Both original "asn" and new "czds" keys should be present
	if _, ok := result["asn"]; !ok {
		t.Error("expected 'asn' key to be preserved after merge")
	}
	if _, ok := result["czds"]; !ok {
		t.Error("expected 'czds' key after enrichment")
	}
}

func TestFilterLinks(t *testing.T) {
	links := []string{
		"https://czds-api.icann.org/czds/downloads/com.zone",
		"https://czds-api.icann.org/czds/downloads/net.zone",
		"https://czds-api.icann.org/czds/downloads/org.zone",
	}

	filtered := filterLinks(links, []string{"com", "org"})
	if len(filtered) != 2 {
		t.Fatalf("filtered len = %d, want 2", len(filtered))
	}
}

func TestZoneFromLink(t *testing.T) {
	tests := []struct {
		link string
		want string
	}{
		{"https://czds-api.icann.org/czds/downloads/aaa.zone", "aaa"},
		{"https://czds-api.icann.org/czds/downloads/co.uk.zone", "co.uk"},
		{"https://example.com/foo.zone", "foo"},
	}
	for _, tt := range tests {
		got := zoneFromLink(tt.link)
		if got != tt.want {
			t.Errorf("zoneFromLink(%q) = %q, want %q", tt.link, got, tt.want)
		}
	}
}

func TestAddrToIPNet(t *testing.T) {
	v4 := netip.MustParseAddr("1.2.3.4")
	net4 := addrToIPNet(v4)
	ones, bits := net4.Mask.Size()
	if ones != 32 || bits != 32 {
		t.Errorf("IPv4 mask = /%d (of %d), want /32", ones, bits)
	}

	v6 := netip.MustParseAddr("2001:db8::1")
	net6 := addrToIPNet(v6)
	ones, bits = net6.Mask.Size()
	if ones != 128 || bits != 128 {
		t.Errorf("IPv6 mask = /%d (of %d), want /128", ones, bits)
	}
}
