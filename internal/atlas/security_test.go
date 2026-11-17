package atlas

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdmissionLimitsBeforeHandlers(t *testing.T) {
	guard := NewSecurity()
	guard.burst = 2
	calls := 0
	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	for i := 0; i < 4; i++ {
		r := httptest.NewRequest("GET", "/api/listings", nil)
		r.RemoteAddr = "192.0.2.1:10"
		r.Header.Set("X-Real-IP", fmt.Sprintf("198.51.100.%d", i))
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i))
		res := httptest.NewRecorder()
		h.ServeHTTP(res, r)
		if i < 2 {
			requireStatus(t, res, 204)
		} else {
			requireStatus(t, res, 429)
			if res.Header().Get("Retry-After") == "" {
				t.Fatal("missing retry delay")
			}
		}
	}
	if calls != 2 {
		t.Fatal("spoofed headers bypassed admission")
	}
}
func TestTrustedProxyOriginAndIPv6(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "https://rental.example.com")
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1/32,::1/128")
	guard, err := SecurityFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	received := ""
	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = clientAddress(r); w.WriteHeader(204) }))
	request := func(peer, host, proto, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://rental.example.com/api/auth/login", nil)
		r.TLS = nil
		r.Host = host
		r.RemoteAddr = peer
		r.Header.Set("X-Real-IP", "2001:db8:abcd::1")
		r.Header.Set("X-Forwarded-Proto", proto)
		r.Header.Set("Origin", origin)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, r)
		return res
	}
	res := request("127.0.0.1:99", "rental.example.com", "https", "https://rental.example.com")
	requireStatus(t, res, 204)
	if received != "2001:db8:abcd::1" || res.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("proxy identity or HTTPS lost")
	}
	requireStatus(t, request("203.0.113.1:99", "rental.example.com", "https", "https://rental.example.com"), 400)
	requireStatus(t, request("127.0.0.1:99", "evil.example.com", "https", "https://evil.example.com"), 421)
	requireStatus(t, request("127.0.0.1:99", "rental.example.com", "https", "http://rental.example.com"), 403)
	guard.burst = 1
	guard.clients = map[string]*clientBudget{}
	if !guard.admit(netip.MustParseAddr("2001:db8::1"), false) || guard.admit(netip.MustParseAddr("2001:db8::2"), false) {
		t.Fatal("IPv6 /64 rotation bypass")
	}
}
func TestSecurityConfigurationFailsClosed(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_ORIGIN", "")
	t.Setenv("TRUSTED_PROXIES", "")
	if _, err := SecurityFromEnv(); err == nil {
		t.Fatal("production without domain accepted")
	}
	t.Setenv("PUBLIC_ORIGIN", "https://rental.example.com")
	for _, value := range []string{"0.0.0.0/0", "::/0", "invalid-proxy"} {
		t.Setenv("TRUSTED_PROXIES", value)
		if _, err := SecurityFromEnv(); err == nil {
			t.Fatal("unsafe proxy accepted")
		}
	}
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("MAX_CONCURRENT_UPLOADS", "0")
	if _, err := SecurityFromEnv(); err == nil {
		t.Fatal("disabled upload limit accepted")
	}
}
func TestConcurrencyAdmissionFailsFast(t *testing.T) {
	for _, route := range []string{"/api/auth/login", "/api/listings"} {
		t.Run(route, func(t *testing.T) {
			guard := NewSecurity()
			guard.auth = make(chan struct{}, 1)
			guard.uploads = make(chan struct{}, 1)
			started := make(chan struct{})
			release := make(chan struct{})
			done := make(chan struct{})
			h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.WriteHeader(204) }))
			go func() {
				defer close(done)
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", route, nil))
			}()
			<-started
			second := httptest.NewRecorder()
			h.ServeHTTP(second, httptest.NewRequest("POST", route, nil))
			requireStatus(t, second, 503)
			close(release)
			<-done
		})
	}
}
func TestClientTableIsBoundedAndExpires(t *testing.T) {
	guard := NewSecurity()
	guard.maxClients = 2
	now := time.Now()
	guard.now = func() time.Time { return now }
	for _, ip := range []string{"192.0.2.1", "192.0.2.2"} {
		if !guard.admit(netip.MustParseAddr(ip), false) {
			t.Fatal("admission failed")
		}
	}
	if guard.admit(netip.MustParseAddr("192.0.2.3"), false) || len(guard.clients) != 2 {
		t.Fatal("unbounded IP state")
	}
	now = now.Add(6 * time.Minute)
	if !guard.admit(netip.MustParseAddr("192.0.2.3"), false) || len(guard.clients) != 1 {
		t.Fatal("idle clients not reclaimed")
	}
}
func TestRequestSecurityAndPrivateFiles(t *testing.T) {
	s, _ := testServer(t)
	os.MkdirAll(filepath.Join(s.StaticDir, "assets"), 0700)
	for _, name := range []string{"index.html", ".env.server.local", "rental.db", "assets/app.js.map", "assets/app.js"} {
		if err := os.WriteFile(filepath.Join(s.StaticDir, name), []byte("private-sentinel"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := s.Handler()
	for _, path := range []string{"/.env.server.local", "/rental.db", "/assets/app.js.map", "/data/regions.json", "/runtime/auth-secret.key", "/images/", "/assets/", "/uploads/", "/api/../.env.server.local"} {
		res := get(t, h, path)
		if res.Code == 200 || strings.Contains(res.Body.String(), "private-sentinel") {
			t.Fatalf("private path exposed %s", path)
		}
	}
	res := get(t, h, "/")
	requireStatus(t, res, 200)
	for _, header := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Permissions-Policy"} {
		if res.Header().Get(header) == "" {
			t.Fatal("missing header " + header)
		}
	}
	tooLong := httptest.NewRecorder()
	h.ServeHTTP(tooLong, httptest.NewRequest("GET", "/api/listings?q="+strings.Repeat("x", 5000), nil))
	requireStatus(t, tooLong, 414)
	trace := httptest.NewRecorder()
	h.ServeHTTP(trace, httptest.NewRequest("TRACE", "/", nil))
	requireStatus(t, trace, 405)
	cross := httptest.NewRequest("POST", "/api/listings", nil)
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	blocked := httptest.NewRecorder()
	h.ServeHTTP(blocked, cross)
	requireStatus(t, blocked, 403)
}
func TestPublicFileRootDoesNotFollowEscapingSymlinks(t *testing.T) {
	s, _ := testServer(t)
	secret := filepath.Join(t.TempDir(), "private.json")
	os.WriteFile(secret, []byte("secret"), 0600)
	os.MkdirAll(filepath.Join(s.StaticDir, "assets"), 0700)
	if err := os.Symlink(secret, filepath.Join(s.StaticDir, "assets", "escape.json")); err != nil {
		t.Skip("symlink creation unavailable")
	}
	requireStatus(t, get(t, s.Handler(), "/assets/escape.json"), 404)
}
func TestUploadStorageBudgetConcurrentAndRestart(t *testing.T) {
	dir := t.TempDir()
	uploads := filepath.Join(dir, "runtime", "uploads")
	os.MkdirAll(uploads, 0700)
	os.WriteFile(filepath.Join(uploads, "existing.jpg"), make([]byte, 8), 0600)
	b := newPhotoBudget(dir, 10)
	if b.reserve(3) {
		t.Fatal("existing uploads ignored")
	}
	var wins atomic.Int32
	var wait sync.WaitGroup
	for i := 0; i < 10; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if b.reserve(1) {
				wins.Add(1)
			}
		}()
	}
	wait.Wait()
	if wins.Load() != 2 || b.used != 10 {
		t.Fatal("concurrent quota oversubscribed")
	}
	b.release(1)
	if !b.reserve(1) {
		t.Fatal("reservation not returned")
	}
}
func TestSearchInjectionAndPublicDataMinimization(t *testing.T) {
	s, h := testServer(t)
	for _, path := range []string{"/api/listings?q=%27%20OR%201%3D1--", "/api/listings?ids=%27%20OR%201%3D1--", "/api/listings?tag=%25", "/api/listings/quote%27"} {
		res := get(t, h, path)
		if res.Code >= 500 {
			t.Fatalf("unhandled SQL input at %s", path)
		}
	}
	records, err := s.Store.All(context.Background())
	if err != nil || len(records) != 3 {
		t.Fatal("injection damaged data")
	}
	l := validListing()
	l.ID = "private-contact-test"
	l.Owner = "not-public-account"
	l.CreatedAt = time.Now().Format(time.RFC3339)
	if err := s.Store.Insert(context.Background(), l); err != nil {
		t.Fatal(err)
	}
	res := get(t, h, "/api/listings")
	requireStatus(t, res, 200)
	if strings.Contains(res.Body.String(), l.Contact) || strings.Contains(res.Body.String(), l.Owner) {
		t.Fatal("bulk list leaks contact or account")
	}
	detail := get(t, h, "/api/listings/"+l.ID)
	requireStatus(t, detail, 200)
	if !strings.Contains(detail.Body.String(), l.Contact) || strings.Contains(detail.Body.String(), l.Owner) {
		t.Fatal("public contact or private owner behavior changed")
	}
}
func TestDirectTLSWorksWithoutForwardedHeaders(t *testing.T) {
	guard := NewSecurity()
	guard.Origin, _ = url.Parse("https://rental.example.com")
	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "https://rental.example.com/", nil)
	r.TLS = &tls.ConnectionState{}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, r)
	requireStatus(t, res, 204)
}

func TestMapAndMetroFailureCooldown(t *testing.T) {
	s, _ := testServer(t)
	m := testMetro(t)
	var mapCalls, metroCalls atomic.Int32
	s.Maps.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		mapCalls.Add(1)
		return nil, fmt.Errorf("upstream unavailable")
	})
	m.client.Transport = metroTransport(func(*http.Request) (*http.Response, error) {
		metroCalls.Add(1)
		return nil, fmt.Errorf("upstream unavailable")
	})
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := s.Maps.Read(context.Background(), "330100"); err == nil {
				t.Error("map failure not reported")
			}
			if _, err := m.Read(context.Background(), "330100"); err == nil {
				t.Error("metro failure not reported")
			}
		}()
	}
	wait.Wait()
	if mapCalls.Load() != 1 || metroCalls.Load() != 1 {
		t.Fatalf("retry storm: map=%d metro=%d", mapCalls.Load(), metroCalls.Load())
	}
	s.Maps.failures["330100"] = time.Now().Add(-time.Second)
	m.cache["330100"] = metroEntry{until: time.Now().Add(-time.Second)}
	s.Maps.Read(context.Background(), "330100")
	m.Read(context.Background(), "330100")
	if mapCalls.Load() != 2 || metroCalls.Load() != 2 {
		t.Fatal("upstream retry did not resume after cooldown")
	}
}

func TestStoreRejectsUnsafeOrderAndProductionSQLite(t *testing.T) {
	s, _ := testServer(t)
	if _, err := s.Store.Search(context.Background(), url.Values{}, "", 0, 0, 0, 1, "l.price; DROP TABLE listings"); err == nil {
		t.Fatal("store accepted a raw SQL order")
	}
	t.Setenv("APP_ENV", "production")
	if db, err := OpenStore(context.Background(), t.TempDir(), ""); err == nil {
		db.DB.Close()
		t.Fatal("production accepted SQLite")
	}
}
