// Package cli Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/cli/storage.go.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var storageCmd = &cobra.Command{
	Use:   "storage",
	Short: "Manage file storage on the running AYB server",
}

var storageLsCmd = &cobra.Command{
	Use:   "ls <bucket>",
	Short: "List files in a bucket",
	Args:  cobra.ExactArgs(1),
	RunE:  runStorageLs,
}

var storageUploadCmd = &cobra.Command{
	Use:   "upload <bucket> <file>",
	Short: "Upload a file to a bucket",
	Args:  cobra.ExactArgs(2),
	RunE:  runStorageUpload,
}

var storageDownloadCmd = &cobra.Command{
	Use:   "download <bucket> <name>",
	Short: "Download a file from a bucket",
	Args:  cobra.ExactArgs(2),
	RunE:  runStorageDownload,
}

var storageDeleteCmd = &cobra.Command{
	Use:   "delete <bucket> <name>",
	Short: "Delete a file from a bucket",
	Args:  cobra.ExactArgs(2),
	RunE:  runStorageDelete,
}

func init() {
	storageCmd.PersistentFlags().String("admin-token", "", "Admin/JWT token (or set AYB_ADMIN_TOKEN)")
	storageCmd.PersistentFlags().String("url", "", "Server URL (default http://127.0.0.1:8090)")

	storageDownloadCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")

	storageCmd.AddCommand(storageLsCmd)
	storageCmd.AddCommand(storageUploadCmd)
	storageCmd.AddCommand(storageDownloadCmd)
	storageCmd.AddCommand(storageDeleteCmd)
}

// storageRequest makes an authenticated HTTP request to the storage API, using an admin token from flags or the AYB_ADMIN_TOKEN environment variable.
func storageRequest(cmd *cobra.Command, method, path string, body io.Reader, contentType string) (*http.Response, []byte, error) {
	token, baseURL := resolveStorageRequestOptions(cmd)
	req, err := newAuthenticatedRequest(method, baseURL, path, token, contentType, body)
	if err != nil {
		return nil, nil, err
	}
	return doBufferedRequest(req)
}

func resolveStorageRequestOptions(cmd *cobra.Command) (string, string) {
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")

	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}
	return token, baseURL
}

// runStorageLs lists files in a storage bucket and displays them as a formatted table, CSV, or JSON output.
func runStorageLs(cmd *cobra.Command, args []string) error {
	bucket := args[0]
	outFmt := outputFormat(cmd)

	resp, body, err := storageRequest(cmd, "GET", "/api/storage/"+bucket, nil, "")
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
		Items []struct {
			Name      string `json:"name"`
			Size      int64  `json:"size"`
			CreatedAt string `json:"createdAt"`
		} `json:"items"`
		TotalItems int `json:"totalItems"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Items) == 0 {
		fmt.Printf("No files in bucket %q.\n", bucket)
		return nil
	}

	// Build string rows for table and CSV output.
	cols := []string{"Name", "Size", "Created"}
	rows := make([][]string, len(result.Items))
	for i, f := range result.Items {
		rows[i] = []string{f.Name, formatBytes(f.Size), f.CreatedAt}
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
	fmt.Printf("\n%d file(s) in %q\n", result.TotalItems, bucket)
	return nil
}

// runStorageUpload uploads a file to a storage bucket by creating a multipart form request and posting it to the server's storage API.
func runStorageUpload(cmd *cobra.Command, args []string) error {
	bucket := args[0]
	filePath := args[1]
	outFmt := outputFormat(cmd)
	token, baseURL := resolveStorageRequestOptions(cmd)

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	requestBody, contentType := streamingMultipartFileBody("file", filepath.Base(filePath), f, nil)

	req, err := newAuthenticatedRequest(http.MethodPost, baseURL, "/api/storage/"+bucket, token, contentType, requestBody)
	if err != nil {
		return err
	}
	resp, respBody, err := doBufferedRequest(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return serverError(resp.StatusCode, respBody)
	}

	if outFmt == "json" {
		os.Stdout.Write(respBody)
		fmt.Println()
		return nil
	}

	var uploaded struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.Unmarshal(respBody, &uploaded); err != nil {
		return fmt.Errorf("parsing upload response: %w", err)
	}
	fmt.Printf("Uploaded %s (%s) to %s\n", uploaded.Name, formatBytes(uploaded.Size), bucket)
	return nil
}

// runStorageDownload downloads a file from a storage bucket, writing the contents to a specified output file or stdout.
func runStorageDownload(cmd *cobra.Command, args []string) error {
	bucket := args[0]
	name := args[1]
	output, _ := cmd.Flags().GetString("output")
	token, baseURL := resolveStorageRequestOptions(cmd)
	req, err := newAuthenticatedRequest(http.MethodGet, baseURL, "/api/storage/"+bucket+"/"+name, token, "", nil)
	if err != nil {
		return err
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return serverError(resp.StatusCode, body)
	}

	var dst io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		dst = f
	}

	n, err := io.Copy(dst, resp.Body)
	if err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	if output != "" {
		fmt.Fprintf(os.Stderr, "Downloaded %s (%s)\n", name, formatBytes(n))
	}
	return nil
}

func runStorageDelete(cmd *cobra.Command, args []string) error {
	bucket := args[0]
	name := args[1]

	resp, body, err := storageRequest(cmd, "DELETE", "/api/storage/"+bucket+"/"+name, nil, "")
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		fmt.Printf("Deleted %s/%s\n", bucket, name)
		return nil
	}
	return serverError(resp.StatusCode, body)
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// TODO: Document streamingMultipartFileBody.
func streamingMultipartFileBody(fileFieldName, fileName string, file io.Reader, formValues map[string]string) (io.Reader, string) {
	bodyReader, bodyWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(bodyWriter)
	contentType := multipartWriter.FormDataContentType()

	go func() {
		keys := make([]string, 0, len(formValues))
		for key := range formValues {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			if err := multipartWriter.WriteField(key, formValues[key]); err != nil {
				_ = bodyWriter.CloseWithError(err)
				return
			}
		}

		filePart, err := multipartWriter.CreateFormFile(fileFieldName, fileName)
		if err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}
		if _, err := io.Copy(filePart, file); err != nil {
			_ = bodyWriter.CloseWithError(err)
			return
		}

		_ = bodyWriter.CloseWithError(multipartWriter.Close())
	}()

	return bodyReader, contentType
}
