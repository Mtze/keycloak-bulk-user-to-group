# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o keycloak-bulk-user-to-group .
go run . <subcommand> [flags] <csv-file>
```

Flags must appear before the positional CSV file argument (Go `flag` package stops parsing at the first non-flag argument).

There are no tests. The only way to verify behaviour is to run against a real Keycloak instance.

Release builds (cross-platform) use GoReleaser:
```bash
goreleaser release --snapshot --clean
```

## Architecture

Everything lives in `main.go` - single file, no packages. The flow is:

1. `main()` - reads config via Viper, dispatches to `runAddUsers`, `runAssignUsers`, or `runCreateGroups`
2. `keycloakConnect()` - authenticates via service account (`LoginClient`), returns `*gocloak.GoCloak` and access token
3. Subcommand runners use `flag.FlagSet` where CLI flags override config-file values

### Subcommands

**`add-users [--group] [--col] <csv-file>`**  
Looks up each username from the CSV column in Keycloak (exact match) and calls `AddUserToGroup`. Non-fatal per-user errors are logged and skipped.

**`assign-users [--username-col] [--group-col] [--prefix] <csv-file>`**  
Reads username and group name from each row, sanitizes the group name, and adds each user to their respective group. Group ID lookups are cached (one API call per unique group). Non-fatal per-row errors are logged and skipped; prints a summary at the end.

**`create-groups [--parent] [--prefix] [--col] [--yes] <csv-file>`**  
Reads unique group names from a CSV column, sanitizes them, prepends `--prefix`, compares against existing subgroups of the parent (fetched via `GetGroup`), then prints a terraform-plan-style diff and prompts for confirmation before calling `CreateChildGroup` for each new group. `--yes` skips the prompt.

### Key patterns

- `sanitizeGroupName(name)` - lowercases, replaces spaces with dashes, strips everything outside `[a-z0-9-]`. Applied in both `create-groups` and `assign-users` before any Keycloak interaction.
- `findGroupByName(groups, name)` - recursive exact-name search across top-level groups and their `SubGroups`
- CSV parsing: semicolon delimiter, `LazyQuotes = true`, header row for column index lookup
- Config: Viper with `config.yaml` + `KC_` env var prefix; env vars take precedence

### Config keys

| Key | Subcommand | CLI flag |
|-----|------------|----------|
| `server_url`, `realm`, `client_id`, `client_secret` | all | - |
| `group_path` | add-users | `--group` |
| `username_column` | add-users, assign-users | `--col` / `--username-col` |
| `parent_group` | create-groups | `--parent` |
| `group_name_column` | create-groups, assign-users | `--col` / `--group-col` |
| `group_prefix` | create-groups, assign-users | `--prefix` |

`config.yaml` is gitignored. Use `config.example.yaml` as the template. Pass `client_secret` via `KC_CLIENT_SECRET` in CI.

Enable debug logging: `DEBUG=true go run . ...`
