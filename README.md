# go-mmdb-extender

A tool that enriches MaxMind MMDB databases with additional data sources. Currently supports enrichment from ICANN's Centralized Zone Data Service (CZDS), adding domain-to-IP mapping data from TLD zone files.

## What it does

For each IP address found in CZDS zone file A/AAAA records, the tool adds:

- `czds.domain_count` -- number of domains pointing to the IP
- `czds.zones` -- list of TLD zones containing records for the IP
- `czds.domains` -- list of domain names resolving to the IP

This data is merged with the existing MMDB records (e.g., ASN data from GeoLite2-ASN), preserving all original fields.

## Installation

```sh
go install github.com/msadministrator/go-mmdb-extender/cmd/go-mmdb-extender@latest
go install github.com/msadministrator/go-mmdb-extender/cmd/mmdb-test@latest
```

Or build from source:

```sh
git clone https://github.com/msadministrator/go-mmdb-extender.git
cd go-mmdb-extender
go build ./cmd/go-mmdb-extender
go build ./cmd/mmdb-test
```

## Usage

### Download and enrich with CZDS data

Requires an [ICANN CZDS account](https://czds.icann.org/).

```sh
# Via flags
./go-mmdb-extender \
  --input GeoLite2-ASN.mmdb \
  --output extended.mmdb \
  --czds-username you@example.com \
  --czds-password yourpassword

# Via environment variables
export CZDS_USERNAME=you@example.com
export CZDS_PASSWORD=yourpassword
./go-mmdb-extender --input GeoLite2-ASN.mmdb --output extended.mmdb
```

Zone files are cached in `./czds-data/` (configurable with `--czds-data-dir`) and reused on subsequent runs.

### Process only specific zones

```sh
./go-mmdb-extender \
  --input GeoLite2-ASN.mmdb \
  --output extended.mmdb \
  --czds-zones "com,net,org"
```

### Process cached zone files without API credentials

If zone files have already been downloaded, use `--czds-local-only` to process them without contacting the CZDS API:

```sh
./go-mmdb-extender \
  --input GeoLite2-ASN.mmdb \
  --output extended.mmdb \
  --czds-local-only
```

### Inspect the output

```sh
./mmdb-test extended.mmdb
```

This prints database metadata, iterates all networks to count CZDS-enriched records, shows sample records, and performs a direct IP lookup.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(required)* | Path to input MMDB database file |
| `--output` | *(required)* | Path to output extended MMDB file |
| `--czds-username` | `$CZDS_USERNAME` | CZDS account username |
| `--czds-password` | `$CZDS_PASSWORD` | CZDS account password |
| `--czds-zones` | all available | Comma-separated list of zones to process |
| `--czds-data-dir` | `./czds-data` | Directory for cached zone files |
| `--czds-local-only` | `false` | Process cached zone files without calling the CZDS API |

## Adding new enrichment sources

Implement the `source.Source` interface:

```go
type Source interface {
    Name() string
    Enrich(tree *mmdbwriter.Tree) error
}
```

Use `inserter.TopLevelMergeWith` when inserting records to preserve existing data in the MMDB tree.

## Testing

```sh
go test ./...
```
