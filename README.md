# keycloak-bulk-user-to-group

A CLI tool that bulk-adds users to a Keycloak group from a CSV file.

## Installation

```bash
go install github.com/Mtze/keycloak-bulk-user-to-group@latest
```

Or download a pre-built binary from the [releases page](https://github.com/Mtze/keycloak-bulk-user-to-group/releases).

## Configuration

Copy `config.example.yaml` to `config.yaml` and fill in your values:

```bash
cp config.example.yaml config.yaml
```

| Key | Description | Default |
| --- | ----------- | ------- |
| `server_url` | Keycloak base URL | - |
| `realm` | Realm name | - |
| `client_id` | Service account client ID | - |
| `client_secret` | Client secret | - |
| `group_path` | Name of the group to add users to | - |
| `username_column` | Header name of the username column | `username` |

Every key can also be set via environment variable with a `KC_` prefix (e.g. `KC_CLIENT_SECRET`). Environment variables take precedence over the config file.

## Usage

```bash
keycloak-bulk-user-to-group <csv-file>
```

The CSV file path is a required positional argument. The tool expects a semicolon-delimited CSV with a header row. It reads the column specified by `username_column`, looks up each user in Keycloak by exact username match, and adds them to the configured group.

## Credentials

Keep `config.yaml` out of version control (it is in `.gitignore`). Pass `client_secret` via the `KC_CLIENT_SECRET` environment variable in CI or automated environments.

## Development

### Prerequisites

- Go 1.24+
- A running Keycloak instance (or access to one) for manual testing

### Build

```bash
go build -o keycloak-bulk-user-to-group .
```

### Run locally

```bash
cp config.example.yaml config.yaml
# fill in config.yaml or export KC_* env vars
go run . 
```

### Project structure

```text
.
├── main.go                  # entry point and all tool logic
├── config.example.yaml      # annotated config template
├── go.mod / go.sum
├── .goreleaser.yaml          # cross-platform release config
└── .github/workflows/
    └── release.yml           # publishes a release on every v* tag
```

### Releasing

Releases are published automatically by GoReleaser when a `v*` tag is pushed:

```bash
git tag v1.2.3
git push origin v1.2.3
```

The workflow builds binaries for Linux, macOS, and Windows (amd64 + arm64), packages them as `.tar.gz`/`.zip` archives, and attaches a `checksums.txt` to the GitHub release.

To test the release process locally (requires [GoReleaser](https://goreleaser.com/install/)):

```bash
goreleaser release --snapshot --clean
```
