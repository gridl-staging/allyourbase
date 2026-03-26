package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/allyourbase/ayb/internal/testutil"
)

type fakeDNSResolver struct {
	records []string
	err     error
	calls   []string
}

func (f *fakeDNSResolver) LookupTXT(_ context.Context, hostname string) ([]string, error) {
	f.calls = append(f.calls, hostname)
	return f.records, f.err
}

type enqueueCall struct {
	jobType string
	payload json.RawMessage
	opts    jobs.EnqueueOpts
}

type fakeJobEnqueuer struct {
	calls []enqueueCall
	err   error
}

func (f *fakeJobEnqueuer) Enqueue(_ context.Context, jobType string, payload json.RawMessage, opts jobs.EnqueueOpts) (*jobs.Job, error) {
	f.calls = append(f.calls, enqueueCall{jobType: jobType, payload: append(json.RawMessage(nil), payload...), opts: opts})
	if f.err != nil {
		return nil, f.err
	}
	return &jobs.Job{ID: "job_1"}, nil
}

func TestVerifyRetryDelay(t *testing.T) {
	t.Parallel()
	d1 := verifyRetryDelay(1)
	d20 := verifyRetryDelay(20)
	d21 := verifyRetryDelay(21)

	testutil.True(t, d1 >= 30*time.Second && d1 <= 32*time.Second, "attempt 1 delay out of range: %s", d1)
	testutil.True(t, d20 >= 30*time.Second && d20 <= 32*time.Second, "attempt 20 delay out of range: %s", d20)
	testutil.True(t, d21 >= 5*time.Minute && d21 <= 5*time.Minute+10*time.Second, "attempt 21 delay out of range: %s", d21)
}

func TestVerifyTimedOut(t *testing.T) {
	t.Parallel()
	testutil.True(t, verifyTimedOut(time.Now().Add(-25*time.Hour)), "expected timeout for 25h-old domain")
	testutil.True(t, !verifyTimedOut(time.Now().Add(-23*time.Hour)), "did not expect timeout for 23h-old domain")
}

func TestDomainDNSVerifyHandlerSuccess(t *testing.T) {
	t.Parallel()

	domainID := "00000000-0000-0000-0000-000000000001"
	resolver := &fakeDNSResolver{records: []string{"wrong", "token123"}}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{
		domains: []DomainBinding{{
			ID:                domainID,
			Hostname:          "example.com",
			Status:            StatusPendingVerification,
			VerificationToken: "token123",
			LastError:         strptr("old error"),
		}},
	}
	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: domainID, StartedAt: time.Now(), Attempt: 1}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 0, len(enqueuer.calls))
	b, err := mgr.GetDomain(context.Background(), domainID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusVerified, b.Status)
	testutil.Equal(t, (*string)(nil), b.LastError)
}

func TestDomainDNSVerifyHandlerTokenMismatchRetries(t *testing.T) {
	t.Parallel()

	domainID := "00000000-0000-0000-0000-000000000001"
	resolver := &fakeDNSResolver{records: []string{"wrong", "also-wrong"}}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{
		domains: []DomainBinding{{
			ID:                domainID,
			Hostname:          "example.com",
			Status:            StatusPendingVerification,
			VerificationToken: "expected-token",
		}},
	}
	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: domainID, StartedAt: time.Now(), Attempt: 1}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 1, len(enqueuer.calls))
	testutil.Equal(t, JobTypeDomainDNSVerify, enqueuer.calls[0].jobType)
	var next domainVerifyPayload
	testutil.NoError(t, json.Unmarshal(enqueuer.calls[0].payload, &next))
	testutil.Equal(t, domainID, next.DomainID)
	testutil.Equal(t, 2, next.Attempt)
	testutil.True(t, enqueuer.calls[0].opts.MaxAttempts == 1, "expected max attempts 1")
	testutil.True(t, enqueuer.calls[0].opts.RunAt != nil, "expected runAt to be set")
	delay := time.Until(*enqueuer.calls[0].opts.RunAt)
	testutil.True(t, delay >= 29*time.Second && delay <= 32*time.Second, "expected short retry delay, got: %s", delay)

	b, err := mgr.GetDomain(context.Background(), domainID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusPendingVerification, b.Status)
	testutil.True(t, b.LastError != nil, "expected last_error")
	testutil.Contains(t, *b.LastError, "does not match")
}

func TestDomainDNSVerifyHandlerNoTXTRetries(t *testing.T) {
	t.Parallel()

	domainID := "00000000-0000-0000-0000-000000000001"
	resolver := &fakeDNSResolver{records: []string{}}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{
		domains: []DomainBinding{{
			ID:                domainID,
			Hostname:          "example.com",
			Status:            StatusPendingVerification,
			VerificationToken: "expected-token",
		}},
	}
	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: domainID, StartedAt: time.Now(), Attempt: 3}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 1, len(enqueuer.calls))
	b, err := mgr.GetDomain(context.Background(), domainID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusPendingVerification, b.Status)
	testutil.True(t, b.LastError != nil, "expected last_error")
	testutil.Contains(t, *b.LastError, "no TXT record found")
}

func TestDomainDNSVerifyHandlerLookupErrorRetries(t *testing.T) {
	t.Parallel()

	domainID := "00000000-0000-0000-0000-000000000001"
	resolverErr := fmt.Errorf("temporary dns failure")
	resolver := &fakeDNSResolver{err: resolverErr}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{
		domains: []DomainBinding{{
			ID:                domainID,
			Hostname:          "example.com",
			Status:            StatusPendingVerification,
			VerificationToken: "expected-token",
		}},
	}
	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: domainID, StartedAt: time.Now(), Attempt: 1}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 1, len(enqueuer.calls))
	b, err := mgr.GetDomain(context.Background(), domainID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusPendingVerification, b.Status)
	testutil.True(t, b.LastError != nil)
	testutil.Contains(t, *b.LastError, "temporary dns failure")
}

func TestDomainDNSVerifyHandlerTimeout(t *testing.T) {
	t.Parallel()

	domainID := "00000000-0000-0000-0000-000000000001"
	resolver := &fakeDNSResolver{records: []string{"wrong"}}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{
		domains: []DomainBinding{{
			ID:                domainID,
			Hostname:          "example.com",
			Status:            StatusPendingVerification,
			VerificationToken: "expected-token",
		}},
	}
	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: domainID, StartedAt: time.Now().Add(-25 * time.Hour), Attempt: 1}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 0, len(enqueuer.calls))
	b, err := mgr.GetDomain(context.Background(), domainID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusVerificationFailed, b.Status)
	testutil.True(t, b.LastError != nil)
	testutil.Contains(t, *b.LastError, "does not match")
}

func TestDomainDNSVerifyHandlerStaleVerified(t *testing.T) {
	t.Parallel()

	domainID := "00000000-0000-0000-0000-000000000001"
	resolver := &fakeDNSResolver{records: []string{"wrong"}}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{
		domains: []DomainBinding{{
			ID:                domainID,
			Hostname:          "example.com",
			Status:            StatusVerified,
			VerificationToken: "expected-token",
		}},
	}
	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: domainID, StartedAt: time.Now().Add(-25 * time.Hour), Attempt: 1}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 0, len(enqueuer.calls))
	testutil.Equal(t, 0, len(resolver.calls))
	b, err := mgr.GetDomain(context.Background(), domainID)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusVerified, b.Status)
}

func TestDomainDNSVerifyHandlerStaleTombstoned(t *testing.T) {
	t.Parallel()

	domainID := "00000000-0000-0000-0000-000000000001"
	resolver := &fakeDNSResolver{records: []string{"wrong"}}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{
		domains: []DomainBinding{{
			ID:                domainID,
			Hostname:          "example.com",
			Status:            StatusTombstoned,
			VerificationToken: "expected-token",
		}},
	}
	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: domainID, StartedAt: time.Now().Add(-25 * time.Hour), Attempt: 1}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 0, len(enqueuer.calls))
	testutil.Equal(t, 0, len(resolver.calls))
}

func TestDomainDNSVerifyHandlerDomainNotFound(t *testing.T) {
	t.Parallel()

	resolver := &fakeDNSResolver{records: []string{"token123"}}
	enqueuer := &fakeJobEnqueuer{}
	mgr := &fakeDomainManager{} // empty — no domains

	h := DomainDNSVerifyHandler(mgr, resolver, enqueuer, nil)

	payload := domainVerifyPayload{DomainID: "00000000-0000-0000-0000-000000000099", StartedAt: time.Now(), Attempt: 1}
	raw, err := json.Marshal(payload)
	testutil.NoError(t, err)

	testutil.NoError(t, h(context.Background(), raw))

	testutil.Equal(t, 0, len(enqueuer.calls))
	testutil.Equal(t, 0, len(resolver.calls))
}

func TestUpdateDomainStatusViaFakeManager(t *testing.T) {
	t.Parallel()

	mgr := &fakeDomainManager{domains: []DomainBinding{{
		ID:        "00000000-0000-0000-0000-000000000001",
		Hostname:  "example.com",
		Status:    StatusPendingVerification,
		LastError: strptr("old"),
	}}}

	updated, err := mgr.UpdateDomainStatus(context.Background(), "00000000-0000-0000-0000-000000000001", StatusVerified, nil)
	testutil.NoError(t, err)
	testutil.Equal(t, StatusVerified, updated.Status)
	testutil.Equal(t, (*string)(nil), updated.LastError)
	testutil.Equal(t, StatusVerified, mgr.domains[0].Status)
	testutil.Equal(t, (*string)(nil), mgr.domains[0].LastError)

	_, err = mgr.UpdateDomainStatus(context.Background(), "missing", StatusVerified, nil)
	testutil.True(t, err != nil, "expected not found error")
	testutil.Equal(t, ErrDomainNotFound, err)
}
