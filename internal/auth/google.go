package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google implements the authorization-code flow directly against Google's
// endpoints. This is two HTTP calls, so we do it with the standard library
// rather than taking a dependency on x/oauth2 -- see agents.md > Stack.
type Google struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	HTTP         *http.Client

	// Endpoints default to Google's; tests point them at a fake.
	AuthEndpoint     string
	TokenEndpoint    string
	UserinfoEndpoint string
}

const (
	googleAuth     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleToken    = "https://oauth2.googleapis.com/token"
	googleUserinfo = "https://openidconnect.googleapis.com/v1/userinfo"
)

func (g Google) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func or(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// AuthURL is where we send the browser to begin sign-in.
func (g Google) AuthURL(state string) string {
	q := url.Values{
		"client_id":     {g.ClientID},
		"redirect_uri":  {g.RedirectURL},
		"response_type": {"code"},
		// profile for the display name. No offline scope, so Google never
		// issues a refresh token we would have to store.
		"scope":  {"openid email profile"},
		"state":  {state},
		"prompt": {"select_account"},
	}
	return or(g.AuthEndpoint, googleAuth) + "?" + q.Encode()
}

// UserInfo is the subset of the OIDC userinfo response we care about.
type UserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// Exchange trades the authorization code for an access token and resolves the
// signed-in identity.
//
// We call the userinfo endpoint rather than verifying the id_token locally.
// Both are sound; this avoids carrying a JWKS cache and a JWT library, and the
// token came to us directly from Google over TLS, so there is no untrusted
// intermediary to guard against.
func (g Google) Exchange(ctx context.Context, code string) (UserInfo, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"redirect_uri":  {g.RedirectURL},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, or(g.TokenEndpoint, googleToken), strings.NewReader(form.Encode()))
	if err != nil {
		return UserInfo{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.client().Do(req)
	if err != nil {
		return UserInfo{}, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		// Google echoes the client_id in errors; log the status only.
		return UserInfo{}, fmt.Errorf("token exchange: google returned %s", resp.Status)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return UserInfo{}, fmt.Errorf("token exchange: no access token in response")
	}
	return g.userInfo(ctx, tok.AccessToken)
}

func (g Google) userInfo(ctx context.Context, accessToken string) (UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, or(g.UserinfoEndpoint, googleUserinfo), nil)
	if err != nil {
		return UserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := g.client().Do(req)
	if err != nil {
		return UserInfo{}, fmt.Errorf("userinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, fmt.Errorf("userinfo: google returned %s", resp.Status)
	}
	var ui UserInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&ui); err != nil {
		return UserInfo{}, fmt.Errorf("userinfo: %w", err)
	}
	if ui.Sub == "" {
		return UserInfo{}, fmt.Errorf("userinfo: response had no subject")
	}
	return ui, nil
}
