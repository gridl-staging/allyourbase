package storage

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"reflect"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestCloudFront_PublicURLToInvalidationPath(t *testing.T) {
	t.Parallel()

	path, err := publicURLToCloudFrontInvalidationPath("https://cdn.example.com/assets/image.png?size=large")
	testutil.NoError(t, err)
	testutil.Equal(t, "/assets/image.png?size=large", path)
}

func TestCloudFront_PurgeURLs_BatchesAt3000(t *testing.T) {
	t.Parallel()

	calls := 0
	batchSizes := make(map[int]struct{})

	client := &fakeCloudFrontClient{}
	client.createFn = func(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
		calls++
		quantity := int32(0)
		if params.InvalidationBatch.Paths.Quantity != nil {
			quantity = *params.InvalidationBatch.Paths.Quantity
		}
		batchSizes[int(quantity)] = struct{}{}
		return &cloudfront.CreateInvalidationOutput{}, nil
	}

	urls := make([]string, 0, 3001)
	for i := 0; i < 3001; i++ {
		urls = append(urls, fmt.Sprintf("https://cdn.example.com/path/%d", i))
	}

	provider := NewCloudFrontCDNProvider(CloudFrontCDNOptions{
		DistributionID: "dist-1",
		Client:         client,
		MaxRetries:     0,
	})

	err := provider.PurgeURLs(context.Background(), urls)
	testutil.NoError(t, err)
	testutil.Equal(t, 2, calls)
	if !reflect.DeepEqual(map[int]struct{}{1: {}, 3000: {}}, batchSizes) {
		t.Fatalf("expected batch sizes {1,3000}, got %v", batchSizes)
	}
}

func TestCloudFront_PurgeAll_UsesWildcardPath(t *testing.T) {
	t.Parallel()

	var paths []string
	client := &fakeCloudFrontClient{}
	client.createFn = func(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
		paths = append(paths, params.InvalidationBatch.Paths.Items...)
		return &cloudfront.CreateInvalidationOutput{}, nil
	}

	provider := NewCloudFrontCDNProvider(CloudFrontCDNOptions{DistributionID: "dist-1", Client: client})
	err := provider.PurgeAll(context.Background())
	testutil.NoError(t, err)
	if !reflect.DeepEqual([]string{cloudfrontWildcardPath}, paths) {
		t.Fatalf("expected wildcard path %q, got %v", cloudfrontWildcardPath, paths)
	}
}

func TestCloudFront_CallerReferencesAreUnique(t *testing.T) {
	t.Parallel()

	refs := make(map[string]struct{})
	client := &fakeCloudFrontClient{}
	client.createFn = func(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
		if params.InvalidationBatch == nil || params.InvalidationBatch.CallerReference == nil {
			return nil, errors.New("missing caller reference")
		}
		refs[*params.InvalidationBatch.CallerReference] = struct{}{}
		return &cloudfront.CreateInvalidationOutput{}, nil
	}

	provider := NewCloudFrontCDNProvider(CloudFrontCDNOptions{DistributionID: "dist-1", Client: client})
	err := provider.PurgeAll(context.Background())
	testutil.NoError(t, err)
	err = provider.PurgeAll(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, len(refs))
}

func TestCloudFront_PurgeURLs_TransientErrorsRetry(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := &fakeCloudFrontClient{}
	client.createFn = func(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
		attempts++
		if attempts == 1 {
			return nil, &smithyhttp.ResponseError{
				Response: &smithyhttp.Response{
					Response: &stdhttp.Response{
						StatusCode: stdhttp.StatusServiceUnavailable,
					},
				},
				Err: errors.New("service unavailable"),
			}
		}
		return &cloudfront.CreateInvalidationOutput{}, nil
	}

	provider := NewCloudFrontCDNProvider(CloudFrontCDNOptions{DistributionID: "dist-1", Client: client, MaxRetries: 3})
	err := provider.PurgeURLs(context.Background(), []string{"https://cdn.example.com/a"})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, attempts)
}

func TestCloudFront_PurgeURLs_TooManyInvalidationsInProgressRetries(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := &fakeCloudFrontClient{}
	client.createFn = func(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
		attempts++
		if attempts == 1 {
			return nil, &cloudfronttypes.TooManyInvalidationsInProgress{Message: aws.String("still processing")}
		}
		return &cloudfront.CreateInvalidationOutput{}, nil
	}

	provider := NewCloudFrontCDNProvider(CloudFrontCDNOptions{DistributionID: "dist-1", Client: client})
	err := provider.PurgeURLs(context.Background(), []string{"https://cdn.example.com/a"})
	testutil.NoError(t, err)
	testutil.Equal(t, 2, attempts)
}

func TestCloudFront_PurgeURLs_PermanentErrorsFailFast(t *testing.T) {
	t.Parallel()

	attempts := 0
	client := &fakeCloudFrontClient{}
	client.createFn = func(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
		attempts++
		return nil, &smithy.GenericAPIError{Code: "AccessDenied", Message: "denied", Fault: smithy.FaultClient}
	}

	provider := NewCloudFrontCDNProvider(CloudFrontCDNOptions{DistributionID: "dist-1", Client: client, MaxRetries: 3})
	err := provider.PurgeURLs(context.Background(), []string{"https://cdn.example.com/a"})
	testutil.ErrorContains(t, err, "denied")
	testutil.Equal(t, 1, attempts)
}

type fakeCloudFrontClient struct {
	createFn func(context.Context, *cloudfront.CreateInvalidationInput, ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error)
}

func (f *fakeCloudFrontClient) CreateInvalidation(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
	if f.createFn == nil {
		return nil, errors.New("no fake create function")
	}
	return f.createFn(ctx, params, optFns...)
}
