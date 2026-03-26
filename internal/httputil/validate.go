package httputil

import (
	"fmt"
	"net/mail"
	"strings"
)

// ValidateEmail performs strict email validation for API boundaries.
func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if strings.ContainsAny(email, "\r\n") {
		return fmt.Errorf("invalid email format")
	}

	addr, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("invalid email format")
	}
	if addr.Address != email {
		return fmt.Errorf("invalid email format")
	}

	atIdx := strings.LastIndex(email, "@")
	if atIdx < 1 || atIdx == len(email)-1 {
		return fmt.Errorf("invalid email format")
	}
	domain := email[atIdx+1:]
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return fmt.Errorf("invalid email format")
	}
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// ValidateEmailLoose performs permissive validation used by auth flows.
func ValidateEmailLoose(email string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if strings.ContainsAny(email, "\r\n") {
		return fmt.Errorf("invalid email format")
	}
	atIdx := strings.Index(email, "@")
	if atIdx < 1 {
		return fmt.Errorf("invalid email format")
	}
	domain := email[atIdx+1:]
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("invalid email format")
	}
	return nil
}
