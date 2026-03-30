package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/allyourbase/ayb/examples"
)

func applyDemoSchema(baseURL, name string) (string, error) {
	schemaSQL, err := fs.ReadFile(examples.FS, name+"/schema.sql")
	if err != nil {
		return "", fmt.Errorf("reading embedded schema.sql: %w", err)
	}

	token, err := resolveDemoAdminToken(baseURL)
	if err != nil {
		return "", fmt.Errorf("authenticating with server: %w", err)
	}

	body, err := json.Marshal(map[string]string{"query": string(schemaSQL)})
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}
	req, err := http.NewRequest("POST", baseURL+"/api/admin/sql/", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending schema to server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading server response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(respBody)
		if demoSchemaAlreadyExists(bodyStr) {
			return "exists", nil
		}
		if msg, ok := demoSchemaErrorMessage(respBody); ok {
			return "", fmt.Errorf("SQL error: %s", msg)
		}
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, bodyStr)
	}

	return "applied", nil
}

// resolveDemoAdminToken obtains an admin bearer token for demo schema/seed operations, trying the CLI token, env var, and saved token file in order.
func resolveDemoAdminToken(baseURL string) (string, error) {
	if token := resolveCLIAdminToken("", baseURL); token != "" {
		return token, nil
	}
	if !isLoopbackAdminURL(baseURL) {
		return "", fmt.Errorf("no admin token found.\n\n"+
			"  Refusing to use locally saved admin credentials for non-loopback server %q.\n"+
			"  Set AYB_ADMIN_TOKEN to an admin bearer token for that server.",
			baseURL,
		)
	}

	tokenPath, saved, err := readSavedAdminTokenFile()
	if err != nil {
		if tokenPath == "" {
			return "", fmt.Errorf("no admin token: could not resolve home directory: %w", err)
		}
		return "", fmt.Errorf("no admin token found.\n\n"+
			"  The server is running but wasn't started by the demo command.\n"+
			"  Stop it and let the demo handle everything:\n\n"+
			"    %s%s\n\n"+
			"  Or, if using lsof to find orphan processes:\n"+
			"    %s%s",
			demoRestartCommand(),
			demoCustomPortNote(baseURL, fmt.Sprintf("ayb stop --port %s && ayb demo <name>", demoServerPort(baseURL))),
			demoKillCommand(),
			demoCustomPortNote(baseURL, fmt.Sprintf("lsof -ti :%s | xargs kill && ayb demo <name>", demoServerPort(baseURL))),
		)
	}

	if saved == "" {
		return "", fmt.Errorf("admin token file is empty: %s", tokenPath)
	}
	return exchangeSavedAdminAuth(baseURL, saved), nil
}

// seedDemoUsers registers the seed accounts via the auth API.
// Ignores 409 Conflict (user already exists).
func seedDemoUsers(baseURL string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	for _, u := range demoSeedUsers {
		body, err := json.Marshal(map[string]string{"email": u.Email, "password": u.Password})
		if err != nil {
			return err
		}
		resp, err := client.Post(baseURL+"/api/auth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("registering %s: %w", u.Email, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
			return fmt.Errorf("registering %s: unexpected status %d", u.Email, resp.StatusCode)
		}
	}
	return nil
}

func demoSchemaAlreadyExists(body string) bool {
	return strings.Contains(body, "already exists")
}

func demoSchemaErrorMessage(respBody []byte) (string, bool) {
	var errResp map[string]any
	if err := json.Unmarshal(respBody, &errResp); err != nil {
		return "", false
	}
	msg, ok := errResp["message"].(string)
	return msg, ok && msg != ""
}
