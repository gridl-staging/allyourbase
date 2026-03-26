package cli

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/push"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBuildPushProvidersFallbackToLog(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	logger := slog.Default()

	providers := buildPushProviders(cfg, logger)
	testutil.Equal(t, 2, len(providers))

	_, fcmOK := providers[push.ProviderFCM].(*push.LogProvider)
	testutil.True(t, fcmOK, "expected fcm provider fallback to *push.LogProvider")

	_, apnsOK := providers[push.ProviderAPNS].(*push.LogProvider)
	testutil.True(t, apnsOK, "expected apns provider fallback to *push.LogProvider")
}

func TestBuildPushProvidersConfiguredFCM(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	cfg.Push.FCM.CredentialsFile = writeTestFCMCredentials(t)
	logger := slog.Default()

	providers := buildPushProviders(cfg, logger)

	_, fcmOK := providers[push.ProviderFCM].(*push.FCMProvider)
	testutil.True(t, fcmOK, "expected configured fcm provider to be *push.FCMProvider")

	_, apnsOK := providers[push.ProviderAPNS].(*push.LogProvider)
	testutil.True(t, apnsOK, "expected apns fallback to *push.LogProvider when not configured")
}

func TestBuildPushProvidersConfiguredAPNS(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	cfg.Push.APNS.KeyFile = writeTestAPNSKey(t)
	cfg.Push.APNS.TeamID = "TEAM123"
	cfg.Push.APNS.KeyID = "KEY123"
	cfg.Push.APNS.BundleID = "com.example.app"
	cfg.Push.APNS.Environment = "sandbox"
	logger := slog.Default()

	providers := buildPushProviders(cfg, logger)

	_, apnsOK := providers[push.ProviderAPNS].(*push.APNSProvider)
	testutil.True(t, apnsOK, "expected configured apns provider to be *push.APNSProvider")

	_, fcmOK := providers[push.ProviderFCM].(*push.LogProvider)
	testutil.True(t, fcmOK, "expected fcm fallback to *push.LogProvider when not configured")
}

func TestPushProviderNamesSorted(t *testing.T) {
	t.Parallel()

	providers := map[string]push.Provider{
		push.ProviderFCM:  &push.CaptureProvider{},
		push.ProviderAPNS: push.NewLogProvider(slog.Default()),
	}

	names := pushProviderNames(providers)
	testutil.Equal(t, 2, len(names))
	testutil.Equal(t, "apns", names[0])
	testutil.Equal(t, "fcm", names[1])
}

type stubScheduleUpserter struct {
	upsertFn func(ctx context.Context, sched *jobs.Schedule) (*jobs.Schedule, error)
}

func (s *stubScheduleUpserter) UpsertSchedule(ctx context.Context, sched *jobs.Schedule) (*jobs.Schedule, error) {
	if s.upsertFn != nil {
		return s.upsertFn(ctx, sched)
	}
	return sched, nil
}

func TestRegisterPushTokenCleanupSchedule(t *testing.T) {
	t.Parallel()

	upserter := &stubScheduleUpserter{
		upsertFn: func(ctx context.Context, sched *jobs.Schedule) (*jobs.Schedule, error) {
			testutil.Equal(t, "push_token_cleanup_daily", sched.Name)
			testutil.Equal(t, push.JobTypePushTokenClean, sched.JobType)
			testutil.Equal(t, "0 2 * * *", sched.CronExpr)
			testutil.Equal(t, "UTC", sched.Timezone)
			testutil.True(t, sched.Enabled, "expected schedule to be enabled")
			testutil.Equal(t, 3, sched.MaxAttempts)
			testutil.NotNil(t, sched.NextRunAt)
			return sched, nil
		},
	}

	registerPushTokenCleanupSchedule(context.Background(), upserter, slog.Default())
}

func writeTestFCMCredentials(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	testutil.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	testutil.NoError(t, err)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	})

	creds := map[string]string{
		"project_id":   "proj-test",
		"client_email": "svc@proj-test.iam.gserviceaccount.com",
		"private_key":  string(privateKeyPEM),
		"token_uri":    "https://oauth2.googleapis.com/token",
	}
	raw, err := json.Marshal(creds)
	testutil.NoError(t, err)

	path := filepath.Join(t.TempDir(), "fcm-creds.json")
	testutil.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func writeTestAPNSKey(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	testutil.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	testutil.NoError(t, err)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	})

	path := filepath.Join(t.TempDir(), "AuthKey_TEST.p8")
	testutil.NoError(t, os.WriteFile(path, privateKeyPEM, 0o600))
	return path
}
