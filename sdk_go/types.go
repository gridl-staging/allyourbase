// Package allyourbase Core data types for SDK operations including authentication, list responses, batch operations, and storage objects.
package allyourbase

import "encoding/json"

type User struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	EmailVerified *bool   `json:"emailVerified,omitempty"`
	CreatedAt     string  `json:"createdAt,omitempty"`
	UpdatedAt     *string `json:"updatedAt,omitempty"`
}

// UnmarshalJSON unmarshals JSON data into u, supporting both camelCase and snake_case naming conventions with camelCase taking precedence.
func (u *User) UnmarshalJSON(data []byte) error {
	type userWire struct {
		ID                 string  `json:"id"`
		Email              string  `json:"email"`
		EmailVerified      *bool   `json:"emailVerified"`
		EmailVerifiedSnake *bool   `json:"email_verified"`
		CreatedAt          string  `json:"createdAt"`
		CreatedAtSnake     string  `json:"created_at"`
		UpdatedAt          *string `json:"updatedAt"`
		UpdatedAtSnake     *string `json:"updated_at"`
	}

	var wire userWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	u.ID = wire.ID
	u.Email = wire.Email

	if wire.EmailVerified != nil {
		u.EmailVerified = wire.EmailVerified
	} else {
		u.EmailVerified = wire.EmailVerifiedSnake
	}

	if wire.CreatedAt != "" {
		u.CreatedAt = wire.CreatedAt
	} else {
		u.CreatedAt = wire.CreatedAtSnake
	}

	if wire.UpdatedAt != nil {
		u.UpdatedAt = wire.UpdatedAt
	} else {
		u.UpdatedAt = wire.UpdatedAtSnake
	}

	return nil
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	User         User   `json:"user"`
}

type ListResponse struct {
	Items      []map[string]any `json:"items"`
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

type BatchOperation struct {
	Method string         `json:"method"`
	ID     string         `json:"id,omitempty"`
	Body   map[string]any `json:"body,omitempty"`
}

type BatchResult struct {
	Index  int            `json:"index"`
	Status int            `json:"status"`
	Body   map[string]any `json:"body,omitempty"`
}

type StorageObject struct {
	ID          string  `json:"id"`
	Bucket      string  `json:"bucket"`
	Name        string  `json:"name"`
	Size        int64   `json:"size"`
	ContentType string  `json:"contentType"`
	UserID      *string `json:"userId,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   *string `json:"updatedAt,omitempty"`
}

type StorageListResponse struct {
	Items      []StorageObject `json:"items"`
	TotalItems int             `json:"totalItems"`
}
