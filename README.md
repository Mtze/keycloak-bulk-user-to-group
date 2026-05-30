# keycloak-bulk-user-to-group

A CLI tool to bulk-manage Keycloak groups and group memberships from a CSV file.

## Installation

```bash
go install github.com/Mtze/keycloak-bulk-user-to-group@latest
```

Or download a pre-built binary from the [releases page](https://github.com/Mtze/keycloak-bulk-user-to-group/releases).

## Keycloak setup

The tool authenticates as a **service account** (client credentials grant). You need to create a dedicated confidential client in your realm:

1. In the Keycloak Admin Console go to your realm - **Clients** - **Create client**
2. Set **Client type** to `OpenID Connect`, choose a **Client ID** (e.g. `bulk-importer`), click Next
3. Enable **Client authentication** (makes it a confidential client), disable **Standard flow** and **Direct access grants**, enable **Service accounts roles**, click Save
4. On the **Credentials** tab copy the **Client secret** - this is your `client_secret`
5. On the **Service account roles** tab click **Assign role**, filter by **realm-management**, and assign:
   - `query-groups` - required by both subcommands to search and list groups
   - `manage-users` - required for `add-users` (user lookup + adding members to groups) and for `create-groups` (creating subgroups)

The `server_url` is the base URL of your Keycloak instance (e.g. `https://keycloak.example.com`), without a trailing slash and without `/realms/...`.

## Configuration

Copy `config.example.yaml` to `config.yaml` and fill in your values:

```bash
cp config.example.yaml config.yaml
```

| Key | Used by | Description | Default |
|-----|---------|-------------|---------|
| `server_url` | both | Keycloak base URL | - |
| `realm` | both | Realm name | - |
| `client_id` | both | Service account client ID | - |
| `client_secret` | both | Client secret | - |
| `group_path` | add-users | Target group name | - |
| `username_column` | add-users, assign-users | CSV column header containing usernames | `username` |
| `parent_group` | create-groups | Existing group under which to create subgroups | - |
| `group_name_column` | create-groups, assign-users | CSV column header containing group names | - |
| `group_prefix` | create-groups, assign-users | Prefix prepended to every group name | `""` |

Every key can also be set via environment variable with the `KC_` prefix (e.g. `KC_CLIENT_SECRET`). Environment variables take precedence over the config file.

Keep `config.yaml` out of version control - it is already in `.gitignore`. In CI or automated environments, pass credentials via `KC_CLIENT_SECRET` (and other `KC_*` vars) instead of a config file.

## Usage

```
keycloak-bulk-user-to-group <subcommand> [flags] <csv-file>
```

Flags must appear **before** the CSV file argument. The tool expects a **semicolon-delimited CSV** with a header row.

### add-users

Look up users from a CSV column by exact username match and add them to a Keycloak group.

```bash
keycloak-bulk-user-to-group add-users \
  --group "ws25-my-group" \
  --col "University Login" \
  students.csv
```

| Flag | Description | Default |
|------|-------------|---------|
| `--group` | Target group name | config `group_path` |
| `--col` | CSV column containing usernames | config `username_column` |

Users that cannot be found in Keycloak are logged and skipped; the run continues.

### assign-users

Add each user to the group named in their own row - useful after running `create-groups` to populate the groups.

```bash
keycloak-bulk-user-to-group assign-users \
  --username-col "University Login" \
  --group-col "Allocated Team" \
  --prefix "devops26-team-" \
  students.csv
```

| Flag | Description | Default |
|------|-------------|---------|
| `--username-col` | CSV column containing usernames | config `username_column` |
| `--group-col` | CSV column containing group names | config `group_name_column` |
| `--prefix` | Prefix prepended to each group name before lookup | config `group_prefix` |

Group names are sanitized the same way as in `create-groups` (lowercased, spaces to dashes, non-alphanumeric characters removed). Group lookups are cached so each unique group is only fetched once. Users that cannot be found and groups that do not exist are logged and skipped; a summary is printed at the end.

### create-groups

Read group names from a CSV column, optionally prepend a prefix, and create them as subgroups of a parent group. Groups that already exist are skipped.

Before making any changes the tool prints a plan and asks for confirmation:

```
Parent group: my-parent (id: abc-123)

Groups to create (3):
  + ws25-alpha-team
  + ws25-beta-team
  + ws25-gamma-team

Groups already exist, will skip (1):
  ~ ws25-delta-team

Proceed? [y/N]:
```

```bash
keycloak-bulk-user-to-group create-groups \
  --parent "my-parent" \
  --prefix "ws25-" \
  --col "Team Name" \
  students.csv
```

| Flag | Description | Default |
|------|-------------|---------|
| `--parent` | Parent group name | config `parent_group` |
| `--prefix` | Prefix prepended to each group name | config `group_prefix` |
| `--col` | CSV column containing group names | config `group_name_column` |
| `--yes` | Skip confirmation prompt (for scripted use) | `false` |

## CSV format

The tool reads any semicolon-delimited CSV with a header row. Only the column specified by `--col` is used; all other columns are ignored.

Example:

```
"First Name";"Last Name";"Username";"Team"
"Alice";"Smith";"asmith";"Alpha Team"
"Bob";"Jones";"bjones";"Beta Team"
```

## Development

Prerequisites: Go 1.24+

```bash
# build
go build -o keycloak-bulk-user-to-group .

# run directly
go run . <subcommand> <csv-file> [flags]

# debug output
DEBUG=true go run . ...
```

### Releasing

Releases are published automatically by GoReleaser when a `v*` tag is pushed:

```bash
git tag v1.2.3
git push origin v1.2.3
```

Binaries are built for Linux, macOS, and Windows (amd64 + arm64). To test the release process locally:

```bash
goreleaser release --snapshot --clean
```
