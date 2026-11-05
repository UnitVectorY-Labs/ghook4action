package github

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
func appClient(t *testing.T, key *rsa.PrivateKey) *Client {
	t.Helper()
	c, err := NewAppClient("123", 456, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestAppKeyValidation(t *testing.T) {
	key := testKey(t)
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	for _, block := range []*pem.Block{{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}, {Type: "PRIVATE KEY", Bytes: pkcs8}} {
		if _, err := NewAppClient("123", 1, pem.EncodeToMemory(block)); err != nil {
			t.Fatal(err)
		}
	}
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecBytes, _ := x509.MarshalPKCS8PrivateKey(ec)
	for _, raw := range [][]byte{[]byte("not PEM"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecBytes}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")}), append(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), []byte("trailing")...)} {
		if _, err := NewAppClient("123", 1, raw); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
	good := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	for _, id := range []string{"", "0", "-1", "12x", " 123", "00123"} {
		if _, err := NewAppClient(id, 1, good); err == nil {
			t.Fatal("invalid app ID accepted")
		}
	}
	if _, err := NewAppClient("123", 0, good); err == nil {
		t.Fatal("invalid installation ID accepted")
	}
}
func verifyJWT(t *testing.T, auth string, key *rsa.PrivateKey, now time.Time) {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(auth, "Bearer "), ".")
	if len(parts) != 3 {
		t.Error("invalid JWT")
		return
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var h map[string]string
	json.Unmarshal(header, &h)
	if h["alg"] != "RS256" {
		t.Error("wrong JWT alg")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Error(err)
		return
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, hash[:], signature); err != nil {
		t.Error("invalid JWT signature")
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c struct {
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(claims, &c); err != nil {
		t.Error(err)
	}
	if c.Iss != "123" || c.Iat != now.Add(-time.Minute).Unix() || c.Exp != now.Add(9*time.Minute).Unix() {
		t.Error("invalid JWT claims")
	}
}
func TestAppAuthScopeReuseRefreshAndGraphQL(t *testing.T) {
	key := testKey(t)
	c := appClient(t, key)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	c.app.now = func() time.Time { return now }
	var tokenCalls atomic.Int32
	var graphCalls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/456/access_tokens" {
			tokenCalls.Add(1)
			verifyJWT(t, r.Header.Get("Authorization"), key, now)
			if r.Method != "POST" || r.Header.Get("X-GitHub-Api-Version") == "" {
				t.Error("wrong token request")
			}
			var p struct {
				Permissions map[string]string `json:"permissions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				t.Error(err)
			}
			if len(p.Permissions) != 3 || p.Permissions["organization_projects"] != "write" || p.Permissions["issues"] != "read" || p.Permissions["pull_requests"] != "read" {
				t.Error("wrong permissions")
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"token":"installation-token-%d","expires_at":%q}`, tokenCalls.Load(), now.Add(time.Hour).Format(time.RFC3339))
			return
		}
		graphCalls.Add(1)
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer installation-token-%d", tokenCalls.Load()) {
			t.Error("wrong GraphQL credential")
		}
		fmt.Fprint(w, `{"data":{"ok":true}}`)
	}))
	defer s.Close()
	c.app.endpoint = s.URL
	c.endpoint = s.URL + "/graphql"
	for range 2 {
		var out any
		if err := c.request(context.Background(), "query", nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls.Load() != 1 || graphCalls.Load() != 2 {
		t.Fatal("token not reused")
	}
	now = now.Add(59*time.Minute + time.Second)
	var out any
	if err := c.request(context.Background(), "query", nil, &out); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 2 {
		t.Fatal("token not refreshed ahead of expiry")
	}
}
func TestAppConcurrentReuse(t *testing.T) {
	c := appClient(t, testKey(t))
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"token":"installation-token","expires_at":%q}`, time.Now().Add(time.Hour).Format(time.RFC3339))
	}))
	defer s.Close()
	c.app.endpoint = s.URL
	var wg sync.WaitGroup
	for range 25 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := c.app.token(context.Background(), c.http)
			if err != nil || token != "installation-token" {
				t.Errorf("token %q error %v", token, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("expected single renewal, got %d", calls.Load())
	}
}
func TestAppFailuresRedactedAndNotCached(t *testing.T) {
	c := appClient(t, testKey(t))
	for _, tc := range []struct {
		status int
		body   string
	}{{401, "private installation-token secret"}, {201, `{"token":"installation-token","expires_at":"bad"}`}, {201, `{"token":"installation-token","expires_at":"2000-01-01T00:00:00Z"}`}, {201, `{"token":"","expires_at":"2099-01-01T00:00:00Z"}`}, {201, `{"token":"secret\nheader","expires_at":"2099-01-01T00:00:00Z"}`}, {201, strings.Repeat("secret", maxResponseBytes)}} {
		t.Run(fmt.Sprint(tc.status, len(tc.body)), func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			c.app.endpoint = s.URL
			for range 2 {
				_, err := c.app.token(context.Background(), c.http)
				if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "installation-token") {
					t.Fatalf("unsafe error %v", err)
				}
			}
			if calls != 2 {
				t.Fatal("failed credential cached")
			}
		})
	}
}
func TestAppRedirectRefused(t *testing.T) {
	c := appClient(t, testKey(t))
	var called atomic.Bool
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Store(true) }))
	defer dest.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	c.app.endpoint = source.URL
	if _, err := c.app.token(context.Background(), c.http); err == nil || called.Load() {
		t.Fatal("token JWT followed redirect")
	}
}
func TestAppCancellationDuringRenewalWait(t *testing.T) {
	c := appClient(t, testKey(t))
	c.app.gate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := c.app.token(ctx, c.http); err == nil {
		t.Fatal("cancellation ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("canceled waiter blocked")
	}
	<-c.app.gate
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer s.Close()
	defer close(release)
	c.app.endpoint = s.URL
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.app.token(ctx, c.http); err == nil {
		t.Fatal("request cancellation ignored")
	}
}

func TestCheckProjectCachesLookup(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"data":{"organization":{"projectV2":{"id":"PVT_1"}}}}`)
	}))
	defer s.Close()
	c := NewClient("test")
	c.endpoint = s.URL
	for range 2 {
		if err := c.CheckProject(context.Background(), "Org", 41); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("project validation did not reuse cache")
	}
	if err := c.CheckProject(context.Background(), "Org", 0); err == nil {
		t.Fatal("invalid project accepted")
	}
}

func TestRevokedAppTokenRenewsOnNextGraphQLCall(t *testing.T) {
	c := appClient(t, testKey(t))
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			calls++
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"token":"installation-%d","expires_at":%q}`, calls, time.Now().Add(time.Hour).Format(time.RFC3339))
			return
		}
		if r.Header.Get("Authorization") == "Bearer installation-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer installation-2" {
			t.Error("unexpected credential")
		}
		fmt.Fprint(w, `{"data":{"ok":true}}`)
	}))
	defer s.Close()
	c.endpoint = s.URL + "/graphql"
	c.app.endpoint = s.URL
	var out any
	if err := c.request(context.Background(), "query", nil, &out); err == nil {
		t.Fatal("revoked token accepted")
	}
	if calls != 1 {
		t.Fatal("unexpected inline token retry")
	}
	if err := c.request(context.Background(), "query", nil, &out); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("rejected token not refreshed")
	}
}

func TestStaticUnauthorizedPreservesToken(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer static-token" {
			t.Error("static credential changed")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer s.Close()
	c := NewClient("static-token")
	c.endpoint = s.URL
	for range 2 {
		var out any
		if err := c.request(context.Background(), "query", nil, &out); err == nil {
			t.Fatal("unauthorized accepted")
		}
	}
	if calls != 2 || c.token != "static-token" {
		t.Fatal("static auth changed")
	}
}

func TestDelayedUnauthorizedDoesNotInvalidateNewToken(t *testing.T) {
	c := appClient(t, testKey(t))
	c.app.cached = "old-token"
	c.app.expires = time.Now().Add(time.Hour)
	received := make(chan struct{})
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer old-token" {
			t.Error("expected old token")
		}
		close(received)
		<-release
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer s.Close()
	c.endpoint = s.URL
	done := make(chan error, 1)
	go func() { var out any; done <- c.request(context.Background(), "query", nil, &out) }()
	<-received
	c.app.mu.Lock()
	c.app.cached = "new-token"
	c.app.expires = time.Now().Add(time.Hour)
	c.app.mu.Unlock()
	close(release)
	if err := <-done; err == nil {
		t.Fatal("unauthorized accepted")
	}
	c.app.mu.Lock()
	cached := c.app.cached
	c.app.mu.Unlock()
	if cached != "new-token" {
		t.Fatal("stale unauthorized invalidated refreshed credential")
	}
}
