package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
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

func main() {
	debug = os.Getenv("DEBUG") == "true"

	initConfig()

	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <csv-file>", os.Args[0])
	}
	csvFile := os.Args[1]

	serverURL := viper.GetString("server_url")
	realm := viper.GetString("realm")
	clientID := viper.GetString("client_id")
	clientSecret := viper.GetString("client_secret")
	groupPath := viper.GetString("group_path")
	usernameColumn := viper.GetString("username_column")

	debugf("config: server_url=%s realm=%s client_id=%s group_path=%s username_column=%s csv_file=%s",
		serverURL, realm, clientID, groupPath, usernameColumn, csvFile)

	for _, key := range []string{"server_url", "realm", "client_id", "client_secret", "group_path"} {
		if viper.GetString(key) == "" {
			log.Fatalf("Required config key '%s' is not set (env: KC_%s)", key, strings.ToUpper(key))
		}
	}

	ctx := context.Background()

	client := gocloak.NewClient(serverURL)
	token, err := client.LoginClient(ctx, clientID, clientSecret, realm)
	if err != nil {
		log.Fatalf("Failed to login to Keycloak: %v", err)
	}
	debugf("login successful, token type: %s", token.TokenType)

	groups, err := client.GetGroups(ctx, token.AccessToken, realm, gocloak.GetGroupsParams{
		Search: gocloak.StringP(groupPath),
	})
	if err != nil {
		log.Fatalf("Failed to get groups using search '%s': %v", groupPath, err)
	}
	debugf("GetGroups returned %d top-level group(s) for search '%s'", len(groups), groupPath)
	for _, g := range groups {
		if g != nil {
			debugf("  group: name=%s path=%s id=%s", strVal(g.Name), strVal(g.Path), strVal(g.ID))
		}
	}

	group := findGroupByName(groups, groupPath)
	if group == nil || group.ID == nil {
		log.Fatalf("Group '%s' not found in realm '%s'.", groupPath, realm)
	}
	groupID := *group.ID
	fmt.Printf("Found group '%s' with ID: %s\n", groupPath, groupID)

	// Read usernames from the CSV file and add them to the group
	err = addUsersToGroup(ctx, client, token.AccessToken, realm, groupID, csvFile, groupPath, usernameColumn)
	if err != nil {
		log.Fatalf("Failed to add users to group: %v", err)
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
