package cli

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

// snsPublisherAdapter wraps the AWS SNS client to implement sms.SNSPublisher.
type snsPublisherAdapter struct {
	client *sns.Client
}

func newSNSPublisher(region string) (*snsPublisherAdapter, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return &snsPublisherAdapter{client: sns.NewFromConfig(cfg)}, nil
}

func (a *snsPublisherAdapter) Publish(ctx context.Context, phoneNumber, message string) (string, error) {
	out, err := a.client.Publish(ctx, &sns.PublishInput{
		PhoneNumber: &phoneNumber,
		Message:     &message,
	})
	if err != nil {
		return "", err
	}
	if out.MessageId == nil {
		return "", nil
	}
	return *out.MessageId, nil
}
