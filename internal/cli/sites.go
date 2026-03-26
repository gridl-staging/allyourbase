// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/cli/sites.go.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var sitesCmd = &cobra.Command{
	Use:   "sites",
	Short: "Manage hosted static sites",
}

var sitesDeployCmd = &cobra.Command{
	Use:   "deploy <site-id-or-slug>",
	Short: "Deploy a prebuilt directory to a hosted site",
	Args:  cobra.ExactArgs(1),
	RunE:  runSitesDeploy,
}

func init() {
	sitesCmd.PersistentFlags().String("admin-token", "", "Admin/JWT token (or set AYB_ADMIN_TOKEN)")
	sitesCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")
	sitesDeployCmd.Flags().String("dir", "dist", "Path to prebuilt deploy directory")
	sitesCmd.AddCommand(sitesDeployCmd)
}

type adminSiteSummary struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

type adminSiteListResponse struct {
	Sites      []adminSiteSummary `json:"sites"`
	TotalCount int                `json:"totalCount"`
}

type adminDeploySummary struct {
	ID string `json:"id"`
}

// TODO: Document runSitesDeploy.
func runSitesDeploy(cmd *cobra.Command, args []string) error {
	siteReference := strings.TrimSpace(args[0])
	if siteReference == "" {
		return fmt.Errorf("site id or slug is required")
	}

	deployDirectory, _ := cmd.Flags().GetString("dir")
	deployFiles, err := collectDeployFiles(deployDirectory)
	if err != nil {
		return err
	}

	site, err := lookupAdminSiteByReference(cmd, siteReference)
	if err != nil {
		return err
	}

	deploy, err := createAdminSiteDeploy(cmd, site.ID)
	if err != nil {
		return err
	}

	for _, deployFile := range deployFiles {
		if err := uploadAdminDeployFile(cmd, site.ID, deploy.ID, deployDirectory, deployFile); err != nil {
			_ = failAdminSiteDeploy(cmd, site.ID, deploy.ID, err.Error())
			return err
		}
	}

	if err := promoteAdminSiteDeploy(cmd, site.ID, deploy.ID); err != nil {
		_ = failAdminSiteDeploy(cmd, site.ID, deploy.ID, err.Error())
		return err
	}

	fmt.Printf("Deployed site %q with deploy %s (%d files)\n", site.Slug, deploy.ID, len(deployFiles))
	return nil
}

// TODO: Document collectDeployFiles.
func collectDeployFiles(deployDirectory string) ([]string, error) {
	if strings.TrimSpace(deployDirectory) == "" {
		return nil, fmt.Errorf("deploy directory is required")
	}

	directoryInfo, err := os.Stat(deployDirectory)
	if err != nil {
		return nil, fmt.Errorf("deploy directory %q: %w", deployDirectory, err)
	}
	if !directoryInfo.IsDir() {
		return nil, fmt.Errorf("deploy directory %q is not a directory", deployDirectory)
	}

	deployFiles := []string{}
	err = filepath.WalkDir(deployDirectory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(deployDirectory, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)

		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("deploy directory contains symlink %q; symlinks are not allowed", relativePath)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("deploy path %q must be a regular file", relativePath)
		}

		deployFiles = append(deployFiles, relativePath)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking deploy directory: %w", err)
	}
	if len(deployFiles) == 0 {
		return nil, fmt.Errorf("deploy directory %q contains no files", deployDirectory)
	}

	sort.Strings(deployFiles)
	if !sortedPathsContain(deployFiles, "index.html") {
		return nil, fmt.Errorf("deploy directory %q must include index.html", deployDirectory)
	}
	return deployFiles, nil
}

func sortedPathsContain(paths []string, target string) bool {
	targetIndex := sort.SearchStrings(paths, target)
	return targetIndex < len(paths) && paths[targetIndex] == target
}

// TODO: Document lookupAdminSiteByReference.
func lookupAdminSiteByReference(cmd *cobra.Command, siteReference string) (*adminSiteSummary, error) {
	const pageSize = 100

	for page := 1; ; page++ {
		requestPath := fmt.Sprintf("/api/admin/sites?page=%d&perPage=%d", page, pageSize)
		response, body, err := adminRequest(cmd, http.MethodGet, requestPath, nil)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			return nil, serverError(response.StatusCode, body)
		}

		var payload adminSiteListResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("parsing sites list: %w", err)
		}

		for _, site := range payload.Sites {
			if site.ID == siteReference || strings.EqualFold(site.Slug, siteReference) {
				return &site, nil
			}
		}

		reachedEndOfList := len(payload.Sites) < pageSize
		if payload.TotalCount > 0 {
			reachedEndOfList = page*pageSize >= payload.TotalCount
		}
		if reachedEndOfList {
			break
		}
	}

	return nil, fmt.Errorf("site %q not found", siteReference)
}

// TODO: Document createAdminSiteDeploy.
func createAdminSiteDeploy(cmd *cobra.Command, siteID string) (*adminDeploySummary, error) {
	response, body, err := adminRequest(cmd, http.MethodPost, "/api/admin/sites/"+siteID+"/deploys", nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusCreated {
		return nil, serverError(response.StatusCode, body)
	}

	var deploy adminDeploySummary
	if err := json.Unmarshal(body, &deploy); err != nil {
		return nil, fmt.Errorf("parsing deploy create response: %w", err)
	}
	if deploy.ID == "" {
		return nil, fmt.Errorf("deploy create response missing deploy id")
	}
	return &deploy, nil
}

// TODO: Document uploadAdminDeployFile.
func uploadAdminDeployFile(cmd *cobra.Command, siteID, deployID, deployDirectory, relativePath string) error {
	sourcePath := filepath.Join(deployDirectory, filepath.FromSlash(relativePath))
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("opening deploy file %q: %w", relativePath, err)
	}
	defer sourceFile.Close()

	requestBody, contentType := streamingMultipartFileBody(
		"file",
		filepath.Base(relativePath),
		sourceFile,
		map[string]string{"name": relativePath},
	)
	uploadPath := "/api/admin/sites/" + siteID + "/deploys/" + deployID + "/files"
	response, body, err := adminRequestWithContentType(cmd, http.MethodPost, uploadPath, requestBody, contentType)
	if err != nil {
		return fmt.Errorf("uploading %q: %w", relativePath, err)
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return fmt.Errorf("uploading %q: %w", relativePath, serverError(response.StatusCode, body))
	}
	return nil
}

func failAdminSiteDeploy(cmd *cobra.Command, siteID, deployID, message string) error {
	payload, err := json.Marshal(map[string]string{"errorMessage": message})
	if err != nil {
		return fmt.Errorf("building fail payload: %w", err)
	}
	failPath := "/api/admin/sites/" + siteID + "/deploys/" + deployID + "/fail"
	response, body, err := adminRequest(cmd, http.MethodPost, failPath, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return serverError(response.StatusCode, body)
	}
	return nil
}

func promoteAdminSiteDeploy(cmd *cobra.Command, siteID, deployID string) error {
	promotePath := "/api/admin/sites/" + siteID + "/deploys/" + deployID + "/promote"
	response, body, err := adminRequest(cmd, http.MethodPost, promotePath, nil)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return serverError(response.StatusCode, body)
	}
	return nil
}
