# Storage Test Specification

**User Stories:** B-STOR-001 through B-STOR-005
**Read this file BEFORE implementing storage tests**

---

## Overview

Storage (file upload/download) has three test levels:
1. **Integration tests** (Go) — Test storage handlers with real Postgres + local/S3 storage
2. **Unit tests** (SDK) — Test SDK storage methods with mocked fetch
3. **Browser tests (unmocked) tests** (Playwright) — Test full storage flow through admin UI

**This spec focuses on integration and Browser tests (unmocked) tests.**

---

## <a id="upload-file"></a>TEST: Upload File (Integration)

**BDD Story:** B-STOR-001
**Type:** Integration test
**File:** `internal/storage/handler_integration_test.go`

### Prerequisites
- Test database
- Storage backend configured (local or S3)
- Test bucket created

### Test Cases

#### 1. Upload File with Valid Data

**Fixture:** `tests/fixtures/storage/upload-image.json`
```json
{
  "metadata": {
    "description": "Upload valid PNG image",
    "expected_response_status": 200,
    "expected_fields": ["name", "size", "content_type", "url", "created_at"],
    "expected_content_type": "image/png",
    "expected_size_bytes": 1024,
    "expected_bucket": "test-bucket"
  },
  "bucket": "test-bucket",
  "filename": "test-image.png",
  "file_content": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
}
```

**Execute:**
```go
fixture := loadFixture("storage/upload-image.json")

// Decode base64 file content
fileBytes, _ := base64.StdEncoding.DecodeString(fixture.FileContent)

// Create multipart form
body := new(bytes.Buffer)
writer := multipart.NewWriter(body)
part, _ := writer.CreateFormFile("file", fixture.Filename)
part.Write(fileBytes)
writer.Close()

// Upload file
resp := makeRequest("POST", "/api/storage/"+fixture.Bucket, body, map[string]string{
    "Content-Type": writer.FormDataContentType(),
})
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

// Verify response fields
testutil.True(t, result["name"] != nil)
testutil.Equal(t, fixture.Metadata.ExpectedContentType, result["content_type"])
testutil.True(t, result["size"].(float64) > 0)
testutil.True(t, result["url"] != nil)

// Verify file exists on disk/S3
storageObject := result["name"].(string)
filePath := filepath.Join(storagePath, fixture.Bucket, storageObject)
testutil.True(t, fileExists(filePath))
```

**Cleanup:**
```go
t.Cleanup(func() {
    deleteFile(filepath.Join(storagePath, fixture.Bucket, storageObject))
})
```

---

#### 2. Upload File Exceeding Size Limit

**Fixture:** `tests/fixtures/storage/upload-too-large.json`
```json
{
  "metadata": {
    "description": "Upload file exceeding max size limit",
    "expected_response_status": 413,
    "expected_error_message": "file too large",
    "max_file_size_mb": 10
  },
  "bucket": "test-bucket",
  "filename": "huge-file.bin",
  "file_size_mb": 15
}
```

**Execute:**
```go
// Create a large file buffer (15MB)
largeFile := make([]byte, 15*1024*1024)

// Upload
resp := makeRequestWithFile("POST", "/api/storage/test-bucket", "file", "huge-file.bin", largeFile)
```

**Verify:**
```go
testutil.Equal(t, 413, resp.StatusCode)

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)
testutil.Contains(t, result["error"], "file too large")
```

**Cleanup:** None (file not uploaded)

---

#### 3. Upload to Non-Existent Bucket (Local Storage)

**Fixture:** `tests/fixtures/storage/upload-auto-create-bucket.json`
```json
{
  "metadata": {
    "description": "Upload to non-existent bucket (auto-created on local storage)",
    "expected_response_status": 200,
    "expected_bucket_created": true
  },
  "bucket": "new-test-bucket",
  "filename": "test.txt",
  "file_content": "dGVzdCBjb250ZW50"
}
```

**Execute & Verify:** Bucket auto-created on local storage, 200 response

---

## <a id="download-file"></a>TEST: Download File (Integration)

**BDD Story:** B-STOR-002
**Type:** Integration test

### Test Cases

#### 1. Download Existing File

**Fixture:** `tests/fixtures/storage/download-valid.json`
```json
{
  "metadata": {
    "description": "Download existing file",
    "expected_response_status": 200,
    "expected_content_type": "image/png",
    "expected_content": "test file content"
  },
  "bucket": "test-bucket",
  "filename": "test-file.png"
}
```

**Execute:**
```go
// Setup: Upload file first
fixture := loadFixture("storage/download-valid.json")
uploadedFile := uploadTestFile(t, fixture.Bucket, fixture.Filename, []byte(fixture.Metadata.ExpectedContent))

// Download file
resp := makeRequest("GET", "/api/storage/"+fixture.Bucket+"/"+uploadedFile.Name, nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)
testutil.Equal(t, fixture.Metadata.ExpectedContentType, resp.Header.Get("Content-Type"))

body, _ := io.ReadAll(resp.Body)
testutil.Equal(t, fixture.Metadata.ExpectedContent, string(body))
```

**Cleanup:** Delete test file

---

#### 2. Download Non-Existent File

**Fixture:** `tests/fixtures/storage/download-non-existent.json`
```json
{
  "metadata": {
    "description": "Download non-existent file",
    "expected_response_status": 404,
    "expected_error_message": "file not found"
  },
  "bucket": "test-bucket",
  "filename": "non-existent.txt"
}
```

**Execute & Verify:** Return 404 with error

---

## <a id="list-files"></a>TEST: List Files (Integration)

**BDD Story:** B-STOR-003
**Type:** Integration test

### Test Cases

#### 1. List Files in Bucket

**Fixture:** `tests/fixtures/storage/list-files.json`
```json
{
  "metadata": {
    "description": "List files in bucket with 5 files",
    "expected_response_status": 200,
    "expected_total_items": 5,
    "expected_fields": ["name", "size", "content_type", "url", "created_at"]
  },
  "bucket": "test-bucket",
  "files": [
    {"filename": "file1.txt", "content": "content1"},
    {"filename": "file2.txt", "content": "content2"},
    {"filename": "file3.txt", "content": "content3"},
    {"filename": "file4.txt", "content": "content4"},
    {"filename": "file5.txt", "content": "content5"}
  ]
}
```

**Execute:**
```go
// Setup: Upload 5 test files
fixture := loadFixture("storage/list-files.json")
for _, file := range fixture.Files {
    uploadTestFile(t, fixture.Bucket, file.Filename, []byte(file.Content))
}

// List files
resp := makeRequest("GET", "/api/storage/"+fixture.Bucket, nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

items := result["items"].([]interface{})
testutil.Equal(t, fixture.Metadata.ExpectedTotalItems, len(items))

// Verify first item has expected fields
firstItem := items[0].(map[string]interface{})
for _, field := range fixture.Metadata.ExpectedFields {
    testutil.True(t, firstItem[field] != nil)
}
```

**Cleanup:** Delete test files

---

#### 2. List Files in Empty Bucket

**Fixture:** `tests/fixtures/storage/list-empty-bucket.json`
```json
{
  "metadata": {
    "description": "List files in empty bucket",
    "expected_response_status": 200,
    "expected_total_items": 0
  },
  "bucket": "empty-bucket"
}
```

**Execute & Verify:** Return empty items array with totalItems=0

---

## <a id="delete-file"></a>TEST: Delete File (Integration)

**BDD Story:** B-STOR-004
**Type:** Integration test

### Test Cases

#### 1. Delete Existing File

**Fixture:** `tests/fixtures/storage/delete-valid.json`
```json
{
  "metadata": {
    "description": "Delete existing file",
    "expected_response_status": 204
  },
  "bucket": "test-bucket",
  "filename": "file-to-delete.txt",
  "content": "This file will be deleted"
}
```

**Execute:**
```go
// Setup: Upload file
fixture := loadFixture("storage/delete-valid.json")
uploadedFile := uploadTestFile(t, fixture.Bucket, fixture.Filename, []byte(fixture.Content))

// Delete file
resp := makeRequest("DELETE", "/api/storage/"+fixture.Bucket+"/"+uploadedFile.Name, nil)
```

**Verify:**
```go
testutil.Equal(t, 204, resp.StatusCode)

// Verify file is deleted
filePath := filepath.Join(storagePath, fixture.Bucket, uploadedFile.Name)
testutil.False(t, fileExists(filePath))
```

---

#### 2. Delete Non-Existent File

**Fixture:** `tests/fixtures/storage/delete-non-existent.json`
```json
{
  "metadata": {
    "description": "Delete non-existent file",
    "expected_response_status": 404,
    "expected_error_message": "file not found"
  },
  "bucket": "test-bucket",
  "filename": "non-existent.txt"
}
```

**Execute & Verify:** Return 404 with error

---

## <a id="generate-signed-url"></a>TEST: Generate Signed URL (Integration)

**BDD Story:** B-STOR-005
**Type:** Integration test

### Test Cases

#### 1. Generate Signed URL with Default Expiration

**Fixture:** `tests/fixtures/storage/sign-url-default.json`
```json
{
  "metadata": {
    "description": "Generate signed URL with default 1 hour expiration",
    "expected_response_status": 200,
    "expected_url_pattern": "^https?://.+\\?.*signature=.+&expires=.+$",
    "expected_default_expires_in": 3600
  },
  "bucket": "test-bucket",
  "filename": "private-file.txt",
  "content": "Private content"
}
```

**Execute:**
```go
// Setup: Upload file
fixture := loadFixture("storage/sign-url-default.json")
uploadedFile := uploadTestFile(t, fixture.Bucket, fixture.Filename, []byte(fixture.Content))

// Generate signed URL
resp := makeRequest("POST", "/api/storage/"+fixture.Bucket+"/"+uploadedFile.Name+"/sign", nil)
```

**Verify:**
```go
testutil.Equal(t, 200, resp.StatusCode)

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

signedURL := result["url"].(string)
testutil.True(t, signedURL != "")

// Verify URL format includes signature and expires
re := regexp.MustCompile(fixture.Metadata.ExpectedURLPattern)
testutil.True(t, re.MatchString(signedURL))

// Verify signed URL works (can download without auth)
downloadResp, _ := http.Get(signedURL)
testutil.Equal(t, 200, downloadResp.StatusCode)
```

**Cleanup:** Delete test file

---

#### 2. Generate Signed URL with Custom Expiration

**Fixture:** `tests/fixtures/storage/sign-url-custom-expiry.json`
```json
{
  "metadata": {
    "description": "Generate signed URL with custom 5 minute expiration",
    "expected_response_status": 200,
    "expected_expires_in": 300
  },
  "bucket": "test-bucket",
  "filename": "private-file.txt",
  "content": "Private content",
  "expires_in": 300
}
```

**Execute & Verify:** Same pattern as above with custom expiresIn parameter

---

#### 3. Signed URL Expires After Timeout

**Fixture:** `tests/fixtures/storage/sign-url-expired.json`
```json
{
  "metadata": {
    "description": "Signed URL becomes invalid after expiration",
    "expected_response_status": 200,
    "expected_expires_in": 1,
    "expected_expired_download_status": 403
  },
  "bucket": "test-bucket",
  "filename": "private-file.txt",
  "content": "Private content",
  "expires_in": 1
}
```

**Execute:**
```go
// Setup: Upload file and generate signed URL with 1 second expiration
fixture := loadFixture("storage/sign-url-expired.json")
uploadedFile := uploadTestFile(t, fixture.Bucket, fixture.Filename, []byte(fixture.Content))

resp := makeRequest("POST", "/api/storage/"+fixture.Bucket+"/"+uploadedFile.Name+"/sign",
    map[string]int{"expiresIn": fixture.ExpiresIn})

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)
signedURL := result["url"].(string)

// Wait for URL to expire
time.Sleep(2 * time.Second)

// Try to download with expired URL
downloadResp, _ := http.Get(signedURL)
testutil.Equal(t, 403, downloadResp.StatusCode)
```

**Cleanup:** Delete test file

---

## Browser tests (unmocked) Tests

**Location:** `ui/browser-tests-unmocked/smoke/storage-upload.spec.ts`
**Purpose:** Test full storage flow through admin UI

### Test Cases

#### 1. Upload File via UI

**Execute:**
1. Navigate to admin dashboard
2. Click "Storage" in navigation
3. Select or create bucket
4. Click "Upload" button
5. Select file from file picker
6. Click "Submit"

**Verify:**
- File appears in storage list
- File has correct name and size
- File can be downloaded

#### 2. Delete File via UI

**Execute:**
1. Click on file row
2. Click "Delete" button
3. Confirm deletion

**Verify:**
- File removed from list
- File no longer accessible

---

## Fixture Data Needed

**Create these fixtures in `tests/fixtures/storage/`:**

1. `upload-image.json` — Valid PNG upload
2. `upload-too-large.json` — File exceeding size limit
3. `upload-auto-create-bucket.json` — Upload to new bucket
4. `download-valid.json` — Download existing file
5. `download-non-existent.json` — Download missing file
6. `list-files.json` — List 5 files in bucket
7. `list-empty-bucket.json` — List empty bucket
8. `delete-valid.json` — Delete existing file
9. `delete-non-existent.json` — Delete missing file
10. `sign-url-default.json` — Generate signed URL (1 hour)
11. `sign-url-custom-expiry.json` — Generate signed URL (5 min)
12. `sign-url-expired.json` — Test expired signed URL

---

**Spec Version:** 1.0
**Last Updated:** 2026-02-13 (Session 078)
