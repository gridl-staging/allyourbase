package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/cli/ui"
	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/testutil"
)

// --- redactURL tests ---

func TestRedactURLStripsCredentials(t *testing.T) {
	got := redactURL("postgres://user:secret@host:5432/mydb")
	testutil.Contains(t, got, "***")
	testutil.True(t, !strings.Contains(got, "secret"), "secret should be redacted")
	testutil.True(t, !strings.Contains(got, "user"), "username should be redacted")
	testutil.Contains(t, got, "host:5432/mydb")
}

func TestRedactURLStripsUserOnly(t *testing.T) {
	got := redactURL("postgres://admin@host:5432/db")
	testutil.Contains(t, got, "***")
	testutil.True(t, !strings.Contains(got, "admin"), "username should be redacted")
	testutil.Contains(t, got, "host:5432/db")
}

func TestRedactURLPassesThroughNoCredentials(t *testing.T) {
	got := redactURL("postgres://host:5432/db")
	testutil.Equal(t, "postgres://host:5432/db", got)
}

func TestRedactURLReturnsStarsOnInvalidURL(t *testing.T) {
	got := redactURL("://not a valid url")
	testutil.Equal(t, "***", got)
}

// bannerToString runs printBannerTo with a bytes.Buffer to capture output.
func bannerToString(cfg *config.Config, embeddedPG bool, useColor bool) string {
	return bannerToStringWithPassword(cfg, embeddedPG, useColor, "")
}

func bannerToStringWithPassword(cfg *config.Config, embeddedPG bool, useColor bool, generatedPassword string) string {
	var buf bytes.Buffer
	printBannerTo(&buf, cfg, embeddedPG, useColor, generatedPassword, "")
	return buf.String()
}

func defaultTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host: "0.0.0.0",
			Port: 8090,
		},
		Admin: config.AdminConfig{
			Enabled: true,
			Path:    "/admin",
		},
	}
}

func TestBannerContainsVersion(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "Allyourbase v")
}

func TestBannerContainsBrandEmoji(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, ui.BrandEmoji)
}

func TestBannerContainsAPIURL(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "http://localhost:8090/api")
}

func TestBannerContainsAdminURL(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "http://localhost:8090/admin")
}

func TestBannerHidesAdminWhenDisabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Admin.Enabled = false
	out := bannerToString(cfg, false, false)
	testutil.False(t, strings.Contains(out, "Admin:"))
}

func TestBannerShowsManagedDatabase(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, true, false)
	testutil.Contains(t, out, "managed")
}

func TestBannerShowsExternalDatabase(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "external")
}

func TestBannerContainsHints(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "Try:")
	testutil.Contains(t, out, "ayb sql")
	testutil.Contains(t, out, "CREATE TABLE")
	testutil.Contains(t, out, "ayb schema")
}

func TestBannerContainsDocsLink(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "https://allyourbase.io/guide/quickstart")
}

func TestBannerNoColorHasNoANSI(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.False(t, strings.Contains(out, "\033["))
}

func TestBannerWithColorHasANSI(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, true)
	testutil.Contains(t, out, "\033[")
}

func TestBannerCustomPort(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.Port = 3000
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "http://localhost:3000/api")
}

func TestBannerWithSiteURL(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.SiteURL = "https://myapp.example.com"
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "https://myapp.example.com/api")
	testutil.Contains(t, out, "https://myapp.example.com/admin")
}

func TestBannerCustomAdminPath(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Admin.Path = "/dashboard"
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "/dashboard")
}

func TestBannerShowsGeneratedPassword(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToStringWithPassword(cfg, false, false, "abc123secret")
	testutil.Contains(t, out, "Admin password:")
	testutil.Contains(t, out, "abc123secret")
}

func TestBannerHidesPasswordWhenNotGenerated(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.False(t, strings.Contains(out, "Admin password:"))
}

func TestBannerShowsPasswordWarningWhenLow(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Auth.MinPasswordLength = 3
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "WARNING")
	testutil.Contains(t, out, "min_password_length is 3")
	testutil.Contains(t, out, "Not suitable for production")
}

func TestBannerHidesPasswordWarningAtDefault(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Auth.MinPasswordLength = 8
	out := bannerToString(cfg, false, false)
	testutil.False(t, strings.Contains(out, "WARNING"))
	testutil.False(t, strings.Contains(out, "min_password_length"))
}

func TestBannerHidesPasswordWarningAboveDefault(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Auth.MinPasswordLength = 12
	out := bannerToString(cfg, false, false)
	testutil.False(t, strings.Contains(out, "WARNING"))
}

func TestBannerStripsDoubleV(t *testing.T) {
	// When buildVersion includes "v" prefix (from git tag), banner should not produce "vv".
	oldVersion := buildVersion
	buildVersion = "v0.1.0"
	defer func() { buildVersion = oldVersion }()

	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "Allyourbase v0.1.0")
	testutil.False(t, strings.Contains(out, "vv0.1.0"))
}

func TestBannerDevBuildShowsDev(t *testing.T) {
	oldVersion := buildVersion
	buildVersion = "v0.1.0-43-ge534c04-dirty"
	defer func() { buildVersion = oldVersion }()

	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "Allyourbase v0.1.0-dev")
	testutil.False(t, strings.Contains(out, "ge534c04"))
}

func TestBannerVersionCleanTag(t *testing.T) {
	testutil.Equal(t, "0.1.0", bannerVersion("v0.1.0"))
	testutil.Equal(t, "0.1.0", bannerVersion("0.1.0"))
}

func TestBannerVersionDevBuild(t *testing.T) {
	testutil.Equal(t, "0.1.0-dev", bannerVersion("v0.1.0-43-ge534c04"))
	testutil.Equal(t, "0.1.0-dev", bannerVersion("v0.1.0-43-ge534c04-dirty"))
	testutil.Equal(t, "1.2.3-dev", bannerVersion("v1.2.3-1-gabcdef0"))
}

func TestBannerVersionPreRelease(t *testing.T) {
	// Semver pre-release labels (e.g. "beta.1") should be preserved, not turned into -dev.
	testutil.Equal(t, "0.1.0-beta.1", bannerVersion("v0.1.0-beta.1"))
	testutil.Equal(t, "1.0.0-rc.2", bannerVersion("v1.0.0-rc.2"))
}

func TestBannerVersionDev(t *testing.T) {
	testutil.Equal(t, "dev", bannerVersion("dev"))
}

func TestBannerShowsResetHint(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToStringWithPassword(cfg, false, false, "abc123secret")
	testutil.Contains(t, out, "ayb admin reset-password")
}

func TestBannerHidesResetHintWithoutPassword(t *testing.T) {
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	testutil.False(t, strings.Contains(out, "reset-password"))
}

func TestBannerCodeLinesNoPadding(t *testing.T) {
	// Example code lines should start at column 0 for easy copy-paste.
	cfg := defaultTestConfig()
	out := bannerToString(cfg, false, false)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "ayb sql") {
			testutil.True(t, strings.HasPrefix(line, "ayb"), "code line should start at column 0, got: %q", line)
		}
		if strings.Contains(line, "ayb schema") {
			testutil.True(t, strings.HasPrefix(line, "ayb"), "ayb schema line should start at column 0, got: %q", line)
		}
	}
}

// --- TLS banner tests ---

func TestBannerUsesHTTPSWhenTLSEnabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = "api.myapp.com"
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "https://api.myapp.com/api")
	testutil.False(t, strings.Contains(out, "http://"), "banner should not contain http:// when TLS enabled")
}

func TestBannerAdminUsesHTTPSWhenTLSEnabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = "api.myapp.com"
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "https://api.myapp.com/admin")
}

func TestBannerHintsAppearWhenTLSEnabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Server.TLSEnabled = true
	cfg.Server.TLSDomain = "api.myapp.com"
	out := bannerToString(cfg, false, false)
	// CLI hints should still appear with TLS enabled.
	testutil.Contains(t, out, "ayb schema")
	testutil.Contains(t, out, "ayb sql")
}

func TestBannerUsesHTTPWhenTLSDisabled(t *testing.T) {
	cfg := defaultTestConfig()
	// TLS not configured — should use http:// with host:port as before.
	out := bannerToString(cfg, false, false)
	testutil.Contains(t, out, "http://localhost:8090/api")
	// API/admin URLs must not be https:// when TLS is off.
	// (The docs link is always https://allyourbase.io, which is fine.)
	testutil.False(t, strings.Contains(out, "https://localhost"), "API URL should not use https:// when TLS disabled")
}

func TestBannerHintNeverShowsToken(t *testing.T) {
	// Token is stored in ~/.ayb/admin-token, not shown in the banner hint.
	cfg := defaultTestConfig()
	out := bannerToStringWithPassword(cfg, false, false, "mytoken123")
	testutil.False(t, strings.Contains(out, "--admin-token"))
	testutil.Contains(t, out, `ayb sql`)
}
