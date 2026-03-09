package sms

import (
	"errors"

	"github.com/nyaruka/phonenumbers"
)

// ErrInvalidPhoneNumber is returned when a phone number cannot be parsed or validated.
var ErrInvalidPhoneNumber = errors.New("invalid phone number")

// NormalizePhone parses and validates a phone number using libphonenumber,
// returning E.164 format. Requires a '+' prefix (no default region).
func NormalizePhone(input string) (string, error) {
	// Pre-screen: only ASCII digits, '+', and formatting chars allowed.
	// Reject non-ASCII, multiple '+' signs, or missing '+' prefix.
	plusCount := 0
	for _, r := range input {
		switch {
		case r == '+':
			plusCount++
		case r >= '0' && r <= '9', r == ' ', r == '-', r == '(', r == ')', r == '.':
			// ok
		default:
			return "", ErrInvalidPhoneNumber
		}
	}
	if plusCount != 1 {
		return "", ErrInvalidPhoneNumber
	}

	num, err := phonenumbers.Parse(input, "")
	if err != nil {
		return "", ErrInvalidPhoneNumber
	}
	if !phonenumbers.IsValidNumber(num) {
		return "", ErrInvalidPhoneNumber
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}

// PhoneCountry returns the ISO 3166-1 alpha-2 country code for an E.164
// phone number, or "" if parsing fails.
func PhoneCountry(phone string) string {
	num, err := phonenumbers.Parse(phone, "")
	if err != nil {
		return ""
	}
	return phonenumbers.GetRegionCodeForNumber(num)
}

// IsAllowedCountry checks whether the phone's country matches one of the
// allowed country codes. An empty allowed list permits all.
func IsAllowedCountry(phone string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	region := PhoneCountry(phone)
	if region == "" {
		return false
	}
	for _, code := range allowed {
		if code == region {
			return true
		}
	}
	return false
}
