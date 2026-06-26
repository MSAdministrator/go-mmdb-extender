# go-mmdb-extender

A tool that enriches MaxMind-format MMDB databases from multiple data sources. A base MMDB is loaded, each configured source merges its data in (namespaced under its own top-level key), and an extended MMDB is written out. Original fields are always preserved.

## Sources

| Source | Key | What it adds |
|--------|-----|--------------|
| **ipinfo** | `ipinfo.*` | All fields from an IPinfo MMDB file (e.g. the free [IPinfo Lite database](https://ipinfo.io/developers/ipinfo-lite-database)) — ASN, country, continent, geo, etc., depending on the dataset. |
| **czds** | `czds.*` | Domain-to-IP mapping from ICANN [CZDS](https://czds.icann.org/) zone files: `domain_count`, `zones`, `domains`. |

Sources compose: enable any combination in config and each writes under its own key, so the IPinfo and CZDS data sit side by side on the same record alongside whatever the base MMDB already contained.

### How IPinfo works

IPinfo ships its datasets as MMDB files, so enrichment is a direct MMDB-to-MMDB merge — every network in the IPinfo database is merged into the tree under `ipinfo`. Download the `.mmdb` from IPinfo (a free token is required for Lite) and point the source's `path` at it. This same MMDB-merge engine works for any vendor MMDB.

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

Configuration is driven by a YAML file. Copy [`config.example.yaml`](config.example.yaml) to `config.yaml`, edit it, and run:

```sh
./go-mmdb-extender --config config.yaml
```

`--config` defaults to `config.yaml`.

### Example config

```yaml
input: GeoLite2-ASN.mmdb
output: extended.mmdb
sources:
  ipinfo:
    path: ipinfo_lite.mmdb
  czds:
    zones: [com, net, org]
    data_dir: ./czds-data
    local_only: false
```

Each key under `sources` enables that source. Omit a block to skip it.

### IPinfo source

```yaml
sources:
  ipinfo:
    path: ipinfo_lite.mmdb   # path to any IPinfo MMDB file
```

### CZDS source

Requires an [ICANN CZDS account](https://czds.icann.org/). Credentials may be set inline or via the `CZDS_USERNAME` / `CZDS_PASSWORD` environment variables.

```yaml
sources:
  czds:
    username: you@example.com   # or $CZDS_USERNAME
    password: yourpassword      # or $CZDS_PASSWORD
    zones: [com, net, org]      # omit for all zones you have access to
    data_dir: ./czds-data       # cached zone files, reused on later runs
    local_only: false           # true: process cached files without the API
```

If credentials are absent and `local_only` is false, the CZDS source is silently skipped.

### Inspect the output

```sh
./mmdb-test extended.mmdb
```

This prints database metadata, iterates all networks, shows sample records, and performs a direct IP lookup.

## Config reference

| Key | Default | Description |
|-----|---------|-------------|
| `input` | *(required)* | Path to input MMDB database file |
| `output` | *(required)* | Path to output extended MMDB file |
| `sources.<name>` | *(at least one required)* | Config block for a registered source |
| `sources.ipinfo.path` | *(required)* | Path to an IPinfo MMDB file |
| `sources.czds.username` | `$CZDS_USERNAME` | CZDS account username |
| `sources.czds.password` | `$CZDS_PASSWORD` | CZDS account password |
| `sources.czds.zones` | all available | List of zones to process |
| `sources.czds.data_dir` | `./czds-data` | Directory for cached zone files |
| `sources.czds.local_only` | `false` | Process cached zone files without calling the CZDS API |

## Adding new enrichment sources

Implement the `source.Source` interface and register a factory in the package's `init()`:

```go
func init() {
    source.Register("mysource", func(cfg map[string]any) (source.Source, error) {
        return &Source{ /* read cfg */ }, nil
    })
}

type Source interface {
    Name() string                       // also the top-level MMDB key
    Enrich(tree *mmdbwriter.Tree) error
}
```

Then blank-import the package in `cmd/go-mmdb-extender/main.go`. `main` never names a source directly — the registry wires everything up from the config's `sources` map.

Two conventions keep sources composable:

- **Namespace** all data under a single top-level key matching `Name()`.
- **Merge**, don't overwrite: use `inserter.TopLevelMergeWith` when inserting.

If your source's upstream data is itself an MMDB (most commercial IP datasets), use the `internal/mmdbmerge` helper — `mmdbmerge.Merge(tree, path, key)` does the whole iterate-and-merge in one call (this is exactly how the IPinfo source is built).

## Testing

```sh
go test ./...
```
