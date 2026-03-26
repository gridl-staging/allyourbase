// Package support provides a validation layer for managing support tickets. It includes both a full service backed by a store and a no-op implementation for disabled mode.
package support

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrEmptySubject     = errors.New("support subject cannot be empty")
	ErrEmptyBody        = errors.New("support body cannot be empty")
	ErrInvalidPriority  = errors.New("invalid support priority")
	ErrInvalidSender    = errors.New("invalid support sender type")
	ErrInvalidStatus    = errors.New("invalid support status")
	ErrNoTicketUpdates  = errors.New("at least one ticket field update is required")
	ErrStoreUnavailable = errors.New("support store is required")
)

// NewService creates a validation layer over the support store.
func NewService(store *Store) SupportService {
	return &service{store: store}
}

// NewNoopSupportService returns a disabled-mode implementation.
func NewNoopSupportService() SupportService {
	return &noopSupportService{}
}

type service struct {
	store *Store
}

type noopSupportService struct{}

// CreateTicket validates and creates a new support ticket. It requires non-empty subject and body, defaults priority to Normal if not provided, and validates that the priority is one of the allowed values. Returns an error if the store is unavailable or if any validation fails.
func (s *service) CreateTicket(ctx context.Context, tenantID, userID, subject, body, priority string) (*Ticket, error) {
	if s.store == nil {
		return nil, ErrStoreUnavailable
	}
	if strings.TrimSpace(subject) == "" {
		return nil, ErrEmptySubject
	}
	if strings.TrimSpace(body) == "" {
		return nil, ErrEmptyBody
	}

	normalizedPriority := priority
	if normalizedPriority == "" {
		normalizedPriority = TicketPriorityNormal
	}
	if !isAllowedPriority(normalizedPriority) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPriority, normalizedPriority)
	}

	return s.store.CreateTicket(ctx, tenantID, userID, subject, body, normalizedPriority)
}

func (s *service) ListTickets(ctx context.Context, tenantID string, filters TicketFilters) ([]Ticket, error) {
	if s.store == nil {
		return nil, ErrStoreUnavailable
	}
	return s.store.ListTickets(ctx, tenantID, filters)
}

func (s *service) GetTicket(ctx context.Context, ticketID string) (*Ticket, error) {
	if s.store == nil {
		return nil, ErrStoreUnavailable
	}
	return s.store.GetTicket(ctx, ticketID)
}

func (s *service) UpdateTicket(ctx context.Context, ticketID string, updates TicketUpdate) (*Ticket, error) {
	if s.store == nil {
		return nil, ErrStoreUnavailable
	}
	if updates.Status == nil && updates.Priority == nil {
		return nil, ErrNoTicketUpdates
	}
	if updates.Status != nil && !isAllowedStatus(*updates.Status) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidStatus, *updates.Status)
	}
	if updates.Priority != nil && !isAllowedPriority(*updates.Priority) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPriority, *updates.Priority)
	}
	return s.store.UpdateTicket(ctx, ticketID, updates)
}

func (s *service) AddMessage(ctx context.Context, ticketID, senderType, body string) (*Message, error) {
	if s.store == nil {
		return nil, ErrStoreUnavailable
	}
	if strings.TrimSpace(body) == "" {
		return nil, ErrEmptyBody
	}
	if !isAllowedSenderType(senderType) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidSender, senderType)
	}
	return s.store.AddMessage(ctx, ticketID, senderType, body)
}

func (s *service) ListMessages(ctx context.Context, ticketID string) ([]Message, error) {
	if s.store == nil {
		return nil, ErrStoreUnavailable
	}
	return s.store.ListMessages(ctx, ticketID)
}

func (s *noopSupportService) CreateTicket(ctx context.Context, tenantID, userID, subject, body, priority string) (*Ticket, error) {
	return nil, nil
}

func (s *noopSupportService) ListTickets(ctx context.Context, tenantID string, filters TicketFilters) ([]Ticket, error) {
	return []Ticket{}, nil
}

func (s *noopSupportService) GetTicket(ctx context.Context, ticketID string) (*Ticket, error) {
	return nil, nil
}

func (s *noopSupportService) UpdateTicket(ctx context.Context, ticketID string, updates TicketUpdate) (*Ticket, error) {
	return nil, nil
}

func (s *noopSupportService) AddMessage(ctx context.Context, ticketID, senderType, body string) (*Message, error) {
	return nil, nil
}

func (s *noopSupportService) ListMessages(ctx context.Context, ticketID string) ([]Message, error) {
	return []Message{}, nil
}

func isAllowedPriority(priority string) bool {
	switch priority {
	case TicketPriorityLow, TicketPriorityNormal, TicketPriorityHigh, TicketPriorityUrgent:
		return true
	default:
		return false
	}
}

func isAllowedStatus(status string) bool {
	switch status {
	case TicketStatusOpen, TicketStatusInProgress, TicketStatusWaitingOnCustomer, TicketStatusResolved, TicketStatusClosed:
		return true
	default:
		return false
	}
}

func isAllowedSenderType(senderType string) bool {
	switch senderType {
	case SenderCustomer, SenderSupport, SenderSystem:
		return true
	default:
		return false
	}
}
