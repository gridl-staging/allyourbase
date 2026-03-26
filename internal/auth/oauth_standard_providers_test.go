package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Discord ---

func TestParseDiscordUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"123456789","username":"testuser","email":"test@discord.com","global_name":"Test User"}`)
	info, err := parseDiscordUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "123456789", info.ProviderUserID)
	testutil.Equal(t, "test@discord.com", info.Email)
	testutil.Equal(t, "Test User", info.Name)
}

func TestParseDiscordUserFallbackToUsername(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"123456789","username":"testuser","email":"test@discord.com"}`)
	info, err := parseDiscordUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "testuser", info.Name)
}

func TestParseDiscordUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"username":"testuser","email":"test@discord.com"}`)
	_, err := parseDiscordUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestDiscordProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("discord")
	testutil.True(t, ok, "discord provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "discord.com")
	testutil.Contains(t, cfg.TokenURL, "discord.com")
	testutil.Contains(t, cfg.UserInfoURL, "discord.com")
}

func TestDiscordOAuthIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth2/token" {
			// Verify standard POST body auth.
			body, _ := url.ParseQuery(readBody(r))
			if body.Get("code") != "discord-auth-code" {
				http.Error(w, "bad code", http.StatusBadRequest)
				return
			}
			if body.Get("client_id") == "" || body.Get("client_secret") == "" {
				http.Error(w, "missing credentials", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "discord-access-token",
				"token_type":   "Bearer",
			})
			return
		}
		if r.URL.Path == "/api/users/@me" {
			if r.Header.Get("Authorization") != "Bearer discord-access-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          "987654321",
				"username":    "discorduser",
				"email":       "discord@example.com",
				"global_name": "Discord User",
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "discord-cid", ClientSecret: "discord-secret"}
	pc := OAuthProviderConfig{
		AuthURL:     tokenServer.URL + "/api/oauth2/authorize",
		TokenURL:    tokenServer.URL + "/api/oauth2/token",
		UserInfoURL: tokenServer.URL + "/api/users/@me",
		Scopes:      []string{"identify", "email"},
	}

	info, err := exchangeCode(context.Background(), "discord", client, "discord-auth-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "987654321", info.ProviderUserID)
	testutil.Equal(t, "discord@example.com", info.Email)
	testutil.Equal(t, "Discord User", info.Name)
}

// --- Twitter/X ---

func TestParseTwitterUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"data":{"id":"111222333","username":"twitteruser","name":"Twitter User","profile_image_url":"https://pbs.twimg.com/profile_images/123/photo.jpg"}}`)
	info, err := parseTwitterUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "111222333", info.ProviderUserID)
	testutil.Equal(t, "Twitter User", info.Name)
	// Twitter API v2 doesn't return email in users/me, even with scopes
	testutil.Equal(t, "", info.Email)
}

func TestParseTwitterUserFallbackToUsername(t *testing.T) {
	t.Parallel()
	body := []byte(`{"data":{"id":"111222333","username":"twitteruser"}}`)
	info, err := parseTwitterUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "twitteruser", info.Name)
}

func TestParseTwitterUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"data":{"username":"twitteruser"}}`)
	_, err := parseTwitterUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestParseTwitterUserMissingDataWrapper(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"111222333","username":"twitteruser"}`)
	_, err := parseTwitterUser(body)
	testutil.True(t, err != nil, "expected error for missing data wrapper")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestTwitterProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("twitter")
	testutil.True(t, ok, "twitter provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "twitter.com")
	testutil.Contains(t, cfg.TokenURL, "twitter.com")
	testutil.Equal(t, OAuthTokenAuthMethodClientSecretBasic, cfg.TokenAuthMethod)
}

func TestTwitterOAuthIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/2/oauth2/token" {
			// Twitter uses Basic Auth for token exchange.
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Basic ") {
				http.Error(w, "expected Basic auth", http.StatusUnauthorized)
				return
			}
			// Verify client_id is NOT in body (Basic Auth mode).
			body, _ := url.ParseQuery(readBody(r))
			if body.Get("client_id") != "" {
				http.Error(w, "client_id should not be in body for Basic Auth", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "twitter-access-token",
				"token_type":   "Bearer",
			})
			return
		}
		if r.URL.Path == "/2/users/me" {
			if r.Header.Get("Authorization") != "Bearer twitter-access-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"id":                "444555666",
					"username":          "xuser",
					"name":              "X User",
					"profile_image_url": "https://pbs.twimg.com/photo.jpg",
				},
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "twitter-cid", ClientSecret: "twitter-secret"}
	pc := OAuthProviderConfig{
		AuthURL:         tokenServer.URL + "/i/oauth2/authorize",
		TokenURL:        tokenServer.URL + "/2/oauth2/token",
		UserInfoURL:     tokenServer.URL + "/2/users/me?user.fields=profile_image_url",
		Scopes:          []string{"users.read", "tweet.read", "offline.access"},
		TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
	}

	info, err := exchangeCode(context.Background(), "twitter", client, "twitter-auth-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "444555666", info.ProviderUserID)
	testutil.Equal(t, "X User", info.Name)
}

// --- Facebook ---

func TestParseFacebookUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"10001","name":"Facebook User","email":"fb@example.com","picture":{"data":{"url":"https://graph.facebook.com/pic.jpg"}}}`)
	info, err := parseFacebookUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "10001", info.ProviderUserID)
	testutil.Equal(t, "fb@example.com", info.Email)
	testutil.Equal(t, "Facebook User", info.Name)
}

func TestParseFacebookUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"name":"Facebook User","email":"fb@example.com"}`)
	_, err := parseFacebookUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestParseFacebookUserNoEmail(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"10001","name":"Facebook User"}`)
	info, err := parseFacebookUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "", info.Email)
}

func TestFacebookProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("facebook")
	testutil.True(t, ok, "facebook provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "facebook.com")
	testutil.Contains(t, cfg.TokenURL, "graph.facebook.com")
}

func TestFacebookOAuthIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v22.0/oauth/access_token" {
			body, _ := url.ParseQuery(readBody(r))
			if body.Get("code") != "fb-auth-code" {
				http.Error(w, "bad code", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "fb-access-token",
				"token_type":   "Bearer",
			})
			return
		}
		if r.URL.Path == "/v22.0/me" {
			if r.Header.Get("Authorization") != "Bearer fb-access-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "20002",
				"name":  "FB Person",
				"email": "fbperson@example.com",
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "fb-cid", ClientSecret: "fb-secret"}
	pc := OAuthProviderConfig{
		AuthURL:     tokenServer.URL + "/v22.0/dialog/oauth",
		TokenURL:    tokenServer.URL + "/v22.0/oauth/access_token",
		UserInfoURL: tokenServer.URL + "/v22.0/me?fields=id,name,email,picture",
		Scopes:      []string{"email", "public_profile"},
	}

	info, err := exchangeCode(context.Background(), "facebook", client, "fb-auth-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "20002", info.ProviderUserID)
	testutil.Equal(t, "fbperson@example.com", info.Email)
	testutil.Equal(t, "FB Person", info.Name)
}

// --- LinkedIn ---

func TestParseLinkedInUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"sub":"abc123","email":"li@example.com","name":"LinkedIn User","email_verified":true}`)
	info, err := parseLinkedInUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "abc123", info.ProviderUserID)
	testutil.Equal(t, "li@example.com", info.Email)
	testutil.Equal(t, "LinkedIn User", info.Name)
}

func TestParseLinkedInUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"email":"li@example.com","name":"LinkedIn User"}`)
	_, err := parseLinkedInUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestParseLinkedInUserNoEmail(t *testing.T) {
	t.Parallel()
	body := []byte(`{"sub":"abc123","name":"LinkedIn User"}`)
	info, err := parseLinkedInUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "", info.Email)
}

func TestLinkedInProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("linkedin")
	testutil.True(t, ok, "linkedin provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "linkedin.com")
	testutil.Contains(t, cfg.TokenURL, "linkedin.com")
}

func TestLinkedInOAuthIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/v2/accessToken" {
			body, _ := url.ParseQuery(readBody(r))
			if body.Get("code") != "li-auth-code" {
				http.Error(w, "bad code", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "li-access-token",
				"token_type":   "Bearer",
			})
			return
		}
		if r.URL.Path == "/v2/userinfo" {
			if r.Header.Get("Authorization") != "Bearer li-access-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"sub":            "def456",
				"name":           "LI Person",
				"email":          "liperson@example.com",
				"email_verified": true,
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "li-cid", ClientSecret: "li-secret"}
	pc := OAuthProviderConfig{
		AuthURL:     tokenServer.URL + "/oauth/v2/authorization",
		TokenURL:    tokenServer.URL + "/oauth/v2/accessToken",
		UserInfoURL: tokenServer.URL + "/v2/userinfo",
		Scopes:      []string{"openid", "profile", "email"},
	}

	info, err := exchangeCode(context.Background(), "linkedin", client, "li-auth-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "def456", info.ProviderUserID)
	testutil.Equal(t, "liperson@example.com", info.Email)
	testutil.Equal(t, "LI Person", info.Name)
}

// readBody is a helper that reads the full request body.
func readBody(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

// --- Spotify ---

func TestParseSpotifyUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"spotifyuser1","display_name":"Spotify User","email":"spot@example.com"}`)
	info, err := parseSpotifyUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "spotifyuser1", info.ProviderUserID)
	testutil.Equal(t, "spot@example.com", info.Email)
	testutil.Equal(t, "Spotify User", info.Name)
}

func TestParseSpotifyUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"display_name":"Spotify User","email":"spot@example.com"}`)
	_, err := parseSpotifyUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestSpotifyProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("spotify")
	testutil.True(t, ok, "spotify provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "accounts.spotify.com")
}

// --- Twitch ---

func TestParseTwitchUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"data":[{"id":"twitch123","login":"twitchuser","display_name":"Twitch User","email":"twitch@example.com"}]}`)
	info, err := parseTwitchUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "twitch123", info.ProviderUserID)
	testutil.Equal(t, "twitch@example.com", info.Email)
	testutil.Equal(t, "Twitch User", info.Name)
}

func TestParseTwitchUserFallbackToLogin(t *testing.T) {
	t.Parallel()
	body := []byte(`{"data":[{"id":"twitch123","login":"twitchuser","email":"twitch@example.com"}]}`)
	info, err := parseTwitchUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "twitchuser", info.Name)
}

func TestParseTwitchUserEmptyData(t *testing.T) {
	t.Parallel()
	body := []byte(`{"data":[]}`)
	_, err := parseTwitchUser(body)
	testutil.True(t, err != nil, "expected error for empty data")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestTwitchProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("twitch")
	testutil.True(t, ok, "twitch provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "id.twitch.tv")
	// Twitch requires Client-Id header on userinfo requests.
	testutil.Equal(t, "{client_id}", cfg.UserInfoHeaders["Client-Id"])
}

func TestTwitchOAuthIntegration_ClientIdHeader(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "twitch-token",
				"token_type":   "Bearer",
			})
			return
		}
		if r.URL.Path == "/helix/users" {
			// Verify Twitch-specific Client-Id header.
			if r.Header.Get("Client-Id") != "twitch-cid" {
				http.Error(w, fmt.Sprintf("expected Client-Id: twitch-cid, got: %s", r.Header.Get("Client-Id")), http.StatusBadRequest)
				return
			}
			if r.Header.Get("Authorization") != "Bearer twitch-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": "twitch999", "login": "twitch_login", "display_name": "Twitch Person", "email": "twitch@test.com"},
				},
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "twitch-cid", ClientSecret: "twitch-secret"}
	pc := OAuthProviderConfig{
		TokenURL:    tokenServer.URL + "/oauth2/token",
		UserInfoURL: tokenServer.URL + "/helix/users",
		Scopes:      []string{"user:read:email"},
		UserInfoHeaders: map[string]string{
			"Client-Id": "{client_id}",
		},
	}

	info, err := exchangeCode(context.Background(), "twitch", client, "twitch-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "twitch999", info.ProviderUserID)
	testutil.Equal(t, "twitch@test.com", info.Email)
	testutil.Equal(t, "Twitch Person", info.Name)
}

// --- GitLab ---

func TestParseGitLabUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":42,"username":"gitlabuser","name":"GitLab User","email":"gl@example.com"}`)
	info, err := parseGitLabUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "42", info.ProviderUserID)
	testutil.Equal(t, "gl@example.com", info.Email)
	testutil.Equal(t, "GitLab User", info.Name)
}

func TestParseGitLabUserFallbackToUsername(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":42,"username":"gitlabuser","email":"gl@example.com"}`)
	info, err := parseGitLabUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "gitlabuser", info.Name)
}

func TestParseGitLabUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"username":"gitlabuser","email":"gl@example.com"}`)
	_, err := parseGitLabUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestGitLabProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("gitlab")
	testutil.True(t, ok, "gitlab provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "gitlab.com")
}

// --- Bitbucket ---

func TestParseBitbucketUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"uuid":"{abc-123}","display_name":"Bitbucket User","nickname":"bbuser"}`)
	info, err := parseBitbucketUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "{abc-123}", info.ProviderUserID)
	testutil.Equal(t, "Bitbucket User", info.Name)
	// Bitbucket requires separate email endpoint — email is empty from main endpoint.
	testutil.Equal(t, "", info.Email)
}

func TestParseBitbucketUserFallbackToNickname(t *testing.T) {
	t.Parallel()
	body := []byte(`{"uuid":"{abc-123}","nickname":"bbuser"}`)
	info, err := parseBitbucketUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "bbuser", info.Name)
}

func TestParseBitbucketUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"display_name":"Bitbucket User"}`)
	_, err := parseBitbucketUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestBitbucketProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("bitbucket")
	testutil.True(t, ok, "bitbucket provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "bitbucket.org")
}

// --- Slack ---

func TestParseSlackUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"sub":"U12345","email":"slack@example.com","name":"Slack User","email_verified":true}`)
	info, err := parseSlackUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "U12345", info.ProviderUserID)
	testutil.Equal(t, "slack@example.com", info.Email)
	testutil.Equal(t, "Slack User", info.Name)
}

func TestParseSlackUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"email":"slack@example.com","name":"Slack User"}`)
	_, err := parseSlackUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestSlackProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("slack")
	testutil.True(t, ok, "slack provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "slack.com")
}

// --- Zoom ---

func TestParseZoomUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"zoom123","email":"zoom@example.com","first_name":"Zoom","last_name":"User","display_name":"Zoom User"}`)
	info, err := parseZoomUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "zoom123", info.ProviderUserID)
	testutil.Equal(t, "zoom@example.com", info.Email)
	testutil.Equal(t, "Zoom User", info.Name)
}

func TestParseZoomUserFallbackToFirstLast(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"zoom123","email":"zoom@example.com","first_name":"Zoom","last_name":"User"}`)
	info, err := parseZoomUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "Zoom User", info.Name)
}

func TestParseZoomUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"email":"zoom@example.com","first_name":"Zoom"}`)
	_, err := parseZoomUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestZoomProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("zoom")
	testutil.True(t, ok, "zoom provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "zoom.us")
	testutil.Equal(t, OAuthTokenAuthMethodClientSecretBasic, cfg.TokenAuthMethod)
}

// --- Notion ---

func TestParseNotionTokenResponseUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"access_token": "notion-token",
		"token_type": "bearer",
		"owner": {
			"type": "user",
			"user": {
				"id": "notion-user-123",
				"name": "Notion User",
				"person": {"email": "notion@example.com"},
				"type": "person"
			}
		}
	}`)
	info, err := extractNotionUserFromTokenResponse(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "notion-user-123", info.ProviderUserID)
	testutil.Equal(t, "notion@example.com", info.Email)
	testutil.Equal(t, "Notion User", info.Name)
}

func TestParseNotionTokenResponseBotOwner(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"access_token": "notion-token",
		"owner": {"type": "workspace"}
	}`)
	_, err := extractNotionUserFromTokenResponse(body)
	testutil.True(t, err != nil, "expected error for non-user owner")
	testutil.Contains(t, err.Error(), "not a user")
}

func TestParseNotionTokenResponseMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"access_token": "notion-token",
		"owner": {
			"type": "user",
			"user": {"name": "No ID User"}
		}
	}`)
	_, err := extractNotionUserFromTokenResponse(body)
	testutil.True(t, err != nil, "expected error for missing user ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestNotionProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("notion")
	testutil.True(t, ok, "notion provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "notion.com")
	testutil.Equal(t, OAuthTokenAuthMethodClientSecretBasic, cfg.TokenAuthMethod)
	testutil.Equal(t, OAuthUserInfoSourceTokenResponse, cfg.UserInfoSource)
}

// --- Figma ---

func TestParseFigmaUser(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"figma123","handle":"figmauser","email":"figma@example.com"}`)
	info, err := parseFigmaUser(body)
	testutil.NoError(t, err)
	testutil.Equal(t, "figma123", info.ProviderUserID)
	testutil.Equal(t, "figma@example.com", info.Email)
	testutil.Equal(t, "figmauser", info.Name)
}

func TestParseFigmaUserMissingID(t *testing.T) {
	t.Parallel()
	body := []byte(`{"handle":"figmauser","email":"figma@example.com"}`)
	_, err := parseFigmaUser(body)
	testutil.True(t, err != nil, "expected error for missing ID")
	testutil.Contains(t, err.Error(), "missing user ID")
}

func TestFigmaProviderRegistered(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("figma")
	testutil.True(t, ok, "figma provider should be registered")
	testutil.Contains(t, cfg.AuthURL, "figma.com")
}

// --- SetFacebookAPIVersion ---

func TestSetFacebookAPIVersion(t *testing.T) {
	t.Cleanup(func() { ResetProviderURLs("facebook") })

	SetFacebookAPIVersion("v23.0")
	cfg, ok := getProviderConfig("facebook")
	testutil.True(t, ok, "facebook provider should exist")
	testutil.Contains(t, cfg.AuthURL, "v23.0")
	testutil.Contains(t, cfg.TokenURL, "v23.0")
	testutil.Contains(t, cfg.UserInfoURL, "v23.0")
	// Ensure old version is gone.
	testutil.True(t, !strings.Contains(cfg.AuthURL, "v22.0"), "old version should be replaced")
}

func TestSetFacebookAPIVersionEmpty(t *testing.T) {
	t.Cleanup(func() { ResetProviderURLs("facebook") })

	// Capture original.
	orig, _ := getProviderConfig("facebook")
	SetFacebookAPIVersion("")
	after, _ := getProviderConfig("facebook")
	testutil.Equal(t, orig.AuthURL, after.AuthURL)
	testutil.Equal(t, orig.TokenURL, after.TokenURL)
}

// --- SetGitLabBaseURL ---

func TestSetGitLabBaseURL(t *testing.T) {
	t.Cleanup(func() { ResetProviderURLs("gitlab") })

	SetGitLabBaseURL("https://gitlab.example.com")
	cfg, ok := getProviderConfig("gitlab")
	testutil.True(t, ok, "gitlab provider should exist")
	testutil.Contains(t, cfg.AuthURL, "gitlab.example.com")
	testutil.Contains(t, cfg.TokenURL, "gitlab.example.com")
	testutil.Contains(t, cfg.UserInfoURL, "gitlab.example.com")
	// Ensure default gitlab.com is gone.
	testutil.True(t, !strings.Contains(cfg.AuthURL, "gitlab.com/"), "default URL should be replaced")
}

func TestSetGitLabBaseURLEmpty(t *testing.T) {
	t.Cleanup(func() { ResetProviderURLs("gitlab") })

	orig, _ := getProviderConfig("gitlab")
	SetGitLabBaseURL("")
	after, _ := getProviderConfig("gitlab")
	testutil.Equal(t, orig.AuthURL, after.AuthURL)
}

func TestSetGitLabBaseURLTrailingSlash(t *testing.T) {
	t.Cleanup(func() { ResetProviderURLs("gitlab") })

	SetGitLabBaseURL("https://gitlab.example.com/")
	cfg, _ := getProviderConfig("gitlab")
	// Should not have double slashes.
	testutil.Contains(t, cfg.AuthURL, "gitlab.example.com/oauth/authorize")
	testutil.True(t, !strings.Contains(cfg.AuthURL, "//oauth"), "should not have double slashes")
}

// --- Notion extractor wiring ---

func TestNotionProviderExtractorWired(t *testing.T) {
	t.Parallel()
	cfg, ok := getProviderConfig("notion")
	testutil.True(t, ok, "notion provider should be registered")
	testutil.True(t, cfg.TokenResponseUserInfoExtractor != nil, "Notion must have TokenResponseUserInfoExtractor wired")
}

// --- Integration tests for remaining providers ---

func TestSpotifyOAuthIntegration_FullFlow(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/token":
			body, _ := url.ParseQuery(readBody(r))
			if body.Get("code") != "spotify-code" {
				http.Error(w, "bad code", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "spotify-tok",
				"token_type":   "Bearer",
			})
		case "/v1/me":
			if r.Header.Get("Authorization") != "Bearer spotify-tok" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "spotify123",
				"display_name": "Spotify User",
				"email":        "spotify@example.com",
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "sp-cid", ClientSecret: "sp-secret"}
	pc := OAuthProviderConfig{
		TokenURL:    tokenServer.URL + "/api/token",
		UserInfoURL: tokenServer.URL + "/v1/me",
		Scopes:      []string{"user-read-email", "user-read-private"},
	}

	info, err := exchangeCode(context.Background(), "spotify", client, "spotify-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.Equal(t, "spotify123", info.ProviderUserID)
	testutil.Equal(t, "spotify@example.com", info.Email)
	testutil.Equal(t, "Spotify User", info.Name)
}

func TestZoomOAuthIntegration_BasicAuth(t *testing.T) {
	t.Parallel()

	var usedBasicAuth bool
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			// Zoom requires Basic Auth for token exchange.
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Basic ") {
				http.Error(w, "Zoom requires Basic Auth", http.StatusUnauthorized)
				return
			}
			usedBasicAuth = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "zoom-tok",
				"token_type":   "Bearer",
			})
		case "/v2/users/me":
			if r.Header.Get("Authorization") != "Bearer zoom-tok" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         "zoom-uid-1",
				"email":      "zoom@example.com",
				"first_name": "Zoom",
				"last_name":  "User",
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "zm-cid", ClientSecret: "zm-secret"}
	pc := OAuthProviderConfig{
		TokenURL:        tokenServer.URL + "/oauth/token",
		UserInfoURL:     tokenServer.URL + "/v2/users/me",
		Scopes:          []string{"user:read:email"},
		TokenAuthMethod: OAuthTokenAuthMethodClientSecretBasic,
	}

	info, err := exchangeCode(context.Background(), "zoom", client, "zoom-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.True(t, usedBasicAuth, "Zoom token exchange must use HTTP Basic Auth")
	testutil.Equal(t, "zoom-uid-1", info.ProviderUserID)
	testutil.Equal(t, "zoom@example.com", info.Email)
	testutil.Equal(t, "Zoom User", info.Name)
}

func TestNotionOAuthIntegration_TokenResponseExtraction(t *testing.T) {
	t.Parallel()

	var usedBasicAuth bool
	var sawUserInfoEndpoint bool
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth/token":
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Basic ") {
				http.Error(w, "Notion requires Basic Auth", http.StatusUnauthorized)
				return
			}
			usedBasicAuth = true
			w.Header().Set("Content-Type", "application/json")
			// Notion returns user info in token response — no separate userinfo endpoint.
			fmt.Fprint(w, `{
				"access_token": "notion-tok",
				"token_type":   "bearer",
				"owner": {
					"type": "user",
					"user": {
						"id": "notion-uid-1",
						"name": "Notion User",
						"person": {"email": "notion@example.com"}
					}
				}
			}`)
		default:
			sawUserInfoEndpoint = true
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer tokenServer.Close()

	client := OAuthClientConfig{ClientID: "not-cid", ClientSecret: "not-secret"}
	pc := OAuthProviderConfig{
		TokenURL:                       tokenServer.URL + "/v1/oauth/token",
		TokenAuthMethod:                OAuthTokenAuthMethodClientSecretBasic,
		UserInfoSource:                 OAuthUserInfoSourceTokenResponse,
		TokenResponseUserInfoExtractor: extractNotionUserFromTokenResponse,
	}

	info, err := exchangeCode(context.Background(), "notion", client, "notion-code", "http://localhost/callback", pc, tokenServer.Client())
	testutil.NoError(t, err)
	testutil.True(t, usedBasicAuth, "Notion token exchange must use HTTP Basic Auth")
	testutil.True(t, !sawUserInfoEndpoint, "Notion should not hit a userinfo endpoint — info comes from token response")
	testutil.Equal(t, "notion-uid-1", info.ProviderUserID)
	testutil.Equal(t, "notion@example.com", info.Email)
	testutil.Equal(t, "Notion User", info.Name)
}
