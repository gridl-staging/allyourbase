package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func parseUserInfo(provider string, body []byte) (*OAuthUserInfo, error) {
	parser, ok := getOAuthUserInfoParser(provider)
	if !ok || parser == nil {
		return nil, fmt.Errorf("%w: %s", ErrOAuthNotConfigured, provider)
	}
	info, err := parser(body)
	if err != nil {
		return nil, err
	}
	if info == nil || info.ProviderUserID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return info, nil
}

func applyUserInfoHeaders(req *http.Request, headers map[string]string, client OAuthClientConfig) {
	for key, value := range headers {
		if key == "" || value == "" {
			continue
		}
		req.Header.Set(key, resolveOAuthHeaderTemplate(value, client))
	}
}

func resolveOAuthHeaderTemplate(value string, client OAuthClientConfig) string {
	return strings.ReplaceAll(value, "{client_id}", client.ClientID)
}

// Parses user information from a Google OAuth response body and returns normalized user identification, email, and name.
func parseGoogleUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Google user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           u.Name,
	}, nil
}

// Parses user information from a GitHub API response body, using the login name as a fallback if the actual name is not provided, and returns normalized user identification and email.
func parseGitHubUserPayload(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing GitHub user: %w", err)
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}

	name := u.Name
	if name == "" {
		name = u.Login
	}

	return &OAuthUserInfo{
		ProviderUserID: fmt.Sprintf("%d", u.ID),
		Email:          u.Email,
		Name:           name,
	}, nil
}

// Parses user information from a Microsoft Graph API response body and returns normalized user identification, email, and display name.
func parseMicrosoftUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID                string `json:"id"`
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
		DisplayName       string `json:"displayName"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Microsoft user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}

	email := u.Mail
	if email == "" {
		email = u.UserPrincipalName
	}

	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          email,
		Name:           u.DisplayName,
	}, nil
}

// Fetches the primary verified email address for a GitHub user using their access token via the GitHub /user/emails endpoint.
func fetchGitHubPrimaryEmail(ctx context.Context, accessToken string, httpClient *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("emails endpoint returned %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no primary verified email")
}
