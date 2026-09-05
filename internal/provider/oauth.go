package provider

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

type OAuthClient struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	UserType     string
	HTTP         *http.Client
}

type TokenSet struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	UserType     string
	LocationID   string
	CompanyID    string
	Scope        string
	ExpiresIn    int
}

func (c *OAuthClient) Exchange(ctx context.Context, code string) (TokenSet, error) {
	values := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.RedirectURI},
	}
	if c.UserType != "" {
		values.Set("user_type", c.UserType)
	}
	return c.tokenRequest(ctx, values, "")
}

func (c *OAuthClient) Refresh(ctx context.Context, refreshToken, userType string) (TokenSet, error) {
	if refreshToken == "" {
		return TokenSet{}, fmt.Errorf("HighLevel refresh token is required")
	}
	if userType == "" {
		userType = c.UserType
	}
	values := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	if userType != "" {
		values.Set("user_type", userType)
	}
	if c.RedirectURI != "" {
		values.Set("redirect_uri", c.RedirectURI)
	}
	return c.tokenRequest(ctx, values, "")
}

func (c *OAuthClient) LocationToken(ctx context.Context, accessToken, companyID, locationID string) (TokenSet, error) {
	body, err := json.Marshal(map[string]string{"companyId": companyID, "locationId": locationID})
	if err != nil {
		return TokenSet{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL(), "/")+"/oauth/locationToken", strings.NewReader(string(body)))
	if err != nil {
		return TokenSet{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Version", "2021-07-28")
	return c.doToken(req)
}

func (c *OAuthClient) tokenRequest(ctx context.Context, values url.Values, bearer string) (TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL(), "/")+"/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return TokenSet{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Version", "2021-07-28")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return c.doToken(req)
}

func (c *OAuthClient) doToken(req *http.Request) (TokenSet, error) {
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return TokenSet{}, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenSet{}, &Error{Status: resp.StatusCode, Code: fmt.Sprint(resp.StatusCode), Message: strings.TrimSpace(string(payload))}
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		UserType     string `json:"userType"`
		LocationID   string `json:"locationId"`
		CompanyID    string `json:"companyId"`
		Scope        string `json:"scope"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return TokenSet{}, fmt.Errorf("decode HighLevel oauth response: %w", err)
	}
	if parsed.AccessToken == "" {
		return TokenSet{}, fmt.Errorf("HighLevel oauth response is missing access_token")
	}
	return TokenSet{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		TokenType:    parsed.TokenType,
		UserType:     parsed.UserType,
		LocationID:   parsed.LocationID,
		CompanyID:    parsed.CompanyID,
		Scope:        parsed.Scope,
		ExpiresIn:    parsed.ExpiresIn,
	}, nil
}

func (c *OAuthClient) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://services.leadconnectorhq.com"
}

func (t TokenSet) ExpiresAt(now time.Time) time.Time {
	if t.ExpiresIn <= 0 {
		return now.Add(24 * time.Hour)
	}
	return now.Add(time.Duration(t.ExpiresIn) * time.Second)
}
