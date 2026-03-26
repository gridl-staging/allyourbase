// Package auth oauth_standard_providers.go defines OAuth2 provider configurations and userinfo parsers for standard platforms including Discord, Twitter, Facebook, LinkedIn, Spotify, Twitch, GitLab, Bitbucket, Slack, Zoom, Notion, and Figma. Each parser normalizes the provider's response format to a common OAuthUserInfo structure.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Standard OAuth2 provider configs and userinfo parsers.
// Each provider follows the pattern: register in oauthProviders + oauthUserInfoParsers,
// implement a parseXxxUser function that normalizes to OAuthUserInfo.

func init() {
	oauthMu.Lock()
	defer oauthMu.Unlock()

	providers := map[string]OAuthProviderConfig{
		"discord": {
			AuthURL:     "https://discord.com/api/oauth2/authorize",
			TokenURL:    "https://discord.com/api/oauth2/token",
			UserInfoURL: "https://discord.com/api/users/@me",
			Scopes:      []string{"identify", "email"},
		},
		"twitter": {
			AuthURL:         "https://twitter.com/i/oauth2/authorize",
			TokenURL:        "https://api.twitter.com/2/oauth2/token",
			UserInfoURL:     "https://api.twitter.com/2/users/me?user.fields=profile_image_url",
			Scopes:          []string{"users.read", "tweet.read", "offline.access"},
			TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
		},
		"facebook": {
			AuthURL:     "https://www.facebook.com/v22.0/dialog/oauth",
			TokenURL:    "https://graph.facebook.com/v22.0/oauth/access_token",
			UserInfoURL: "https://graph.facebook.com/v22.0/me?fields=id,name,email,picture",
			Scopes:      []string{"email", "public_profile"},
		},
		"linkedin": {
			AuthURL:     "https://www.linkedin.com/oauth/v2/authorization",
			TokenURL:    "https://www.linkedin.com/oauth/v2/accessToken",
			UserInfoURL: "https://api.linkedin.com/v2/userinfo",
			Scopes:      []string{"openid", "profile", "email"},
		},
		"spotify": {
			AuthURL:     "https://accounts.spotify.com/authorize",
			TokenURL:    "https://accounts.spotify.com/api/token",
			UserInfoURL: "https://api.spotify.com/v1/me",
			Scopes:      []string{"user-read-email", "user-read-private"},
		},
		"twitch": {
			AuthURL:     "https://id.twitch.tv/oauth2/authorize",
			TokenURL:    "https://id.twitch.tv/oauth2/token",
			UserInfoURL: "https://api.twitch.tv/helix/users",
			Scopes:      []string{"user:read:email"},
			UserInfoHeaders: map[string]string{
				"Client-Id": "{client_id}",
			},
		},
		"gitlab": {
			AuthURL:     "https://gitlab.com/oauth/authorize",
			TokenURL:    "https://gitlab.com/oauth/token",
			UserInfoURL: "https://gitlab.com/api/v4/user",
			Scopes:      []string{"read_user"},
		},
		"bitbucket": {
			AuthURL:     "https://bitbucket.org/site/oauth2/authorize",
			TokenURL:    "https://bitbucket.org/site/oauth2/access_token",
			UserInfoURL: "https://api.bitbucket.org/2.0/user",
			Scopes:      []string{"account", "email"},
		},
		"slack": {
			AuthURL:     "https://slack.com/openid/connect/authorize",
			TokenURL:    "https://slack.com/api/openid.connect.token",
			UserInfoURL: "https://slack.com/api/openid.connect.userInfo",
			Scopes:      []string{"openid", "profile", "email"},
		},
		"zoom": {
			AuthURL:         "https://zoom.us/oauth/authorize",
			TokenURL:        "https://zoom.us/oauth/token",
			UserInfoURL:     "https://api.zoom.us/v2/users/me",
			Scopes:          []string{"user:read:email"},
			TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
		},
		"figma": {
			AuthURL:     "https://www.figma.com/oauth",
			TokenURL:    "https://api.figma.com/v1/oauth/token",
			UserInfoURL: "https://api.figma.com/v1/me",
			Scopes:      []string{"file_read"},
		},
	}
	for name, cfg := range providers {
		oauthProviders[name] = cfg
		defaultProviders[name] = cfg
	}

	parsers := map[string]OAuthUserInfoParser{
		"discord":   parseDiscordUser,
		"twitter":   parseTwitterUser,
		"facebook":  parseFacebookUser,
		"linkedin":  parseLinkedInUser,
		"spotify":   parseSpotifyUser,
		"twitch":    parseTwitchUser,
		"gitlab":    parseGitLabUser,
		"bitbucket": parseBitbucketUser,
		"slack":     parseSlackUser,
		"zoom":      parseZoomUser,
		"figma":     parseFigmaUser,
	}
	for name, parser := range parsers {
		oauthUserInfoParsers[name] = parser
		defaultUserInfoParsers[name] = parser
	}

	// Notion uses token response extraction, not a userinfo parser.
	oauthProviders["notion"] = OAuthProviderConfig{
		AuthURL:                        "https://api.notion.com/v1/oauth/authorize",
		TokenURL:                       "https://api.notion.com/v1/oauth/token",
		Scopes:                         []string{},
		TokenAuthMethod:                OAuthTokenAuthMethodClientSecretBasic,
		UserInfoSource:                 OAuthUserInfoSourceTokenResponse,
		TokenResponseUserInfoExtractor: extractNotionUserFromTokenResponse,
	}
	defaultProviders["notion"] = oauthProviders["notion"]
}

// --- Discord ---

// parseDiscordUser unmarshals the Discord userinfo response and returns an OAuthUserInfo with the user ID, email, and name from the global_name field or username if global_name is empty. It returns an error if the user ID is missing.
func parseDiscordUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		Email      string `json:"email"`
		GlobalName string `json:"global_name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Discord user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.GlobalName
	if name == "" {
		name = u.Username
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- Twitter/X ---

// parseTwitterUser unmarshals the Twitter userinfo response (nested under the data field) and returns an OAuthUserInfo with the user ID, and name from the name field or username if name is empty. Email is not available from the Twitter API v2 users/me endpoint. It returns an error if the user ID is missing.
func parseTwitterUser(body []byte) (*OAuthUserInfo, error) {
	var resp struct {
		Data struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing Twitter user: %w", err)
	}
	if resp.Data.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := resp.Data.Name
	if name == "" {
		name = resp.Data.Username
	}
	return &OAuthUserInfo{
		ProviderUserID: resp.Data.ID,
		Name:           name,
		// Twitter API v2 doesn't return email in users/me.
	}, nil
}

// --- Facebook ---

// parseFacebookUser unmarshals the Facebook userinfo response and returns an OAuthUserInfo with the user ID, email, and name. It returns an error if the user ID is missing.
func parseFacebookUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Facebook user: %w", err)
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

// --- LinkedIn ---

// parseLinkedInUser unmarshals the LinkedIn userinfo response and returns an OAuthUserInfo with the user ID (sub), email, and name. It returns an error if the user ID is missing.
func parseLinkedInUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing LinkedIn user: %w", err)
	}
	if u.Sub == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.Sub,
		Email:          u.Email,
		Name:           u.Name,
	}, nil
}

// --- Spotify ---

// parseSpotifyUser unmarshals the Spotify userinfo response and returns an OAuthUserInfo with the user ID, email, and display name. It returns an error if the user ID is missing.
func parseSpotifyUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Spotify user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           u.DisplayName,
	}, nil
}

// --- Twitch ---

// parseTwitchUser unmarshals the Twitch userinfo response (a data array) and returns an OAuthUserInfo from the first user entry with the user ID, email, and name from the display_name field or login if display_name is empty. It returns an error if the data array is empty or the user ID is missing.
func parseTwitchUser(body []byte) (*OAuthUserInfo, error) {
	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			Login       string `json:"login"`
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing Twitch user: %w", err)
	}
	if len(resp.Data) == 0 || resp.Data[0].ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	u := resp.Data[0]
	name := u.DisplayName
	if name == "" {
		name = u.Login
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- GitLab ---

// parseGitLabUser unmarshals the GitLab userinfo response and returns an OAuthUserInfo with the user ID (converted to string), email, and name from the name field or username if name is empty. It returns an error if the user ID is zero.
func parseGitLabUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing GitLab user: %w", err)
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.Name
	if name == "" {
		name = u.Username
	}
	return &OAuthUserInfo{
		ProviderUserID: fmt.Sprintf("%d", u.ID),
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- Bitbucket ---

// parseBitbucketUser unmarshals the Bitbucket userinfo response and returns an OAuthUserInfo with the user ID (UUID), and name from the display_name field or nickname if display_name is empty. Email must be fetched separately via the Bitbucket emails endpoint. It returns an error if the UUID is missing.
func parseBitbucketUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		UUID        string `json:"uuid"`
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Bitbucket user: %w", err)
	}
	if u.UUID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.DisplayName
	if name == "" {
		name = u.Nickname
	}
	return &OAuthUserInfo{
		ProviderUserID: u.UUID,
		Name:           name,
		// Bitbucket requires a separate email endpoint — handled in fetchUserInfoWithConfig.
	}, nil
}

// --- Slack ---

// parseSlackUser unmarshals the Slack userinfo response and returns an OAuthUserInfo with the user ID (sub), email, and name. It returns an error if the user ID is missing.
func parseSlackUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Slack user: %w", err)
	}
	if u.Sub == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.Sub,
		Email:          u.Email,
		Name:           u.Name,
	}, nil
}

// --- Zoom ---

// parseZoomUser unmarshals the Zoom userinfo response and returns an OAuthUserInfo with the user ID, email, and name from the display_name field or a concatenation of first_name and last_name if display_name is empty. It returns an error if the user ID is missing.
func parseZoomUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Zoom user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	name := u.DisplayName
	if name == "" {
		name = strings.TrimSpace(u.FirstName + " " + u.LastName)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           name,
	}, nil
}

// --- Notion ---
// Notion returns user info in the token response body (owner.user),
// not via a separate userinfo endpoint.

// extractNotionUserFromTokenResponse unmarshals the Notion OAuth token response body and extracts user info from the owner.user field (Notion returns user info in the token response rather than via a separate userinfo endpoint). It returns an OAuthUserInfo with the user ID, email, and name. It returns an error if the owner type is not user or the user ID is missing.
func extractNotionUserFromTokenResponse(body []byte) (*OAuthUserInfo, error) {
	var resp struct {
		Owner struct {
			Type string `json:"type"`
			User struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Person struct {
					Email string `json:"email"`
				} `json:"person"`
			} `json:"user"`
		} `json:"owner"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing Notion token response: %w", err)
	}
	if resp.Owner.Type != "user" {
		return nil, fmt.Errorf("notion owner is not a user (type=%q)", resp.Owner.Type)
	}
	if resp.Owner.User.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: resp.Owner.User.ID,
		Email:          resp.Owner.User.Person.Email,
		Name:           resp.Owner.User.Name,
	}, nil
}

// --- Figma ---

// parseFigmaUser unmarshals the Figma userinfo response and returns an OAuthUserInfo with the user ID, email, and name from the handle field. It returns an error if the user ID is missing.
func parseFigmaUser(body []byte) (*OAuthUserInfo, error) {
	var u struct {
		ID     string `json:"id"`
		Handle string `json:"handle"`
		Email  string `json:"email"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("parsing Figma user: %w", err)
	}
	if u.ID == "" {
		return nil, fmt.Errorf("%w: missing user ID", ErrOAuthProviderError)
	}
	return &OAuthUserInfo{
		ProviderUserID: u.ID,
		Email:          u.Email,
		Name:           u.Handle,
	}, nil
}

// --- Provider config overrides ---

// SetFacebookAPIVersion updates the Facebook provider URLs to use the given
// Graph API version (e.g., "v22.0"). Called at startup if the operator sets
// auth.oauth.facebook.facebook_api_version in ayb.toml.
func SetFacebookAPIVersion(version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()
	cfg, ok := oauthProviders["facebook"]
	if !ok {
		return
	}
	cfg.AuthURL = "https://www.facebook.com/" + version + "/dialog/oauth"
	cfg.TokenURL = "https://graph.facebook.com/" + version + "/oauth/access_token"
	cfg.UserInfoURL = "https://graph.facebook.com/" + version + "/me?fields=id,name,email,picture"
	oauthProviders["facebook"] = cfg
}

// SetGitLabBaseURL updates the GitLab provider URLs to use the given base URL
// (e.g., "https://gitlab.example.com" for a self-hosted instance). Called at
// startup if the operator sets auth.oauth.gitlab.gitlab_base_url in ayb.toml.
func SetGitLabBaseURL(baseURL string) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()
	cfg, ok := oauthProviders["gitlab"]
	if !ok {
		return
	}
	cfg.AuthURL = baseURL + "/oauth/authorize"
	cfg.TokenURL = baseURL + "/oauth/token"
	cfg.UserInfoURL = baseURL + "/api/v4/user"
	oauthProviders["gitlab"] = cfg
}

// fetchBitbucketPrimaryEmail fetches the primary email from the Bitbucket
// emails endpoint (/2.0/user/emails). Similar to fetchGitHubPrimaryEmail.
func fetchBitbucketPrimaryEmail(ctx context.Context, accessToken string, httpClient *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/user/emails", nil)
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
		return "", fmt.Errorf("bitbucket emails endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	var emailResp struct {
		Values []struct {
			Email       string `json:"email"`
			IsPrimary   bool   `json:"is_primary"`
			IsConfirmed bool   `json:"is_confirmed"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &emailResp); err != nil {
		return "", err
	}

	for _, e := range emailResp.Values {
		if e.IsPrimary && e.IsConfirmed {
			return e.Email, nil
		}
	}
	// Fallback: first confirmed email.
	for _, e := range emailResp.Values {
		if e.IsConfirmed {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no confirmed email found")
}
