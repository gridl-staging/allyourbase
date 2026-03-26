package allyourbase

import (
	"context"
	"encoding/json"
	"net/http"
)

type AuthClient struct {
	client *Client
}

func (a *AuthClient) Register(ctx context.Context, email, password string) (*AuthResponse, error) {
	return a.authWithCredentials(ctx, "/api/auth/register", email, password)
}

func (a *AuthClient) Login(ctx context.Context, email, password string) (*AuthResponse, error) {
	return a.authWithCredentials(ctx, "/api/auth/login", email, password)
}

func (a *AuthClient) authWithCredentials(ctx context.Context, path, email, password string) (*AuthResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, path, nil, map[string]string{"email": email, "password": password})
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

func (a *AuthClient) Me(ctx context.Context) (*User, error) {
	body, err := a.client.doJSON(ctx, http.MethodGet, "/api/auth/me", nil, nil)
	if err != nil {
		return nil, err
	}
	var out User
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *AuthClient) Refresh(ctx context.Context) (*AuthResponse, error) {
	body, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/refresh", nil, map[string]string{"refreshToken": a.client.RefreshToken()})
	if err != nil {
		return nil, err
	}
	var out AuthResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	a.client.SetTokens(out.Token, out.RefreshToken)
	return &out, nil
}

func (a *AuthClient) Logout(ctx context.Context) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/logout", nil, map[string]string{"refreshToken": a.client.RefreshToken()})
	if err != nil {
		return err
	}
	a.client.ClearTokens()
	return nil
}

func (a *AuthClient) DeleteAccount(ctx context.Context) error {
	_, err := a.client.doJSON(ctx, http.MethodDelete, "/api/auth/me", nil, nil)
	if err != nil {
		return err
	}
	a.client.ClearTokens()
	return nil
}

func (a *AuthClient) RequestPasswordReset(ctx context.Context, email string) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/password-reset", nil, map[string]string{"email": email})
	return err
}

func (a *AuthClient) ConfirmPasswordReset(ctx context.Context, token, password string) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/password-reset/confirm", nil, map[string]string{"token": token, "password": password})
	return err
}

func (a *AuthClient) VerifyEmail(ctx context.Context, token string) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/verify", nil, map[string]string{"token": token})
	return err
}

func (a *AuthClient) ResendVerification(ctx context.Context) error {
	_, err := a.client.doJSON(ctx, http.MethodPost, "/api/auth/verify/resend", nil, nil)
	return err
}
