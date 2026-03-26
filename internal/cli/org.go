// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/cli/org.go.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type orgCLIRecord struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	ParentOrgID *string `json:"parentOrgId"`
	PlanTier    string  `json:"planTier"`
	CreatedAt   string  `json:"createdAt"`
}

var orgCmd = &cobra.Command{
	Use:   "org",
	Short: "Manage organizations",
}

var orgCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an organization",
	RunE:  runOrgCreate,
}

var orgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List organizations",
	RunE:  runOrgList,
}

var orgMembersCmd = &cobra.Command{
	Use:   "members",
	Short: "Manage organization members",
}

var orgMembersListCmd = &cobra.Command{
	Use:   "list <org-slug>",
	Short: "List organization members",
	Args:  cobra.ExactArgs(1),
	RunE:  runOrgMembersList,
}

var orgMembersAddCmd = &cobra.Command{
	Use:   "add <org-slug> <user-id>",
	Short: "Add organization member",
	Args:  cobra.ExactArgs(2),
	RunE:  runOrgMembersAdd,
}

var orgTeamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "Manage organization teams",
}

var orgTeamsListCmd = &cobra.Command{
	Use:   "list <org-slug>",
	Short: "List teams in an organization",
	Args:  cobra.ExactArgs(1),
	RunE:  runOrgTeamsList,
}

var orgTeamsCreateCmd = &cobra.Command{
	Use:   "create <org-slug>",
	Short: "Create a team in an organization",
	Args:  cobra.ExactArgs(1),
	RunE:  runOrgTeamsCreate,
}

func init() {
	orgCmd.PersistentFlags().String("admin-token", "", "Admin token (or set AYB_ADMIN_TOKEN)")
	orgCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	orgCreateCmd.Flags().String("name", "", "Organization name")
	orgCreateCmd.Flags().String("slug", "", "Organization slug")
	orgCreateCmd.Flags().String("parent-org-id", "", "Parent organization ID")
	orgCreateCmd.Flags().String("plan-tier", "", "Plan tier (free, pro, enterprise)")
	orgCreateCmd.MarkFlagRequired("name")
	orgCreateCmd.MarkFlagRequired("slug")

	orgMembersAddCmd.Flags().String("role", "member", "Organization role (owner, admin, member, viewer)")

	orgTeamsCreateCmd.Flags().String("name", "", "Team name")
	orgTeamsCreateCmd.Flags().String("slug", "", "Team slug")
	orgTeamsCreateCmd.MarkFlagRequired("name")
	orgTeamsCreateCmd.MarkFlagRequired("slug")

	orgCmd.AddCommand(orgCreateCmd)
	orgCmd.AddCommand(orgListCmd)
	orgCmd.AddCommand(orgMembersCmd)
	orgCmd.AddCommand(orgTeamsCmd)
	orgMembersCmd.AddCommand(orgMembersListCmd)
	orgMembersCmd.AddCommand(orgMembersAddCmd)
	orgTeamsCmd.AddCommand(orgTeamsListCmd)
	orgTeamsCmd.AddCommand(orgTeamsCreateCmd)
}

// TODO: Document runOrgCreate.
func runOrgCreate(cmd *cobra.Command, _ []string) error {
	name, _ := cmd.Flags().GetString("name")
	slug, _ := cmd.Flags().GetString("slug")
	parentOrgID, _ := cmd.Flags().GetString("parent-org-id")
	planTier, _ := cmd.Flags().GetString("plan-tier")

	payload := map[string]any{"name": name, "slug": slug}
	if parentOrgID != "" {
		payload["parentOrgId"] = parentOrgID
	}
	if planTier != "" {
		payload["planTier"] = planTier
	}
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	resp, body, err := adminRequest(cmd, http.MethodPost, "/api/admin/orgs", bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, body)
	}
	if outputFormat(cmd) == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var created orgCLIRecord
	if err := json.Unmarshal(body, &created); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	fmt.Printf("Created org %s (%s).\n", created.Slug, created.ID)
	return nil
}

// TODO: Document runOrgList.
func runOrgList(cmd *cobra.Command, _ []string) error {
	resp, body, err := adminRequest(cmd, http.MethodGet, "/api/admin/orgs", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	outFmt := outputFormat(cmd)
	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var result struct {
		Items []orgCLIRecord `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Items) == 0 {
		fmt.Println("No orgs found.")
		return nil
	}

	cols := []string{"ID", "Name", "Slug", "Parent", "Plan", "Created"}
	rows := make([][]string, 0, len(result.Items))
	for _, item := range result.Items {
		parent := ""
		if item.ParentOrgID != nil {
			parent = *item.ParentOrgID
		}
		rows = append(rows, []string{item.ID, item.Name, item.Slug, parent, item.PlanTier, item.CreatedAt})
	}
	if outFmt == "csv" {
		return writeCSVStdout(cols, rows)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
	return nil
}

// TODO: Document runOrgMembersList.
func runOrgMembersList(cmd *cobra.Command, args []string) error {
	org, err := resolveOrgBySlug(cmd, args[0])
	if err != nil {
		return err
	}

	resp, body, err := adminRequest(cmd, http.MethodGet, "/api/admin/orgs/"+org.ID+"/members", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}

	outFmt := outputFormat(cmd)
	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var result struct {
		Items []struct {
			UserID string `json:"userId"`
			Role   string `json:"role"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Items) == 0 {
		fmt.Printf("No members found for org %s.\n", org.Slug)
		return nil
	}

	cols := []string{"UserID", "Role"}
	rows := make([][]string, 0, len(result.Items))
	for _, item := range result.Items {
		rows = append(rows, []string{item.UserID, item.Role})
	}
	if outFmt == "csv" {
		return writeCSVStdout(cols, rows)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
	return nil
}

// TODO: Document runOrgMembersAdd.
func runOrgMembersAdd(cmd *cobra.Command, args []string) error {
	org, err := resolveOrgBySlug(cmd, args[0])
	if err != nil {
		return err
	}
	role, _ := cmd.Flags().GetString("role")

	requestBody, err := json.Marshal(map[string]string{"userId": args[1], "role": role})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	resp, body, err := adminRequest(cmd, http.MethodPost, "/api/admin/orgs/"+org.ID+"/members", bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, body)
	}
	if outputFormat(cmd) == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}
	fmt.Printf("Added user %s to org %s as %s.\n", args[1], org.Slug, role)
	return nil
}

// TODO: Document runOrgTeamsList.
func runOrgTeamsList(cmd *cobra.Command, args []string) error {
	org, err := resolveOrgBySlug(cmd, args[0])
	if err != nil {
		return err
	}

	resp, body, err := adminRequest(cmd, http.MethodGet, "/api/admin/orgs/"+org.ID+"/teams", nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return serverError(resp.StatusCode, body)
	}
	outFmt := outputFormat(cmd)
	if outFmt == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}

	var result struct {
		Items []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Items) == 0 {
		fmt.Printf("No teams found for org %s.\n", org.Slug)
		return nil
	}

	cols := []string{"ID", "Name", "Slug"}
	rows := make([][]string, 0, len(result.Items))
	for _, item := range result.Items {
		rows = append(rows, []string{item.ID, item.Name, item.Slug})
	}
	if outFmt == "csv" {
		return writeCSVStdout(cols, rows)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("---\t", len(cols)))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
	return nil
}

// TODO: Document runOrgTeamsCreate.
func runOrgTeamsCreate(cmd *cobra.Command, args []string) error {
	org, err := resolveOrgBySlug(cmd, args[0])
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	slug, _ := cmd.Flags().GetString("slug")

	requestBody, err := json.Marshal(map[string]string{"name": name, "slug": slug})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	resp, body, err := adminRequest(cmd, http.MethodPost, "/api/admin/orgs/"+org.ID+"/teams", bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, body)
	}
	if outputFormat(cmd) == "json" {
		os.Stdout.Write(body)
		fmt.Println()
		return nil
	}
	fmt.Printf("Created team %s in org %s.\n", slug, org.Slug)
	return nil
}

// TODO: Document resolveOrgBySlug.
func resolveOrgBySlug(cmd *cobra.Command, slug string) (*orgCLIRecord, error) {
	resp, body, err := adminRequest(cmd, http.MethodGet, "/api/admin/orgs", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, serverError(resp.StatusCode, body)
	}

	var result struct {
		Items []orgCLIRecord `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing org list: %w", err)
	}
	for _, item := range result.Items {
		if item.Slug == slug {
			org := item
			return &org, nil
		}
	}
	return nil, fmt.Errorf("organization with slug %q not found", slug)
}
