package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- OTP generation ---

func TestGenerateOTP(t *testing.T) {
	t.Parallel()
	for length := 4; length <= 8; length++ {
		code, err := generateOTP(length)
		testutil.NoError(t, err)
		testutil.Equal(t, length, len(code))
		for _, c := range code {
			testutil.True(t, c >= '0' && c <= '9', "expected digit, got %c", c)
		}
	}
}

func TestGenerateOTPIsRandom(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		code, err := generateOTP(6)
		testutil.NoError(t, err)
		seen[code] = struct{}{}
	}
	testutil.True(t, len(seen) > 50, "OTPs should not repeat heavily, got %d unique out of 100", len(seen))
}

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
		got, err := normalizePhone(c.input)
		testutil.NoError(t, err)
		testutil.Equal(t, c.want, got)
	}
}

func TestNormalizePhoneRejectsInvalid(t *testing.T) {
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
		_, err := normalizePhone(p)
		if !errors.Is(err, ErrInvalidPhoneNumber) {
			t.Errorf("normalizePhone(%q): got %v, want ErrInvalidPhoneNumber", p, err)
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
		got, err := normalizePhone(c.input)
		testutil.NoError(t, err)
		testutil.Equal(t, c.want, got)
	}
}

func TestNormalizePhone_RejectsInvalidForCountry(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"+19995551234",   // correct digit count but unassigned US area code (999)
		"+449999999999",  // invalid UK number (no valid area code 999)
		"+61012345678",   // invalid AU mobile (starts with 0 after country code)
	}
	for _, p := range invalid {
		_, err := normalizePhone(p)
		if !errors.Is(err, ErrInvalidPhoneNumber) {
			t.Errorf("normalizePhone(%q): got %v, want ErrInvalidPhoneNumber", p, err)
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
		got, err := normalizePhone(c.input)
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
		got := phoneCountry(c.phone)
		testutil.Equal(t, c.want, got)
	}
}

func TestPhoneCountry_DistinguishesNANP(t *testing.T) {
	t.Parallel()
	testutil.Equal(t, "US", phoneCountry("+14155552671"))
	testutil.Equal(t, "CA", phoneCountry("+16135550123"))
}

func TestPhoneCountry_Caribbean(t *testing.T) {
	t.Parallel()
	region := phoneCountry("+18765551234") // Jamaica
	testutil.True(t, region != "US", "Jamaica should not be US, got %s", region)
	testutil.True(t, region != "CA", "Jamaica should not be CA, got %s", region)
	testutil.Equal(t, "JM", region)
}

func TestPhoneCountry_InvalidInputs(t *testing.T) {
	t.Parallel()
	// phoneCountry should return "" for unparseable inputs, never panic.
	testutil.Equal(t, "", phoneCountry(""))
	testutil.Equal(t, "", phoneCountry("abc"))
	testutil.Equal(t, "", phoneCountry("+1"))
	testutil.Equal(t, "", phoneCountry("not-a-phone"))
}

// --- Country allow-list ---

func TestIsAllowedCountry_UnparseablePhone(t *testing.T) {
	t.Parallel()
	// Unparseable phone with a non-empty allowed list should return false, not panic.
	testutil.False(t, isAllowedCountry("garbage", []string{"US"}), "unparseable phone should be blocked")
	testutil.False(t, isAllowedCountry("", []string{"US"}), "empty phone should be blocked")
}

func TestIsAllowedCountry(t *testing.T) {
	t.Parallel()
	// Empty list allows all.
	testutil.True(t, isAllowedCountry("+14155552671", nil), "empty list should allow all")
	testutil.True(t, isAllowedCountry("+442079460958", []string{}), "empty list should allow all")

	// Explicit list.
	allowed := []string{"US", "GB"}
	testutil.True(t, isAllowedCountry("+14155552671", allowed), "US number should be allowed")
	testutil.True(t, isAllowedCountry("+442079460958", allowed), "UK number should be allowed")
	testutil.False(t, isAllowedCountry("+919876543210", allowed), "IN number should be blocked")
	testutil.False(t, isAllowedCountry("+4915112345678", allowed), "DE number should be blocked")

	// Unknown country code in list — doesn't panic, just blocks.
	testutil.False(t, isAllowedCountry("+14155552671", []string{"XX"}), "unknown code should block")
}

func TestIsAllowedCountry_WithPhoneNumbers(t *testing.T) {
	t.Parallel()
	// US number allowed when only US in list.
	testutil.True(t, isAllowedCountry("+14155552671", []string{"US"}), "US number should be allowed")
	// Canadian number blocked when only US is allowed (both share +1).
	testutil.False(t, isAllowedCountry("+16135550123", []string{"US"}), "CA number should be blocked when only US allowed")
	// Canadian number allowed when CA in list.
	testutil.True(t, isAllowedCountry("+16135550123", []string{"CA"}), "CA number should be allowed")
	// Jamaican number blocked when only US and CA.
	testutil.False(t, isAllowedCountry("+18765551234", []string{"US", "CA"}), "JM number should be blocked")
}

// --- SMS request handler ---

func newSMSHandler(enabled bool) *Handler {
	svc := newTestService()
	h := NewHandler(svc, testutil.DiscardLogger())
	h.smsEnabled = enabled
	return h
}

func TestHandleSMSRequest_DisabledReturns404(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(false)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/sms",
		strings.NewReader(`{"phone":"+14155552671"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

func TestHandleSMSRequest_MissingPhone_Returns400(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/sms",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSMSRequest_InvalidPhoneFormat_Returns400(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/sms",
		strings.NewReader(`{"phone":"not-a-phone"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSMSRequest_ValidPhoneAlwaysReturns200(t *testing.T) {
	// Even with no provider/DB, the endpoint should return 200 (anti-enumeration).
	t.Parallel()
	h := newSMSHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/sms",
		strings.NewReader(`{"phone":"+14155552671"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)
	testutil.Contains(t, w.Body.String(), "verification code has been sent")
}

func TestHandleSMSRequest_MalformedJSON_Returns400(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/sms",
		strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

// --- SMS confirm handler ---

func TestHandleSMSConfirm_Disabled_Returns404(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(false)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/sms/confirm",
		strings.NewReader(`{"phone":"+14155552671","code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
	testutil.Contains(t, w.Body.String(), "not enabled")
}

func TestHandleSMSConfirm_MissingFields_Returns400(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(true)
	router := h.Routes()

	// Missing code
	req := httptest.NewRequest(http.MethodPost, "/sms/confirm",
		strings.NewReader(`{"phone":"+14155552671"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	testutil.Equal(t, http.StatusBadRequest, w.Code)

	// Missing phone
	req2 := httptest.NewRequest(http.MethodPost, "/sms/confirm",
		strings.NewReader(`{"code":"123456"}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	testutil.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestHandleSMSConfirm_MalformedJSON_Returns400(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(true)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/sms/confirm",
		strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	testutil.Equal(t, http.StatusBadRequest, w.Code)
	testutil.Contains(t, w.Body.String(), "invalid JSON body")
}

// --- Route registration ---

func TestSMSRoutesRegistered(t *testing.T) {
	t.Parallel()
	h := newSMSHandler(true)
	router := h.Routes()

	// POST /sms should work (not 404/405).
	req := httptest.NewRequest(http.MethodPost, "/sms",
		strings.NewReader(`{"phone":"+14155552671"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	testutil.True(t, w.Code != http.StatusMethodNotAllowed,
		"POST /sms should be registered, got %d", w.Code)

	// POST /sms/confirm should work (not 405).
	req2 := httptest.NewRequest(http.MethodPost, "/sms/confirm",
		strings.NewReader(`{}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	testutil.True(t, w2.Code != http.StatusMethodNotAllowed,
		"POST /sms/confirm should be registered, got %d", w2.Code)
	testutil.Equal(t, http.StatusBadRequest, w2.Code)
}

// --- SetSMSEnabled ---

func TestSetSMSEnabled(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(), testutil.DiscardLogger())
	testutil.False(t, h.smsEnabled, "SMS should be disabled by default")

	h.SetSMSEnabled(true)
	testutil.True(t, h.smsEnabled, "SMS should be enabled after SetSMSEnabled(true)")

	h.SetSMSEnabled(false)
	testutil.False(t, h.smsEnabled, "SMS should be disabled after SetSMSEnabled(false)")
}
