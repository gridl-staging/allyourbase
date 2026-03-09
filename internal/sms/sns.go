package sms

import (
	"context"
	"fmt"
)

// SNSPublisher abstracts the AWS SNS Publish call for testability.
type SNSPublisher interface {
	Publish(ctx context.Context, phoneNumber, message string) (messageID string, err error)
}

// SNSProvider sends SMS via AWS SNS.
type SNSProvider struct {
	publisher SNSPublisher
}

// NewSNSProvider creates an SNSProvider with the given publisher.
func NewSNSProvider(publisher SNSPublisher) *SNSProvider {
	return &SNSProvider{publisher: publisher}
}

func (p *SNSProvider) Send(ctx context.Context, to, body string) (*SendResult, error) {
	messageID, err := p.publisher.Publish(ctx, to, body)
	if err != nil {
		return nil, fmt.Errorf("sns: publish: %w", err)
	}

	return &SendResult{
		MessageID: messageID,
		Status:    "sent",
	}, nil
}
