package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/Nerzal/gocloak/v13"
	"github.com/spf13/viper"
)

var debug bool

func debugf(format string, args ...any) {
	if debug {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func strVal(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("KC")
	viper.AutomaticEnv()

	viper.SetDefault("username_column", "username")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Fatalf("Error reading config file: %v", err)
		}
		log.Println("No config.yaml found, relying on environment variables.")
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: %s <subcommand> [flags] <csv-file>

Subcommands:
  add-users      Look up users from a CSV column and add them to a Keycloak group.
  create-groups  Read group names from a CSV column and create them under a parent group.

Flags (add-users):
  --group  string   Target group name (default: config 'group_path')
  --col    string   CSV column containing usernames (default: config 'username_column')

Flags (create-groups):
  --parent  string  Parent group name (default: config 'parent_group')
  --prefix  string  Prefix prepended to every group name (default: config 'group_prefix')
  --col     string  CSV column containing group names (default: config 'group_name_column')
  --yes             Skip confirmation prompt

Note: flags must appear before the csv-file argument.

Configuration is loaded from config.yaml in the current directory.
Every key can also be set via environment variable with the KC_ prefix (e.g. KC_CLIENT_SECRET).

Enable debug output: DEBUG=true %s ...
`, os.Args[0], os.Args[0])
}

func main() {
	debug = os.Getenv("DEBUG") == "true"

	initConfig()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "add-users":
		runAddUsers(os.Args[2:])
	case "create-groups":
		runCreateGroups(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand '%s'.\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func keycloakConnect(ctx context.Context) (*gocloak.GoCloak, string) {
	serverURL := viper.GetString("server_url")
	realm := viper.GetString("realm")
	clientID := viper.GetString("client_id")
	clientSecret := viper.GetString("client_secret")

	for _, key := range []string{"server_url", "realm", "client_id", "client_secret"} {
		if viper.GetString(key) == "" {
			log.Fatalf("Required config key '%s' is not set (env: KC_%s)", key, strings.ToUpper(key))
		}
	}

	client := gocloak.NewClient(serverURL)
	token, err := client.LoginClient(ctx, clientID, clientSecret, realm)
	if err != nil {
		log.Fatalf("Failed to login to Keycloak: %v", err)
	}
	debugf("login successful, token type: %s", token.TokenType)
	return client, token.AccessToken
}

func runAddUsers(args []string) {
	fs := flag.NewFlagSet("add-users", flag.ExitOnError)
	groupFlag := fs.String("group", viper.GetString("group_path"), "Target group name")
	colFlag := fs.String("col", viper.GetString("username_column"), "CSV column containing usernames")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s add-users [--group <name>] [--col <column>] <csv-file>\n", os.Args[0])
		os.Exit(1)
	}
	csvFile := fs.Arg(0)

	if *groupFlag == "" {
		log.Fatalf("Group name is required (--group or config 'group_path')")
	}

	realm := viper.GetString("realm")
	ctx := context.Background()
	client, token := keycloakConnect(ctx)

	groups, err := client.GetGroups(ctx, token, realm, gocloak.GetGroupsParams{
		Search: gocloak.StringP(*groupFlag),
	})
	if err != nil {
		log.Fatalf("Failed to get groups using search '%s': %v", *groupFlag, err)
	}
	debugf("GetGroups returned %d top-level group(s) for search '%s'", len(groups), *groupFlag)

	group := findGroupByName(groups, *groupFlag)
	if group == nil || group.ID == nil {
		log.Fatalf("Group '%s' not found in realm '%s'.", *groupFlag, realm)
	}
	groupID := *group.ID
	fmt.Printf("Found group '%s' with ID: %s\n", *groupFlag, groupID)

	err = addUsersToGroup(ctx, client, token, realm, groupID, csvFile, *groupFlag, *colFlag)
	if err != nil {
		log.Fatalf("Failed to add users to group: %v", err)
	}
}

func runCreateGroups(args []string) {
	fs := flag.NewFlagSet("create-groups", flag.ExitOnError)
	parentFlag := fs.String("parent", viper.GetString("parent_group"), "Parent group name under which to create subgroups")
	prefixFlag := fs.String("prefix", viper.GetString("group_prefix"), "Prefix prepended to each group name")
	colFlag := fs.String("col", viper.GetString("group_name_column"), "CSV column containing group names")
	yesFlag := fs.Bool("yes", false, "Skip confirmation prompt")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s create-groups [--parent <name>] [--prefix <str>] [--col <column>] [--yes] <csv-file>\n", os.Args[0])
		os.Exit(1)
	}
	csvFile := fs.Arg(0)

	if *parentFlag == "" {
		log.Fatalf("Parent group is required (--parent or config 'parent_group')")
	}
	if *colFlag == "" {
		log.Fatalf("Group name column is required (--col or config 'group_name_column')")
	}

	realm := viper.GetString("realm")
	ctx := context.Background()
	client, token := keycloakConnect(ctx)

	err := createGroupsFromCSV(ctx, client, token, realm, csvFile, *parentFlag, *prefixFlag, *colFlag, *yesFlag)
	if err != nil {
		log.Fatalf("Failed to create groups: %v", err)
	}
}

func findGroupByName(groups []*gocloak.Group, name string) *gocloak.Group {
	for _, g := range groups {
		if g == nil {
			continue
		}
		if g.Name != nil && *g.Name == name {
			return g
		}
		if g.SubGroups != nil {
			subs := make([]*gocloak.Group, len(*g.SubGroups))
			for i := range *g.SubGroups {
				subs[i] = &(*g.SubGroups)[i]
			}
			if found := findGroupByName(subs, name); found != nil {
				return found
			}
		}
	}
	return nil
}

func readGroupNamesFromCSV(csvFilePath, groupNameColumn string) ([]string, error) {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file '%s': %w", csvFilePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.Comma = ';'

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header row from CSV: %w", err)
	}

	debugf("CSV header: %v", header)
	colIndex := -1
	for i, h := range header {
		if strings.TrimSpace(h) == groupNameColumn {
			colIndex = i
			break
		}
	}
	if colIndex == -1 {
		return nil, fmt.Errorf("column '%s' not found in CSV header: %v", groupNameColumn, header)
	}
	debugf("using column '%s' at index %d", groupNameColumn, colIndex)

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read records from CSV: %w", err)
	}

	seen := make(map[string]struct{})
	for _, row := range records {
		if len(row) <= colIndex {
			continue
		}
		name := strings.TrimSpace(row[colIndex])
		if name != "" {
			seen[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func createGroupsFromCSV(ctx context.Context, client *gocloak.GoCloak, token, realm, csvFilePath, parentGroupName, prefix, groupNameColumn string, skipConfirm bool) error {
	// Find parent group
	groups, err := client.GetGroups(ctx, token, realm, gocloak.GetGroupsParams{
		Search: gocloak.StringP(parentGroupName),
	})
	if err != nil {
		return fmt.Errorf("failed to search for parent group '%s': %w", parentGroupName, err)
	}
	parent := findGroupByName(groups, parentGroupName)
	if parent == nil || parent.ID == nil {
		return fmt.Errorf("parent group '%s' not found in realm '%s'", parentGroupName, realm)
	}
	parentID := *parent.ID
	fmt.Printf("Parent group: %s (id: %s)\n\n", parentGroupName, parentID)

	// Get existing subgroups of the parent
	parentDetail, err := client.GetGroup(ctx, token, realm, parentID)
	if err != nil {
		return fmt.Errorf("failed to get parent group details: %w", err)
	}
	existing := make(map[string]struct{})
	if parentDetail.SubGroups != nil {
		for _, sg := range *parentDetail.SubGroups {
			if sg.Name != nil {
				existing[*sg.Name] = struct{}{}
			}
		}
	}
	debugf("parent group has %d existing subgroups", len(existing))

	// Read group names from CSV
	names, err := readGroupNamesFromCSV(csvFilePath, groupNameColumn)
	if err != nil {
		return err
	}

	var toCreate []string
	var toSkip []string
	for _, name := range names {
		fullName := prefix + name
		if _, exists := existing[fullName]; exists {
			toSkip = append(toSkip, fullName)
		} else {
			toCreate = append(toCreate, fullName)
		}
	}

	// Print plan
	if len(toCreate) > 0 {
		fmt.Printf("Groups to create (%d):\n", len(toCreate))
		for _, name := range toCreate {
			fmt.Printf("  + %s\n", name)
		}
		fmt.Println()
	}
	if len(toSkip) > 0 {
		fmt.Printf("Groups already exist, will skip (%d):\n", len(toSkip))
		for _, name := range toSkip {
			fmt.Printf("  ~ %s\n", name)
		}
		fmt.Println()
	}

	if len(toCreate) == 0 {
		fmt.Println("Nothing to do.")
		return nil
	}

	// Confirm
	if !skipConfirm {
		fmt.Print("Proceed? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			os.Exit(1)
		}
	}

	// Create groups
	for _, name := range toCreate {
		_, err := client.CreateChildGroup(ctx, token, realm, parentID, gocloak.Group{
			Name: gocloak.StringP(name),
		})
		if err != nil {
			log.Printf("Failed to create group '%s': %v", name, err)
		} else {
			fmt.Printf("Created group '%s' under '%s'.\n", name, parentGroupName)
		}
	}
	return nil
}

func addUsersToGroup(ctx context.Context, client *gocloak.GoCloak, token, realm, groupID, csvFilePath, groupPath, usernameColumn string) error {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file '%s': %w", csvFilePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.Comma = ';'

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read header row from CSV: %w", err)
	}

	debugf("CSV header: %v", header)
	colIndex := -1
	for i, h := range header {
		if strings.TrimSpace(h) == usernameColumn {
			colIndex = i
			break
		}
	}
	if colIndex == -1 {
		return fmt.Errorf("column '%s' not found in CSV header: %v", usernameColumn, header)
	}
	debugf("using column '%s' at index %d", usernameColumn, colIndex)

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read records from CSV: %w", err)
	}

	for _, row := range records {
		if len(row) <= colIndex {
			log.Printf("Skipping row, not enough columns: %v", row)
			continue
		}
		username := strings.TrimSpace(row[colIndex])
		if username == "" {
			log.Printf("Skipping row, empty username in column '%s': %v", usernameColumn, row)
			continue
		}

		fmt.Printf("Processing user '%s'...\n", username)

		users, err := client.GetUsers(ctx, token, realm, gocloak.GetUsersParams{
			Username: gocloak.StringP(username),
			Exact:    gocloak.BoolP(true),
		})
		if err != nil {
			log.Printf("Failed to get user ID for '%s': %v", username, err)
			continue
		}

		if len(users) == 0 || users[0] == nil || users[0].ID == nil {
			log.Printf("User '%s' not found.", username)
			continue
		}
		userID := *users[0].ID
		debugf("resolved user '%s' to ID %s", username, userID)

		err = client.AddUserToGroup(ctx, token, realm, userID, groupID)
		if err != nil {
			log.Printf("Failed to add user '%s' (ID: %s) to group with path '%s': %v", username, userID, groupPath, err)
		} else {
			fmt.Printf("Added user '%s' (ID: %s) to group with path '%s'.\n", username, userID, groupPath)
		}
	}
	return nil
}
