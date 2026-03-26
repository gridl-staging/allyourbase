package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func resetPushFlags() {
	resetJSONFlag()

	resetCmdFlag := func(path []string, flagName, value string) {
		cmd, _, err := rootCmd.Find(path)
		if err != nil || cmd == nil {
			return
		}
		_ = cmd.Flags().Set(flagName, value)
	}

	resetCmdFlag([]string{"push", "list-devices"}, "app-id", "")
	resetCmdFlag([]string{"push", "list-devices"}, "user-id", "")
	resetCmdFlag([]string{"push", "list-devices"}, "include-inactive", "false")

	resetCmdFlag([]string{"push", "register-device"}, "app-id", "")
	resetCmdFlag([]string{"push", "register-device"}, "user-id", "")
	resetCmdFlag([]string{"push", "register-device"}, "provider", "")
	resetCmdFlag([]string{"push", "register-device"}, "platform", "")
	resetCmdFlag([]string{"push", "register-device"}, "token", "")
	resetCmdFlag([]string{"push", "register-device"}, "device-name", "")

	resetCmdFlag([]string{"push", "send"}, "app-id", "")
	resetCmdFlag([]string{"push", "send"}, "user-id", "")
	resetCmdFlag([]string{"push", "send"}, "title", "")
	resetCmdFlag([]string{"push", "send"}, "body", "")
	resetCmdFlag([]string{"push", "send"}, "data", "{}")

	resetCmdFlag([]string{"push", "list-deliveries"}, "app-id", "")
	resetCmdFlag([]string{"push", "list-deliveries"}, "user-id", "")
	resetCmdFlag([]string{"push", "list-deliveries"}, "status", "")
	resetCmdFlag([]string{"push", "list-deliveries"}, "limit", "50")
	resetCmdFlag([]string{"push", "list-deliveries"}, "offset", "0")
}

func TestPushCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "push" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'push' subcommand to be registered")
	}
}

func TestPushListDevicesTable(t *testing.T) {
	resetPushFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodGet, r.Method)
		testutil.Equal(t, "/api/admin/push/devices", r.URL.Path)
		testutil.Equal(t, "app-1", r.URL.Query().Get("app_id"))
		testutil.Equal(t, "user-1", r.URL.Query().Get("user_id"))
		testutil.Equal(t, "true", r.URL.Query().Get("include_inactive"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        "tok-1",
					"app_id":    "app-1",
					"user_id":   "user-1",
					"provider":  "fcm",
					"platform":  "android",
					"token":     "token-abc",
					"is_active": true,
				},
			},
			"count": 1,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"push", "list-devices",
			"--app-id", "app-1",
			"--user-id", "user-1",
			"--include-inactive",
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "tok-1")
	testutil.Contains(t, output, "fcm")
	testutil.Contains(t, output, "android")
}

func TestPushListDevicesJSON(t *testing.T) {
	resetPushFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "tok-1", "provider": "fcm"},
			},
			"count": 1,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"push", "list-devices", "--json", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var items []map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &items))
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "tok-1", items[0]["id"])
}

func TestPushRegisterDevice(t *testing.T) {
	resetPushFlags()
	var reqBody map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/push/devices", r.URL.Path)
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "tok-1",
			"app_id":      "app-1",
			"user_id":     "user-1",
			"provider":    "fcm",
			"platform":    "android",
			"token":       "token-abc",
			"device_name": "Pixel 9",
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"push", "register-device",
			"--app-id", "app-1",
			"--user-id", "user-1",
			"--provider", "fcm",
			"--platform", "android",
			"--token", "token-abc",
			"--device-name", "Pixel 9",
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "app-1", reqBody["app_id"])
	testutil.Equal(t, "user-1", reqBody["user_id"])
	testutil.Equal(t, "fcm", reqBody["provider"])
	testutil.Equal(t, "android", reqBody["platform"])
	testutil.Equal(t, "token-abc", reqBody["token"])
	testutil.Equal(t, "Pixel 9", reqBody["device_name"])
	testutil.Contains(t, output, "registered")
}

func TestPushRegisterDeviceRequiresToken(t *testing.T) {
	resetPushFlags()
	rootCmd.SetArgs([]string{
		"push", "register-device",
		"--app-id", "app-1",
		"--user-id", "user-1",
		"--provider", "fcm",
		"--platform", "android",
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "--token is required")
}

func TestPushRegisterDeviceRejectsInvalidProvider(t *testing.T) {
	resetPushFlags()
	rootCmd.SetArgs([]string{
		"push", "register-device",
		"--app-id", "app-1",
		"--user-id", "user-1",
		"--provider", "gcm",
		"--platform", "android",
		"--token", "token-1",
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "--provider must be one of: fcm, apns")
}

func TestPushRegisterDeviceRejectsInvalidPlatform(t *testing.T) {
	resetPushFlags()
	rootCmd.SetArgs([]string{
		"push", "register-device",
		"--app-id", "app-1",
		"--user-id", "user-1",
		"--provider", "fcm",
		"--platform", "web",
		"--token", "token-1",
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.Contains(t, err.Error(), "--platform must be one of: ios, android")
}

func TestPushRevokeDevice(t *testing.T) {
	resetPushFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodDelete, r.Method)
		testutil.Equal(t, "/api/admin/push/devices/tok-1", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"push", "revoke-device", "tok-1", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	testutil.Contains(t, output, "revoked")
}

func TestPushSend(t *testing.T) {
	resetPushFlags()
	var reqBody map[string]any
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodPost, r.Method)
		testutil.Equal(t, "/api/admin/push/send", r.URL.Path)
		testutil.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deliveries": []map[string]any{
				{"id": "del-1", "status": "pending"},
			},
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"push", "send",
			"--app-id", "app-1",
			"--user-id", "user-1",
			"--title", "Hello",
			"--body", "World",
			"--data", `{"kind":"kudos"}`,
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Equal(t, "app-1", reqBody["app_id"])
	testutil.Equal(t, "user-1", reqBody["user_id"])
	testutil.Equal(t, "Hello", reqBody["title"])
	testutil.Equal(t, "World", reqBody["body"])
	data, ok := reqBody["data"].(map[string]any)
	testutil.True(t, ok, "expected data object")
	testutil.Equal(t, "kudos", data["kind"])
	testutil.Contains(t, output, "delivery")
}

func TestPushSendInvalidDataJSON(t *testing.T) {
	resetPushFlags()
	rootCmd.SetArgs([]string{
		"push", "send",
		"--app-id", "app-1",
		"--user-id", "user-1",
		"--title", "Hello",
		"--body", "World",
		"--data", `{"count":1}`,
		"--url", "http://localhost:0", "--admin-token", "tok",
	})
	err := rootCmd.Execute()
	testutil.NotNil(t, err)
	testutil.True(t, strings.Contains(err.Error(), "invalid --data JSON"),
		"expected invalid --data JSON error, got: %v", err)
}

func TestPushListDeliveriesTable(t *testing.T) {
	resetPushFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		testutil.Equal(t, http.MethodGet, r.Method)
		testutil.Equal(t, "/api/admin/push/deliveries", r.URL.Path)
		testutil.Equal(t, "app-1", r.URL.Query().Get("app_id"))
		testutil.Equal(t, "user-1", r.URL.Query().Get("user_id"))
		testutil.Equal(t, "failed", r.URL.Query().Get("status"))
		testutil.Equal(t, "25", r.URL.Query().Get("limit"))
		testutil.Equal(t, "10", r.URL.Query().Get("offset"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":       "del-1",
					"provider": "fcm",
					"title":    "Hello",
					"status":   "failed",
				},
			},
			"count": 1,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"push", "list-deliveries",
			"--app-id", "app-1",
			"--user-id", "user-1",
			"--status", "failed",
			"--limit", "25",
			"--offset", "10",
			"--url", testAdminURL, "--admin-token", "tok",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	testutil.Contains(t, output, "del-1")
	testutil.Contains(t, output, "Hello")
	testutil.Contains(t, output, "failed")
}

func TestPushListDeliveriesJSON(t *testing.T) {
	resetPushFlags()
	stubAdminHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "del-1", "status": "sent"},
			},
			"count": 1,
		})
	})

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"push", "list-deliveries", "--json", "--url", testAdminURL, "--admin-token", "tok"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	var items []map[string]any
	testutil.NoError(t, json.Unmarshal([]byte(output), &items))
	testutil.Equal(t, 1, len(items))
	testutil.Equal(t, "del-1", items[0]["id"])
}
