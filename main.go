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

	// Get group ID for the specified group path
	groups, err := client.GetGroups(ctx, token.AccessToken, realm, gocloak.GetGroupsParams{
		Search: gocloak.StringP(groupPath), // Search by the full path
		//Exact:  gocloak.BoolP(true),        // Ensure exact match for the path
	})
	log.Printf("API call GetGroups with Search='%s', Exact=true returned %d groups.", groupPath, len(groups))
	if err != nil {
		log.Fatalf("Failed to get groups using search path '%s': %v", groupPath, err)
	}

	var groupID string
	// Iterate through the returned groups and verify the path
	// This is a robust way to ensure we have the correct group,
	// especially if 'Search' with 'Exact' might have nuances.
	for _, group := range groups {
		log.Printf("Group: %v", group)
		if group != nil && group.Path != nil {
			if group.ID != nil {
				groupID = *group.ID
				break
			}
		}
	}

	if groupID == "" {
		// Check if any group was returned but didn't match the path, for debugging.
		if len(groups) > 0 && groups[0] != nil && groups[0].Path != nil {
			log.Printf("Group: %v", groups[0])
			log.Printf("Note: A group was found by search, but its path ('%s') did not exactly match the target path ('%s').", *groups[0].Path, groupPath)
		} else if len(groups) > 0 {
			log.Printf("Note: Search returned %d group(s), but none matched the exact path '%s'. First group details: %+v", len(groups), groupPath, groups[0])
		}
		log.Fatalf("Group with path '%s' not found in realm '%s'. Ensure the path is correct and the group exists.", groupPath, realm)
	}
	fmt.Printf("Found group with path '%s' and ID: %s\n", groupPath, groupID)

	// Read usernames from the CSV file and add them to the group
	err = addUsersToGroup(ctx, client, token.AccessToken, realm, groupID, csvFile, groupPath, usernameColumn)
	if err != nil {
		log.Fatalf("Failed to add users to group: %v", err)
	}
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

		err = client.AddUserToGroup(ctx, token, realm, userID, groupID)
		if err != nil {
			log.Printf("Failed to add user '%s' (ID: %s) to group with path '%s': %v", username, userID, groupPath, err)
		} else {
			fmt.Printf("Added user '%s' (ID: %s) to group with path '%s'.\n", username, userID, groupPath)
		}
	}
	return nil
}
