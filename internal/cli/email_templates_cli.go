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

	"github.com/spf13/cobra"
)

var emailTemplatesCmd = &cobra.Command{
	Use:   "email-templates",
	Short: "Manage custom email templates",
}

var emailTemplatesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List email templates",
	RunE:  runEmailTemplatesList,
}

var emailTemplatesGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get the effective template for a key",
	Args:  cobra.ExactArgs(1),
	RunE:  runEmailTemplatesGet,
}

var emailTemplatesSetCmd = &cobra.Command{
	Use:   "set <key>",
	Short: "Create or update a custom template override",
	Args:  cobra.ExactArgs(1),
	RunE:  runEmailTemplatesSet,
}

var emailTemplatesDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a custom template override",
	Args:  cobra.ExactArgs(1),
	RunE:  runEmailTemplatesDelete,
}

var emailTemplatesPreviewCmd = &cobra.Command{
	Use:   "preview <key>",
	Short: "Preview a template with variables",
	Args:  cobra.ExactArgs(1),
	RunE:  runEmailTemplatesPreview,
}

var emailTemplatesEnableCmd = &cobra.Command{
	Use:   "enable <key>",
	Short: "Enable a custom template override",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEmailTemplatesSetEnabled(cmd, args[0], true)
	},
}

var emailTemplatesDisableCmd = &cobra.Command{
	Use:   "disable <key>",
	Short: "Disable a custom template override",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runEmailTemplatesSetEnabled(cmd, args[0], false)
	},
}

var emailTemplatesSendCmd = &cobra.Command{
	Use:   "send <key>",
	Short: "Send an email using a template key",
	Args:  cobra.ExactArgs(1),
	RunE:  runEmailTemplatesSend,
}

func init() {
	emailTemplatesCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	emailTemplatesCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	emailTemplatesSetCmd.Flags().String("subject", "", "Subject template (required)")
	emailTemplatesSetCmd.Flags().String("html-file", "", "Path to HTML template file (required)")

	emailTemplatesPreviewCmd.Flags().String("vars", "{}", "JSON variables object")
	emailTemplatesPreviewCmd.Flags().String("subject", "", "Preview subject template (optional)")
	emailTemplatesPreviewCmd.Flags().String("html-file", "", "Path to preview HTML template file (optional)")

	emailTemplatesSendCmd.Flags().String("to", "", "Recipient email address (required)")
	emailTemplatesSendCmd.Flags().String("vars", "{}", "JSON variables object")

	emailTemplatesCmd.AddCommand(emailTemplatesListCmd)
	emailTemplatesCmd.AddCommand(emailTemplatesGetCmd)
	emailTemplatesCmd.AddCommand(emailTemplatesSetCmd)
	emailTemplatesCmd.AddCommand(emailTemplatesDeleteCmd)
	emailTemplatesCmd.AddCommand(emailTemplatesPreviewCmd)
	emailTemplatesCmd.AddCommand(emailTemplatesEnableCmd)
	emailTemplatesCmd.AddCommand(emailTemplatesDisableCmd)
	emailTemplatesCmd.AddCommand(emailTemplatesSendCmd)

	rootCmd.AddCommand(emailTemplatesCmd)
}

func runEmailTemplatesList(cmd *cobra.Command, _ []string) error {
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/email/templates", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	var result struct {
		Items []struct {
			TemplateKey     string `json:"templateKey"`
			Source          string `json:"source"`
			SubjectTemplate string `json:"subjectTemplate"`
			Enabled         bool   `json:"enabled"`
			UpdatedAt       string `json:"updatedAt"`
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
		fmt.Println("No email templates found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tSOURCE\tENABLED\tUPDATED")
	for _, item := range result.Items {
		updated := item.UpdatedAt
		if updated == "" {
			updated = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", item.TemplateKey, item.Source, item.Enabled, updated)
	}
	return w.Flush()
}

func runEmailTemplatesGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	outFmt := outputFormat(cmd)

	resp, body, err := adminRequest(cmd, "GET", "/api/admin/email/templates/"+url.PathEscape(key), nil)
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

	var eff struct {
		Source          string   `json:"source"`
		TemplateKey     string   `json:"templateKey"`
		SubjectTemplate string   `json:"subjectTemplate"`
		HTMLTemplate    string   `json:"htmlTemplate"`
		Enabled         bool     `json:"enabled"`
		Variables       []string `json:"variables"`
	}
	if err := json.Unmarshal(body, &eff); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	fmt.Printf("Key: %s\n", eff.TemplateKey)
	fmt.Printf("Source: %s\n", eff.Source)
	fmt.Printf("Enabled: %v\n", eff.Enabled)
	fmt.Printf("Subject: %s\n", eff.SubjectTemplate)
	if len(eff.Variables) > 0 {
		fmt.Printf("Variables: %s\n", strings.Join(eff.Variables, ", "))
	}
	fmt.Println("HTML:")
	fmt.Println(eff.HTMLTemplate)
	return nil
}

func runEmailTemplatesSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	outFmt := outputFormat(cmd)
	subject, _ := cmd.Flags().GetString("subject")
	htmlFile, _ := cmd.Flags().GetString("html-file")

	if subject == "" {
		return fmt.Errorf("--subject is required")
	}
	if htmlFile == "" {
		return fmt.Errorf("--html-file is required")
	}

	htmlBytes, err := os.ReadFile(htmlFile)
	if err != nil {
		return fmt.Errorf("reading --html-file %q: %w", htmlFile, err)
	}

	payload := map[string]string{
		"subjectTemplate": subject,
		"htmlTemplate":    string(htmlBytes),
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, body, err := adminRequest(cmd, "PUT", "/api/admin/email/templates/"+url.PathEscape(key), bytes.NewReader(reqBody))
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

	fmt.Printf("Email template %q updated.\n", key)
	return nil
}

func runEmailTemplatesDelete(cmd *cobra.Command, args []string) error {
	key := args[0]
	resp, body, err := adminRequest(cmd, "DELETE", "/api/admin/email/templates/"+url.PathEscape(key), nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return serverError(resp.StatusCode, body)
	}
	fmt.Printf("Email template %q deleted.\n", key)
	return nil
}

func runEmailTemplatesPreview(cmd *cobra.Command, args []string) error {
	key := args[0]
	outFmt := outputFormat(cmd)
	varsRaw, _ := cmd.Flags().GetString("vars")
	subjectFlag, _ := cmd.Flags().GetString("subject")
	htmlFile, _ := cmd.Flags().GetString("html-file")

	vars, err := parseStringVars("--vars", varsRaw)
	if err != nil {
		return err
	}

	subject := subjectFlag
	var htmlTpl string
	if htmlFile != "" {
		htmlBytes, err := os.ReadFile(htmlFile)
		if err != nil {
			return fmt.Errorf("reading --html-file %q: %w", htmlFile, err)
		}
		htmlTpl = string(htmlBytes)
	}

	// When preview content is not provided explicitly, preview the effective template.
	if subject == "" || htmlTpl == "" {
		resp, body, err := adminRequest(cmd, "GET", "/api/admin/email/templates/"+url.PathEscape(key), nil)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return serverError(resp.StatusCode, body)
		}
		var eff struct {
			SubjectTemplate string `json:"subjectTemplate"`
			HTMLTemplate    string `json:"htmlTemplate"`
		}
		if err := json.Unmarshal(body, &eff); err != nil {
			return fmt.Errorf("parsing effective template response: %w", err)
		}
		if subject == "" {
			subject = eff.SubjectTemplate
		}
		if htmlTpl == "" {
			htmlTpl = eff.HTMLTemplate
		}
	}

	payload := map[string]any{
		"subjectTemplate": subject,
		"htmlTemplate":    htmlTpl,
		"variables":       vars,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, body, err := adminRequest(cmd, "POST", "/api/admin/email/templates/"+url.PathEscape(key)+"/preview", bytes.NewReader(reqBody))
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

	var preview struct {
		Subject string `json:"subject"`
		HTML    string `json:"html"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(body, &preview); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	fmt.Printf("Subject: %s\n\n", preview.Subject)
	fmt.Println("HTML:")
	fmt.Println(preview.HTML)
	fmt.Println()
	fmt.Println("Text:")
	fmt.Println(preview.Text)
	return nil
}

func runEmailTemplatesSetEnabled(cmd *cobra.Command, key string, enabled bool) error {
	outFmt := outputFormat(cmd)
	payload := map[string]bool{"enabled": enabled}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, body, err := adminRequest(cmd, "PATCH", "/api/admin/email/templates/"+url.PathEscape(key), bytes.NewReader(reqBody))
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

	if enabled {
		fmt.Printf("Email template %q enabled.\n", key)
	} else {
		fmt.Printf("Email template %q disabled.\n", key)
	}
	return nil
}

func runEmailTemplatesSend(cmd *cobra.Command, args []string) error {
	key := args[0]
	to, _ := cmd.Flags().GetString("to")
	varsRaw, _ := cmd.Flags().GetString("vars")
	outFmt := outputFormat(cmd)

	if to == "" {
		return fmt.Errorf("--to is required")
	}

	vars, err := parseStringVars("--vars", varsRaw)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"templateKey": key,
		"to":          to,
		"variables":   vars,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializing payload: %w", err)
	}

	resp, body, err := adminRequest(cmd, "POST", "/api/admin/email/send", bytes.NewReader(reqBody))
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

	fmt.Printf("Email sent using template %q to %s.\n", key, to)
	return nil
}

func parseStringVars(flagName, raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}

	var vars map[string]string
	if err := json.Unmarshal([]byte(raw), &vars); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %w", flagName, err)
	}
	if vars == nil {
		vars = map[string]string{}
	}
	return vars, nil
}
