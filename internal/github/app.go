package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// appCredentials caches installation credentials in memory only. The gate
// serializes renewal while letting canceled callers stop waiting immediately.
type appCredentials struct {
	appID          string
	installationID int64
	key            *rsa.PrivateKey
	permissions    map[string]string
	endpoint       string
	now            func() time.Time
	gate           chan struct{}
	mu             sync.Mutex
	cached         string
	expires        time.Time
}

// NewAppClient authenticates using automatically renewed GitHub App installation
// tokens. It accepts an unencrypted PKCS#1 or PKCS#8 RSA PEM private key.
func NewAppClient(appID string, installationID int64, privateKey []byte) (*Client, error) {
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != appID || installationID <= 0 {
		return nil, errors.New("invalid GitHub App or installation ID")
	}
	block, rest := pem.Decode(privateKey)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("invalid GitHub App private key PEM")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			var ok bool
			key, ok = parsed.(*rsa.PrivateKey)
			if !ok {
				err = errors.New("not RSA")
			}
		}
	default:
		err = errors.New("unsupported PEM type")
	}
	if err != nil || key == nil || key.N.BitLen() < 2048 {
		return nil, errors.New("GitHub App private key must be an RSA key of at least 2048 bits")
	}
	if err := key.Validate(); err != nil {
		return nil, errors.New("invalid GitHub App RSA private key")
	}
	c := NewClient("")
	c.app = &appCredentials{appID: appID, installationID: installationID, key: key, endpoint: "https://api.github.com", now: time.Now, gate: make(chan struct{}, 1)}
	return c, nil
}

func (a *appCredentials) jwt(now time.Time) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": a.appID})
	if err != nil {
		return "", errors.New("cannot encode GitHub App JWT")
	}
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", errors.New("cannot sign GitHub App JWT")
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
func (a *appCredentials) token(ctx context.Context, client *http.Client) (string, error) {
	select {
	case a.gate <- struct{}{}:
		defer func() { <-a.gate }()
	case <-ctx.Done():
		return "", errors.New("GitHub installation authentication canceled")
	}
	if err := ctx.Err(); err != nil {
		return "", errors.New("GitHub installation authentication canceled")
	}
	a.mu.Lock()
	cached, expires := a.cached, a.expires
	a.mu.Unlock()
	if cached != "" && a.now().Add(time.Minute).Before(expires) {
		return cached, nil
	}
	jwt, err := a.jwt(a.now())
	if err != nil {
		return "", err
	}
	permissions := a.permissions
	if permissions == nil {
		permissions = map[string]string{"organization_projects": "write", "issues": "read", "pull_requests": "read"}
	}
	payload, err := json.Marshal(map[string]any{"permissions": permissions})
	if err != nil {
		return "", errors.New("cannot encode installation permissions")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/app/installations/%d/access_tokens", a.endpoint, a.installationID), bytes.NewReader(payload))
	if err != nil {
		return "", errors.New("cannot create GitHub installation token request")
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "ghook4action")
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("GitHub installation token request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("GitHub installation token HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return "", errors.New("cannot read bounded GitHub installation token response")
	}
	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", errors.New("invalid GitHub installation token response")
	}
	if result.Token == "" || strings.ContainsAny(result.Token, " \t\r\n") || !a.now().Add(time.Minute).Before(result.ExpiresAt) {
		return "", errors.New("missing or expiring GitHub installation credentials")
	}
	a.mu.Lock()
	a.cached = result.Token
	a.expires = result.ExpiresAt
	a.mu.Unlock()
	return result.Token, nil
}

// invalidate discards only the credential rejected by GitHub. A delayed 401
// must not clear a newer credential installed by another request.
func (a *appCredentials) invalidate(rejected string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cached == rejected {
		a.cached = ""
		a.expires = time.Time{}
	}
}
