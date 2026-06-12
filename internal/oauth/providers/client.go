package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domain "github.com/vpt/blog-backend/internal/oauth"
	gooauth2 "golang.org/x/oauth2"
)

const providerUserAgent = "blog-backend-oauth"

func newProviderHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func tokenSetFromOAuth2(token *gooauth2.Token) *domain.TokenSet {
	var refreshToken *string
	if token.RefreshToken != "" {
		refreshToken = &token.RefreshToken
	}
	var expiry *time.Time
	if !token.Expiry.IsZero() {
		expiry = &token.Expiry
	}
	var idToken *string
	if rawIDToken, ok := token.Extra("id_token").(string); ok && rawIDToken != "" {
		idToken = &rawIDToken
	}
	return &domain.TokenSet{
		AccessToken:  token.AccessToken,
		RefreshToken: refreshToken,
		IDToken:      idToken,
		Expiry:       expiry,
		Extra:        oauth2Extra(token),
	}
}

func oauth2Extra(token *gooauth2.Token) map[string]string {
	extra := make(map[string]string)
	for _, key := range []string{"uid", "openid", "unionid"} {
		if value := token.Extra(key); value != nil {
			extra[key] = fmt.Sprint(value)
		}
	}
	return extra
}

func getJSONWithBearer(ctx context.Context, client httpClient, platform string, endpoint string, accessToken string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", providerUserAgent)
	return doJSON(client, platform, req, out)
}

func getJSON(ctx context.Context, client httpClient, platform string, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", providerUserAgent)
	return doJSON(client, platform, req, out)
}

func doJSON(client httpClient, platform string, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s userinfo 请求失败: status=%d body=%s", platform, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, out)
}

func getText(ctx context.Context, client httpClient, platform string, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", providerUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%s userinfo 请求失败: status=%d body=%s", platform, resp.StatusCode, string(body))
	}
	return string(body), nil
}

func withQuery(endpoint string, values map[string]string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	for key, value := range values {
		if strings.TrimSpace(value) != "" {
			query.Set(key, value)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func strPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
