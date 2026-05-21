# keycloak-user-to-group

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
| `csv_file` | Path to the CSV file | - |
| `username_column` | Header name of the username column | `username` |

Every key can also be set via environment variable with a `KC_` prefix (e.g. `KC_CLIENT_SECRET`). Environment variables take precedence over the config file.

## Usage

```bash
keycloak-user-to-group
```

The tool expects a semicolon-delimited CSV with a header row. It reads the column specified by `username_column`, looks up each user in Keycloak by exact username match, and adds them to the configured group.

## Credentials

Keep `config.yaml` out of version control (it is in `.gitignore`). Pass `client_secret` via the `KC_CLIENT_SECRET` environment variable in CI or automated environments.
