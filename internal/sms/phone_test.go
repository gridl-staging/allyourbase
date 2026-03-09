package sms

import (
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Phone normalization ---

func TestNormalizePhone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input, want string
	}{
		{"+1 415 555 2671", "+14155552671"},
		{"+1-415-555-2671", "+14155552671"},
		{"+44 20 7946 0958", "+442079460958"},
		{"+14155552671", "+14155552671"},
		{"+(1) 415-555-2671", "+14155552671"},
	}
	for _, c := range cases {
		got, err := NormalizePhone(c.input)
		testutil.NoError(t, err)
		testutil.Equal(t, c.want, got)
	}
}

func TestNormalizePhone_RejectsInvalid(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"4155552671",            // no +
		"+1",                    // too short
		"+1234567890123456",     // too long (>15 digits)
		"+abc",                  // non-digits
		"",                      // empty
		"not-a-phone",           // garbage
		"+1+4155552671",         // multiple + signs
		"++14155552671",         // double + at start
		"+\u0661\u0662\u0663\u0664\u0665\u0666\u0667\u0668\u0669\u0660", // Arabic-Indic digits (non-ASCII)
	}
	for _, p := range invalid {
		_, err := NormalizePhone(p)
		if !errors.Is(err, ErrInvalidPhoneNumber) {
			t.Errorf("NormalizePhone(%q): got %v, want ErrInvalidPhoneNumber", p, err)
		}
	}
}

func TestNormalizePhone_LibPhoneNumber(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input, want string
	}{
		// National-style with country code
		{"+1 (415) 555-2671", "+14155552671"},
		// International with dots
		{"+44.20.7946.0958", "+442079460958"},
		// Already E.164
		{"+61412345678", "+61412345678"},
		// With spaces and dashes
		{"+49 30 1234 5678", "+493012345678"},
		// With parentheses
		{"+(33) 1 23 45 67 89", "+33123456789"},
	}
	for _, c := range cases {
		got, err := NormalizePhone(c.input)
		testutil.NoError(t, err)
		testutil.Equal(t, c.want, got)
	}
}

func TestNormalizePhone_RejectsInvalidForCountry(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"+19995551234",  // correct digit count but unassigned US area code (999)
		"+449999999999", // invalid UK number (no valid area code 999)
		"+61012345678",  // invalid AU mobile (starts with 0 after country code)
	}
	for _, p := range invalid {
		_, err := NormalizePhone(p)
		if !errors.Is(err, ErrInvalidPhoneNumber) {
			t.Errorf("NormalizePhone(%q): got %v, want ErrInvalidPhoneNumber", p, err)
		}
	}
}

func TestNormalizePhone_AcceptsGlobalNumbers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input, want string
	}{
		{"+919876543210", "+919876543210"},   // India
		{"+818012345678", "+818012345678"},   // Japan
		{"+5511987654321", "+5511987654321"}, // Brazil
		{"+2348031234567", "+2348031234567"}, // Nigeria
		{"+27821234567", "+27821234567"},     // South Africa
		{"+6591234567", "+6591234567"},       // Singapore
	}
	for _, c := range cases {
		got, err := NormalizePhone(c.input)
		testutil.NoError(t, err)
		testutil.Equal(t, c.want, got)
	}
}

// --- Phone country detection ---

func TestPhoneCountry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		phone, want string
	}{
		{"+14155552671", "US"},
		{"+442079460958", "GB"},
		{"+919876543210", "IN"},
		{"+5511987654321", "BR"},
		{"+818012345678", "JP"},
	}
	for _, c := range cases {
		got := PhoneCountry(c.phone)
		testutil.Equal(t, c.want, got)
	}
}

func TestPhoneCountry_DistinguishesNANP(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "US", PhoneCountry("+14155552671"))
	testutil.Equal(t, "CA", PhoneCountry("+16135550123"))
}

func TestPhoneCountry_Caribbean(t *testing.T) {
	t.Parallel()
	region := PhoneCountry("+18765551234") // Jamaica
	testutil.True(t, region != "US", "Jamaica should not be US, got %s", region)
	testutil.True(t, region != "CA", "Jamaica should not be CA, got %s", region)
	testutil.Equal(t, "JM", region)
}

func TestPhoneCountry_InvalidInputs(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "", PhoneCountry(""))
	testutil.Equal(t, "", PhoneCountry("abc"))
	testutil.Equal(t, "", PhoneCountry("+1"))
	testutil.Equal(t, "", PhoneCountry("not-a-phone"))
}

// --- Country allow-list ---

func TestIsAllowedCountry_UnparseablePhone(t *testing.T) {
	t.Parallel()
	testutil.False(t, IsAllowedCountry("garbage", []string{"US"}), "unparseable phone should be blocked")
	testutil.False(t, IsAllowedCountry("", []string{"US"}), "empty phone should be blocked")
}

func TestIsAllowedCountry(t *testing.T) {
	t.Parallel()
	// Empty list allows all.
	testutil.True(t, IsAllowedCountry("+14155552671", nil), "empty list should allow all")
	testutil.True(t, IsAllowedCountry("+442079460958", []string{}), "empty list should allow all")

	// Explicit list.
	allowed := []string{"US", "GB"}
	testutil.True(t, IsAllowedCountry("+14155552671", allowed), "US number should be allowed")
	testutil.True(t, IsAllowedCountry("+442079460958", allowed), "UK number should be allowed")
	testutil.False(t, IsAllowedCountry("+919876543210", allowed), "IN number should be blocked")
	testutil.False(t, IsAllowedCountry("+4915112345678", allowed), "DE number should be blocked")

	// Unknown country code in list — doesn't panic, just blocks.
	testutil.False(t, IsAllowedCountry("+14155552671", []string{"XX"}), "unknown code should block")
}

func TestIsAllowedCountry_WithPhoneNumbers(t *testing.T) {
	t.Parallel()
	// US number allowed when only US in list.
	testutil.True(t, IsAllowedCountry("+14155552671", []string{"US"}), "US number should be allowed")
	// Canadian number blocked when only US is allowed (both share +1).
	testutil.False(t, IsAllowedCountry("+16135550123", []string{"US"}), "CA number should be blocked when only US allowed")
	// Canadian number allowed when CA in list.
	testutil.True(t, IsAllowedCountry("+16135550123", []string{"CA"}), "CA number should be allowed")
	// Jamaican number blocked when only US and CA.
	testutil.False(t, IsAllowedCountry("+18765551234", []string{"US", "CA"}), "JM number should be blocked")
}
