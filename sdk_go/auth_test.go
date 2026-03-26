package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthRegisterLoginMeRefreshLifecycle(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch step {
		case 0, 1, 3:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":        "tok1",
				"refreshToken": "ref1",
				"user": map[string]any{
					"id":    "usr_1",
					"email": "alice@example.com",
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "usr_1", "email": "alice@example.com"})
		}
		step++
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	if _, err := c.Auth.Register(context.Background(), "alice@example.com", "secret"); err != nil {
		t.Fatal(err)
	}
	res, err := c.Auth.Login(context.Background(), "alice@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "tok1" || c.Token() != "tok1" || c.RefreshToken() != "ref1" {
		t.Fatalf("tokens not set")
	}
	me, err := c.Auth.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Email != "alice@example.com" {
		t.Fatalf("unexpected me")
	}
	if _, err := c.Auth.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAuthLogoutDeleteAndUtilityEndpoints(t *testing.T) {
	step := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if step == 0 || step == 1 || step == 2 || step == 3 || step == 4 || step == 5 {
			w.WriteHeader(http.StatusNoContent)
		}
		step++
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	c.SetTokens("tok", "ref")
	if err := c.Auth.RequestPasswordReset(context.Background(), "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.ConfirmPasswordReset(context.Background(), "token", "password"); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.VerifyEmail(context.Background(), "token"); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.ResendVerification(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Auth.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Token() != "" {
		t.Fatalf("expected token cleared")
	}
	c.SetTokens("tok", "ref")
	if err := c.Auth.DeleteAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Token() != "" {
		t.Fatalf("expected token cleared")
	}
}
