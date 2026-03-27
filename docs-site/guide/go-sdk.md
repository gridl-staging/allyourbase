<!-- audited 2026-03-21 -->
# Go SDK

Use `sdk_go` for idiomatic Go access to AYB auth, records, storage, and edge functions.

## Install

Public `go get` is not available yet because the canonical public module path is still being finalized. Until that publish path is settled, use a local checkout of the public repo and point your app's module at the checked-out SDK.

```bash
git clone https://github.com/gridlhq/allyourbase.git

# inside your app module
go mod edit -replace=github.com/allyourbase/ayb/sdk_go=/absolute/path/to/allyourbase/sdk_go
go get github.com/allyourbase/ayb/sdk_go
```

## Initialize

```go
package main

import (
	"context"
	"fmt"

	allyourbase "github.com/allyourbase/ayb/sdk_go"
)

func main() {
	ctx := context.Background()
	client := allyourbase.NewClient("http://localhost:8090")

	result, err := client.Records.List(ctx, "posts", allyourbase.ListParams{PerPage: 20})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(result.Items))
}
```

### Client options

```go
client := allyourbase.NewClient(
	"https://api.example.com",
	allyourbase.WithHTTPClient(&http.Client{}),
	allyourbase.WithUserAgent("my-app/1.0"),
	allyourbase.WithAPIKey("ayb_api_key_xxx"),
)
```

## Context and timeouts

All SDK calls accept `context.Context`. Use per-request deadlines to avoid hanging calls.

```go
package main

import (
	"context"
	"fmt"
	"time"

	allyourbase "github.com/allyourbase/ayb/sdk_go"
)

func listWithTimeout(client *allyourbase.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	list, err := client.Records.List(ctx, "posts", allyourbase.ListParams{PerPage: 20})
	if err != nil {
		return err
	}

	fmt.Println("items:", len(list.Items))
	return nil
}
```

## Shared setup for snippet blocks

Unless a snippet declares its own `package main`, assume it runs inside `main()` after this setup:

```go
ctx := context.Background()
client := allyourbase.NewClient("http://localhost:8090")
```

All networked snippets below are runtime-dependent and should be validated against a live AYB server.

## Records

### List

```go
list, err := client.Records.List(ctx, "posts", allyourbase.ListParams{
	Filter:  "published=true",
	Sort:    "-created_at",
	PerPage: 50,
})
```

### Iterate all pages

```go
package main

import (
	"context"
	"fmt"

	allyourbase "github.com/allyourbase/ayb/sdk_go"
)

func listAllPosts(ctx context.Context, client *allyourbase.Client) error {
	page := 1
	perPage := 100

	for {
		res, err := client.Records.List(ctx, "posts", allyourbase.ListParams{
			Page:    page,
			PerPage: perPage,
			Sort:    "-created_at",
		})
		if err != nil {
			return err
		}

		for _, item := range res.Items {
			fmt.Println(item["id"])
		}

		if page >= res.TotalPages || len(res.Items) == 0 {
			return nil
		}
		page++
	}
}
```

### Get/Create/Update/Delete

```go
post, err := client.Records.Get(ctx, "posts", "42", allyourbase.GetParams{})

created, err := client.Records.Create(ctx, "posts", map[string]any{
	"title": "Hello",
})

updated, err := client.Records.Update(ctx, "posts", "42", map[string]any{
	"title": "Updated",
})

err = client.Records.Delete(ctx, "posts", "42")
```

### Batch

```go
results, err := client.Records.Batch(ctx, "posts", []allyourbase.BatchOperation{
	{Method: "create", Body: map[string]any{"title": "A"}},
	{Method: "update", ID: "42", Body: map[string]any{"title": "B"}},
})
```

## Auth

```go
_, err := client.Auth.Login(ctx, "user@example.com", "password")
if err != nil {
	panic(err)
}

me, err := client.Auth.Me(ctx)
_, err = client.Auth.Refresh(ctx)
err = client.Auth.Logout(ctx)
```

Also available:

- `Register`
- `DeleteAccount`
- `RequestPasswordReset`
- `ConfirmPasswordReset`
- `VerifyEmail`
- `ResendVerification`

## Storage

```go
avatarBytes := []byte("png-bytes-go-here")

uploaded, err := client.Storage.Upload(ctx, "avatars", "me.png", avatarBytes, "image/png")
if err != nil {
	panic(err)
}

raw, err := client.Storage.Download(ctx, "avatars", uploaded.Name)

objects, err := client.Storage.List(ctx, "avatars", allyourbase.StorageListParams{
	Prefix: "user_",
	Limit:  20,
})

_ = raw
_ = objects
```

## Edge functions

```go
resp, err := client.Edge.Invoke(ctx, "hello", allyourbase.EdgeInvokeRequest{
	Method: "POST",
	Body:   []byte(`{"name":"stuart"}`),
	Headers: map[string]string{
		"Content-Type": "application/json",
	},
})
if err != nil {
	panic(err)
}
fmt.Println(resp.StatusCode, string(resp.Body))
```

## Errors

The SDK normalizes API failures into `*allyourbase.Error` with `Status`, `Code`, `Message`, `Data`, and `DocURL`.
Its `Error()` format is `AYBError(status=<status>, message=<message>)`.

```go
package main

import (
	"context"
	"errors"
	"fmt"

	allyourbase "github.com/allyourbase/ayb/sdk_go"
)

func printAPIError(ctx context.Context, client *allyourbase.Client) {
	_, err := client.Records.Get(ctx, "posts", "missing", allyourbase.GetParams{})
	if err != nil {
		var apiErr *allyourbase.Error
		if errors.As(err, &apiErr) {
			fmt.Println(apiErr.Status, apiErr.Code, apiErr.Message)
		}
	}
}
```

### Retry-worthy vs terminal

Use status codes to distinguish retry strategy:

- Retry-worthy: transport errors, context timeout/cancellation, `429`, and `5xx`.
- Usually terminal: `400`, `401`, `403`, `404`, `409`, and validation/business-rule failures.

```go
package main

import (
	"context"
	"errors"

	allyourbase "github.com/allyourbase/ayb/sdk_go"
)

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	var apiErr *allyourbase.Error
	if errors.As(err, &apiErr) {
		return apiErr.Status == 429 || apiErr.Status >= 500
	}
	return true // network / unknown transport error
}
```
