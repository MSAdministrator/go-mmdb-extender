package czds

import (
	"compress/gzip"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	czdslib "github.com/lanrat/czds"
	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/miekg/dns"
	"github.com/msadministrator/go-mmdb-extender/internal/source"
)

func init() {
	source.Register("czds", factory)
}

// factory builds a CZDS source from its config block. Recognized keys:
//
//	username    string   CZDS account username (falls back to $CZDS_USERNAME)
//	password    string   CZDS account password (falls back to $CZDS_PASSWORD)
//	zones       []string specific zones to process (default: all available)
//	data_dir    string   directory for cached zone files (default: ./czds-data)
//	local_only  bool     process cached zone files without calling the API
//
// It returns (nil, nil) when neither credentials nor local_only are configured,
// so an empty/placeholder czds block simply disables the source.
func factory(cfg map[string]any) (source.Source, error) {
	username, _ := cfg["username"].(string)
	password, _ := cfg["password"].(string)
	if username == "" {
		username = os.Getenv("CZDS_USERNAME")
	}
	if password == "" {
		password = os.Getenv("CZDS_PASSWORD")
	}

	localOnly, _ := cfg["local_only"].(bool)

	dataDir, _ := cfg["data_dir"].(string)
	if dataDir == "" {
		dataDir = "./czds-data"
	}

	var zones []string
	if raw, ok := cfg["zones"].([]any); ok {
		for _, z := range raw {
			if s, ok := z.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					zones = append(zones, trimmed)
				}
			}
		}
	}

	if !localOnly && (username == "" || password == "") {
		return nil, nil
	}

	return New(username, password, zones, dataDir, localOnly), nil
}

// Source enriches an MMDB with data from ICANN's Centralized Zone Data Service.
// For each IP found in zone file A/AAAA records, it adds:
//   - czds.domain_count: number of domains pointing to the IP
//   - czds.zones: list of TLD zones containing records for the IP
type Source struct {
	username  string
	password  string
	zones     []string // specific zones to process; empty means all available
	dataDir   string   // directory for downloaded/cached zone files
	localOnly bool     // if true, process cached zone files without calling the API
}

// New creates a CZDS source. If zones is empty, all zones the user has access
// to will be downloaded. Zone files are cached in dataDir and reused on
// subsequent runs. If localOnly is true, the source will process all cached
// zone files from dataDir without contacting the CZDS API.
func New(username, password string, zones []string, dataDir string, localOnly bool) *Source {
	return &Source{
		username:  username,
		password:  password,
		zones:     zones,
		dataDir:   dataDir,
		localOnly: localOnly,
	}
}

func (s *Source) Name() string { return "czds" }

// ipEnrichment accumulates per-IP data across all processed zone files.
type ipEnrichment struct {
	domainCount uint32
	zones       map[string]struct{}
	domains     []string
}

func (s *Source) Enrich(tree *mmdbwriter.Tree) error {
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	type zoneJob struct {
		zone      string
		localPath string
	}
	var jobs []zoneJob

	if s.localOnly {
		// Scan the data directory for cached zone files.
		entries, err := os.ReadDir(s.dataDir)
		if err != nil {
			return fmt.Errorf("reading data directory: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".zone.gz") {
				continue
			}
			zone := strings.TrimSuffix(e.Name(), ".zone.gz")
			jobs = append(jobs, zoneJob{zone: zone, localPath: filepath.Join(s.dataDir, e.Name())})
		}
		log.Printf("CZDS: found %d cached zone files in %s", len(jobs), s.dataDir)

		if len(s.zones) > 0 {
			want := make(map[string]struct{}, len(s.zones))
			for _, z := range s.zones {
				want[strings.ToLower(strings.TrimSpace(z))] = struct{}{}
			}
			var filtered []zoneJob
			for _, j := range jobs {
				if _, ok := want[j.zone]; ok {
					filtered = append(filtered, j)
				}
			}
			jobs = filtered
			log.Printf("CZDS: filtered to %d requested zones", len(jobs))
		}
	} else {
		client := czdslib.NewClient(s.username, s.password)

		links, err := client.GetLinks()
		if err != nil {
			return fmt.Errorf("getting CZDS download links: %w", err)
		}
		log.Printf("CZDS: %d zones available", len(links))

		if len(s.zones) > 0 {
			links = filterLinks(links, s.zones)
			log.Printf("CZDS: filtered to %d requested zones", len(links))
		}

		for _, link := range links {
			zone := zoneFromLink(link)
			localPath := filepath.Join(s.dataDir, zone+".zone.gz")

			if _, err := os.Stat(localPath); os.IsNotExist(err) {
				log.Printf("CZDS: downloading zone %s", zone)
				if err := client.DownloadZone(link, localPath); err != nil {
					log.Printf("CZDS: skipping %s: download failed: %v", zone, err)
					continue
				}
			} else {
				log.Printf("CZDS: using cached zone file for %s", zone)
			}

			jobs = append(jobs, zoneJob{zone: zone, localPath: localPath})
		}
	}

	enrichment := make(map[netip.Addr]*ipEnrichment)

	for _, j := range jobs {
		log.Printf("CZDS: parsing zone %s", j.zone)
		if err := parseZoneFile(j.localPath, j.zone, enrichment); err != nil {
			log.Printf("CZDS: skipping %s: parse failed: %v", j.zone, err)
			continue
		}
	}

	log.Printf("CZDS: inserting enrichment data for %d IPs into MMDB", len(enrichment))
	for addr, data := range enrichment {
		network := addrToIPNet(addr)

		zones := make(mmdbtype.Slice, 0, len(data.zones))
		for z := range data.zones {
			zones = append(zones, mmdbtype.String(z))
		}

		domains := make(mmdbtype.Slice, 0, len(data.domains))
		for _, d := range data.domains {
			domains = append(domains, mmdbtype.String(d))
		}

		record := mmdbtype.Map{
			"czds": mmdbtype.Map{
				"domain_count": mmdbtype.Uint32(data.domainCount),
				"zones":        zones,
				"domains":      domains,
			},
		}

		if err := tree.InsertFunc(network, inserter.TopLevelMergeWith(record)); err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "aliased network") || strings.Contains(errMsg, "reserved network") {
				continue
			}
			return fmt.Errorf("inserting enrichment for %s: %w", addr, err)
		}
	}

	return nil
}

// parseZoneFile reads a gzipped zone file and extracts A/AAAA records into the
// enrichment map. It streams the file to keep memory usage proportional to the
// enrichment map, not the zone file size.
func parseZoneFile(path, zone string, enrichment map[netip.Addr]*ipEnrichment) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("decompressing %s: %w", path, err)
	}
	defer gz.Close()

	zp := dns.NewZoneParser(gz, zone+".", path)

	var count int
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		var addr netip.Addr
		var domain string
		switch r := rr.(type) {
		case *dns.A:
			if r.A == nil {
				continue
			}
			a4 := r.A.To4()
			if a4 == nil {
				continue
			}
			addr, _ = netip.AddrFromSlice(a4)
			domain = strings.TrimSuffix(r.Hdr.Name, ".")
		case *dns.AAAA:
			if r.AAAA == nil {
				continue
			}
			addr, _ = netip.AddrFromSlice(r.AAAA.To16())
			domain = strings.TrimSuffix(r.Hdr.Name, ".")
		default:
			continue
		}

		if !addr.IsValid() {
			continue
		}

		// Convert v4-mapped v6 addresses (::ffff:x.x.x.x) to plain v4.
		// These would otherwise collide with aliased networks in the MMDB.
		addr = addr.Unmap()

		// Skip unroutable/special addresses (0.0.0.0, loopback, etc.)
		if !addr.IsGlobalUnicast() {
			continue
		}

		data, ok := enrichment[addr]
		if !ok {
			data = &ipEnrichment{zones: make(map[string]struct{})}
			enrichment[addr] = data
		}
		data.domainCount++
		data.zones[zone] = struct{}{}
		data.domains = append(data.domains, domain)
		count++
	}

	if err := zp.Err(); err != nil {
		return fmt.Errorf("parsing zone %s: %w", zone, err)
	}

	log.Printf("CZDS: zone %s: extracted %d A/AAAA records", zone, count)
	return nil
}

// filterLinks returns only the download links whose zone name matches one of
// the requested zones.
func filterLinks(links []string, zones []string) []string {
	want := make(map[string]struct{}, len(zones))
	for _, z := range zones {
		want[strings.ToLower(strings.TrimSpace(z))] = struct{}{}
	}

	var filtered []string
	for _, link := range links {
		if _, ok := want[zoneFromLink(link)]; ok {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

// zoneFromLink extracts the zone name from a CZDS download URL.
// Example: "https://czds-api.icann.org/czds/downloads/aaa.zone" -> "aaa"
func zoneFromLink(link string) string {
	parts := strings.Split(link, "/")
	name := parts[len(parts)-1]
	return strings.TrimSuffix(name, ".zone")
}

// addrToIPNet converts a single IP address to a host-sized *net.IPNet
// (/32 for IPv4, /128 for IPv6).
func addrToIPNet(addr netip.Addr) *net.IPNet {
	ip := net.IP(addr.AsSlice())
	if addr.Is4() {
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}
