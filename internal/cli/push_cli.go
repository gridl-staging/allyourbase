// Package cli This file provides CLI commands for managing push notifications, including device registration, listing devices and deliveries, and sending push notifications to users.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/allyourbase/ayb/internal/push"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Manage push devices and deliveries",
}

var pushListDevicesCmd = &cobra.Command{
	Use:   "list-devices",
	Short: "List push device tokens",
	RunE:  runPushListDevices,
}

var pushRegisterDeviceCmd = &cobra.Command{
	Use:   "register-device",
	Short: "Register a push device token",
	RunE:  runPushRegisterDevice,
}

var pushRevokeDeviceCmd = &cobra.Command{
	Use:   "revoke-device <device-id>",
	Short: "Revoke a push device token",
	Args:  cobra.ExactArgs(1),
	RunE:  runPushRevokeDevice,
}

var pushSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send push notification to a user",
	RunE:  runPushSend,
}

var pushListDeliveriesCmd = &cobra.Command{
	Use:   "list-deliveries",
	Short: "List push delivery history",
	RunE:  runPushListDeliveries,
}

func init() {
	pushCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	pushCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	pushListDevicesCmd.Flags().String("app-id", "", "Filter by app ID")
	pushListDevicesCmd.Flags().String("user-id", "", "Filter by user ID")
	pushListDevicesCmd.Flags().Bool("include-inactive", false, "Include inactive tokens")

	pushRegisterDeviceCmd.Flags().String("app-id", "", "App ID (required)")
	pushRegisterDeviceCmd.Flags().String("user-id", "", "User ID (required)")
	pushRegisterDeviceCmd.Flags().String("provider", "", "Provider (required: fcm|apns)")
	pushRegisterDeviceCmd.Flags().String("platform", "", "Platform (required: ios|android)")
	pushRegisterDeviceCmd.Flags().String("token", "", "Device token (required)")
	pushRegisterDeviceCmd.Flags().String("device-name", "", "Optional device name")

	pushSendCmd.Flags().String("app-id", "", "App ID (required)")
	pushSendCmd.Flags().String("user-id", "", "User ID (required)")
	pushSendCmd.Flags().String("title", "", "Notification title (required)")
	pushSendCmd.Flags().String("body", "", "Notification body (required)")
	pushSendCmd.Flags().String("data", "{}", "JSON map[string]string data payload")

	pushListDeliveriesCmd.Flags().String("app-id", "", "Filter by app ID")
	pushListDeliveriesCmd.Flags().String("user-id", "", "Filter by user ID")
	pushListDeliveriesCmd.Flags().String("status", "", "Filter by status (pending|sent|failed|invalid_token)")
	pushListDeliveriesCmd.Flags().Int("limit", 50, "Maximum results")
	pushListDeliveriesCmd.Flags().Int("offset", 0, "Offset")

	pushCmd.AddCommand(pushListDevicesCmd)
	pushCmd.AddCommand(pushRegisterDeviceCmd)
	pushCmd.AddCommand(pushRevokeDeviceCmd)
	pushCmd.AddCommand(pushSendCmd)
	pushCmd.AddCommand(pushListDeliveriesCmd)

	rootCmd.AddCommand(pushCmd)
}

// runPushListDevices lists registered push device tokens, optionally filtering by app-id, user-id, and active status, and outputs results in table, JSON, or CSV format.
func runPushListDevices(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	appID, _ := cmd.Flags().GetString("app-id")
	userID, _ := cmd.Flags().GetString("user-id")
	includeInactive, _ := cmd.Flags().GetBool("include-inactive")

	q := url.Values{}
	if appID != "" {
		q.Set("app_id", appID)
	}
	if userID != "" {
		q.Set("user_id", userID)
	}
	if includeInactive {
		q.Set("include_inactive", "true")
	}

	path := "/api/admin/push/devices"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	resp, body, err := adminRequest(cmd, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var result struct {
		Items []struct {
			ID         string  `json:"id"`
			UserID     string  `json:"user_id"`
			Provider   string  `json:"provider"`
			Platform   string  `json:"platform"`
			Token      string  `json:"token"`
			DeviceName *string `json:"device_name"`
			IsActive   bool    `json:"is_active"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if outFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Items)
	}

	if len(result.Items) == 0 {
		fmt.Println("No push devices found.")
		return nil
	}

	if outFmt == "csv" {
		rows := make([][]string, 0, len(result.Items))
		for _, item := range result.Items {
			deviceName := ""
			if item.DeviceName != nil {
				deviceName = *item.DeviceName
			}
			rows = append(rows, []string{
				item.ID,
				item.UserID,
				item.Provider,
				item.Platform,
				fmt.Sprintf("%v", item.IsActive),
				tokenPreview(item.Token),
				deviceName,
			})
		}
		return writeCSVStdout(
			[]string{"ID", "User", "Provider", "Platform", "Active", "Token", "Device"},
			rows,
		)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSER\tPROVIDER\tPLATFORM\tACTIVE\tTOKEN\tDEVICE")
	for _, item := range result.Items {
		deviceName := "-"
		if item.DeviceName != nil && *item.DeviceName != "" {
			deviceName = *item.DeviceName
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%v\t%s\t%s\n",
			item.ID,
			item.UserID,
			item.Provider,
			item.Platform,
			item.IsActive,
			tokenPreview(item.Token),
			deviceName,
		)
	}
	return w.Flush()
}

// runPushRegisterDevice registers a new push device token with required fields app-id, user-id, provider (fcm or apns), platform (ios or android), and device token. It returns the registered device information.
func runPushRegisterDevice(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	appID, _ := cmd.Flags().GetString("app-id")
	userID, _ := cmd.Flags().GetString("user-id")
	provider, _ := cmd.Flags().GetString("provider")
	platform, _ := cmd.Flags().GetString("platform")
	token, _ := cmd.Flags().GetString("token")
	deviceName, _ := cmd.Flags().GetString("device-name")

	if appID == "" {
		return fmt.Errorf("--app-id is required")
	}
	if userID == "" {
		return fmt.Errorf("--user-id is required")
	}
	if provider == "" {
		return fmt.Errorf("--provider is required")
	}
	if platform == "" {
		return fmt.Errorf("--platform is required")
	}
	if token == "" {
		return fmt.Errorf("--token is required")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case push.ProviderFCM, push.ProviderAPNS:
	default:
		return fmt.Errorf("--provider must be one of: fcm, apns")
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	switch platform {
	case push.PlatformIOS, push.PlatformAndroid:
	default:
		return fmt.Errorf("--platform must be one of: ios, android")
	}

	payload := map[string]string{
		"app_id":      appID,
		"user_id":     userID,
		"provider":    provider,
		"platform":    platform,
		"token":       token,
		"device_name": deviceName,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, body, err := adminRequest(cmd, http.MethodPost, "/api/admin/push/devices", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var dt struct {
		ID       string `json:"id"`
		UserID   string `json:"user_id"`
		Provider string `json:"provider"`
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal(body, &dt); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf(
		"Push device %q registered for user %s (%s/%s).\n",
		dt.ID,
		dt.UserID,
		dt.Provider,
		dt.Platform,
	)
	return nil
}

func runPushRevokeDevice(cmd *cobra.Command, args []string) error {
	deviceID := args[0]
	resp, body, err := adminRequest(cmd, http.MethodDelete, "/api/admin/push/devices/"+url.PathEscape(deviceID), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return serverError(resp.StatusCode, body)
	}
	fmt.Printf("Push device %q revoked.\n", deviceID)
	return nil
}

// runPushSend sends a push notification to a user with required fields app-id, user-id, title, and body. It accepts an optional JSON data payload, queues the notification via the admin API, and reports the number of deliveries created.
func runPushSend(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	appID, _ := cmd.Flags().GetString("app-id")
	userID, _ := cmd.Flags().GetString("user-id")
	title, _ := cmd.Flags().GetString("title")
	bodyText, _ := cmd.Flags().GetString("body")
	dataRaw, _ := cmd.Flags().GetString("data")

	if appID == "" {
		return fmt.Errorf("--app-id is required")
	}
	if userID == "" {
		return fmt.Errorf("--user-id is required")
	}
	if title == "" {
		return fmt.Errorf("--title is required")
	}
	if bodyText == "" {
		return fmt.Errorf("--body is required")
	}

	data, err := parseStringVars("--data", dataRaw)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"app_id":  appID,
		"user_id": userID,
		"title":   title,
		"body":    bodyText,
		"data":    data,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, body, err := adminRequest(cmd, http.MethodPost, "/api/admin/push/send", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var result struct {
		Deliveries []struct {
			ID string `json:"id"`
		} `json:"deliveries"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	switch len(result.Deliveries) {
	case 0:
		fmt.Println("Push queued: no deliveries created.")
	case 1:
		fmt.Printf("Push queued: 1 delivery created (%s).\n", result.Deliveries[0].ID)
	default:
		fmt.Printf("Push queued: %d deliveries created.\n", len(result.Deliveries))
	}
	return nil
}

// runPushListDeliveries lists push delivery history with optional filtering by app-id, user-id, and status, supporting pagination via limit and offset parameters. Results are output in table, JSON, or CSV format.
func runPushListDeliveries(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)
	appID, _ := cmd.Flags().GetString("app-id")
	userID, _ := cmd.Flags().GetString("user-id")
	status, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	if status != "" {
		switch status {
		case push.DeliveryStatusPending, push.DeliveryStatusSent, push.DeliveryStatusFailed, push.DeliveryStatusInvalidToken:
		default:
			return fmt.Errorf("--status must be one of: pending, sent, failed, invalid_token")
		}
	}
	if limit <= 0 {
		return fmt.Errorf("--limit must be greater than 0")
	}
	if offset < 0 {
		return fmt.Errorf("--offset must be greater than or equal to 0")
	}

	q := url.Values{}
	if appID != "" {
		q.Set("app_id", appID)
	}
	if userID != "" {
		q.Set("user_id", userID)
	}
	if status != "" {
		q.Set("status", status)
	}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", fmt.Sprintf("%d", offset))

	resp, body, err := adminRequest(cmd, http.MethodGet, "/api/admin/push/deliveries?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var result struct {
		Items []struct {
			ID        string  `json:"id"`
			Provider  string  `json:"provider"`
			Title     string  `json:"title"`
			Status    string  `json:"status"`
			ErrorCode *string `json:"error_code"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if outFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Items)
	}

	if len(result.Items) == 0 {
		fmt.Println("No push deliveries found.")
		return nil
	}

	if outFmt == "csv" {
		rows := make([][]string, 0, len(result.Items))
		for _, item := range result.Items {
			errorCode := ""
			if item.ErrorCode != nil {
				errorCode = *item.ErrorCode
			}
			rows = append(rows, []string{
				item.ID,
				item.Provider,
				item.Status,
				item.Title,
				errorCode,
			})
		}
		return writeCSVStdout([]string{"ID", "Provider", "Status", "Title", "Error"}, rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROVIDER\tSTATUS\tTITLE\tERROR")
	for _, item := range result.Items {
		errorCode := "-"
		if item.ErrorCode != nil && strings.TrimSpace(*item.ErrorCode) != "" {
			errorCode = *item.ErrorCode
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Provider, item.Status, item.Title, errorCode)
	}
	return w.Flush()
}

func tokenPreview(token string) string {
	const max = 16
	if len(token) <= max {
		return token
	}
	return token[:max] + "..."
}
