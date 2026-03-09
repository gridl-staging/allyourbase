package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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

func storageRequest(cmd *cobra.Command, method, path string, body io.Reader, contentType string) (*http.Response, []byte, error) {
	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")

	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}
	return resp, respBody, nil
}

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

func runStorageUpload(cmd *cobra.Command, args []string) error {
	bucket := args[0]
	filePath := args[1]
	outFmt := outputFormat(cmd)

	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")
	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	// Build multipart form.
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(writer.Close())
	}()

	req, err := http.NewRequest("POST", baseURL+"/api/storage/"+bucket, pr)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := cliHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
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

func runStorageDownload(cmd *cobra.Command, args []string) error {
	bucket := args[0]
	name := args[1]
	output, _ := cmd.Flags().GetString("output")

	token, _ := cmd.Flags().GetString("admin-token")
	baseURL, _ := cmd.Flags().GetString("url")
	if token == "" {
		token = os.Getenv("AYB_ADMIN_TOKEN")
	}
	if baseURL == "" {
		baseURL = serverURL()
	}

	req, err := http.NewRequest("GET", baseURL+"/api/storage/"+bucket+"/"+name, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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
