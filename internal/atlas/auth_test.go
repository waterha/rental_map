package atlas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type capturedMail struct {
	mu      sync.Mutex
	codes   map[string]string
	failure bool
}

func (m *capturedMail) SendCode(ctx context.Context, email, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure {
		return errors.New("SMTP rejected message")
	}
	m.codes[email] = code
	return nil
}
func (m *capturedMail) code(email string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.codes[email]
}

func authTestServer(t *testing.T) (*Auth, http.Handler, *capturedMail) {
	t.Helper()
	t.Setenv("SMTP_FROM", "")
	t.Setenv("SMTP_AUTH_CODE", "")
	t.Setenv("AUTH_ALLOWED_EMAIL_DOMAINS", "")
	t.Setenv("AUTH_COOKIE_SECURE", "")
	server, _ := testServer(t)
	auth, err := NewAuth(context.Background(), server.Store, server.Maps.Dir)
	if err != nil {
		t.Fatal(err)
	}
	mail := &capturedMail{codes: map[string]string{}}
	auth.Mailer = mail
	server.Auth = auth
	return auth, server.Handler(), mail
}
func authPost(h http.Handler, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	request := httptest.NewRequest("POST", "http://example.com"+path, bytes.NewReader(data))
	request.RemoteAddr = "192.0.2.10:3456"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.com")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}
func codeBody(email string) map[string]string { return map[string]string{"email": email} }
func registerBody(email, code string) map[string]string {
	return map[string]string{"email": email, "password": "Test-only-password123!", "code": code}
}
func requireStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
}
func numberOfAccounts(t *testing.T, a *Auth) int {
	t.Helper()
	var count int
	if err := a.Store.DB.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestEmailRegistrationPolicy(t *testing.T) {
	auth, _, _ := authTestServer(t)
	for _, email := range []string{"someone@qq.com", "12345678@qq.com", "alice_123@163.com", "alice_123@126.com", "person.name@gmail.com", "someone@outlook.com", "some.one@hotmail.com", "someone@icloud.com", " Someone@QQ.COM "} {
		if _, err := auth.normalizeEmail(email); err != nil {
			t.Errorf("valid %s: %v", email, err)
		}
	}
	for _, email := range []string{"someone@madeup.invalid", "someone@qq.com.evil.test", "someone@sub.qq.com", "someone@QQ.COM.", "a..b@qq.com", ".abc@qq.com", "a.@qq.com", "a@@qq.com", "a@qq.com", "012345@qq.com", "123@qq.com", "foo\r\nBcc:evil@qq.com", "中文@qq.com", "some one@qq.com", "a@163.com", "123abc@126.com", "ab+c@gmail.com", "xx@gmail.com"} {
		if _, err := auth.normalizeEmail(email); err == nil {
			t.Errorf("invalid mailbox accepted: %q", email)
		}
	}
	for _, password := range []string{"short1", "onlylettersabcdefgh", "1234567890123", strings.Repeat("a", 73) + "1", "Valid12345\n"} {
		if validPassword(password) {
			t.Error("invalid password accepted")
		}
	}
	if !validPassword("Password123!") {
		t.Error("valid password rejected")
	}
}

func TestVerifiedRegistrationLoginLogoutAndStorage(t *testing.T) {
	auth, h, mail := authTestServer(t)
	email := "someone@qq.com"
	sent := authPost(h, "/api/auth/code", codeBody(email))
	requireStatus(t, sent, 200)
	if numberOfAccounts(t, auth) != 0 {
		t.Fatal("sending mail created account")
	}
	code := mail.code(email)
	if len(code) != 6 || strings.Contains(sent.Body.String(), code) {
		t.Fatal("code missing or leaked through response")
	}
	var storedCode string
	if err := auth.Store.DB.QueryRow("SELECT code_hash FROM email_verifications WHERE email=?", email).Scan(&storedCode); err != nil {
		t.Fatal(err)
	}
	if storedCode == code || strings.Contains(storedCode, code) {
		t.Fatal("verification code was not hashed")
	}
	registered := authPost(h, "/api/auth/register", registerBody(" SomeOne@QQ.COM ", code))
	requireStatus(t, registered, 201)
	if numberOfAccounts(t, auth) != 1 {
		t.Fatal("verified account not created")
	}
	var passwordHash string
	if err := auth.Store.DB.QueryRow("SELECT password_hash FROM accounts WHERE email=?", email).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("Test-only-password123!")); err != nil {
		t.Fatal("password was not hashed correctly")
	}
	if strings.Contains(registered.Body.String(), "password") || strings.Contains(registered.Body.String(), code) {
		t.Fatal("registration leaks private fields")
	}
	var cookie *http.Cookie
	for _, candidate := range registered.Result().Cookies() {
		if candidate.Name == authCookie {
			cookie = candidate
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("missing safe session cookie")
	}
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.AddCookie(cookie)
	me := httptest.NewRecorder()
	h.ServeHTTP(me, req)
	requireStatus(t, me, 200)
	if !strings.Contains(me.Body.String(), email) {
		t.Fatal("authenticated session not returned")
	}
	unauthenticated := get(t, h, "/api/auth/me")
	if !strings.Contains(unauthenticated.Body.String(), `"user":null`) {
		t.Fatal("guest exposed user")
	}
	var sessionHash string
	if err := auth.Store.DB.QueryRow("SELECT token_hash FROM account_sessions").Scan(&sessionHash); err != nil {
		t.Fatal(err)
	}
	if sessionHash == cookie.Value {
		t.Fatal("raw session stored in database")
	}
	replay := authPost(h, "/api/auth/register", registerBody(email, code))
	requireStatus(t, replay, 400)
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 409)
	requireStatus(t, authPost(h, "/api/auth/logout", nil, cookie), 200)
	user, err := auth.Current(req)
	if err != nil || user != nil {
		t.Fatal("logout did not revoke session")
	}
	requireStatus(t, authPost(h, "/api/auth/login", map[string]string{"email": email, "password": "Wrong12345!"}), 401)
	logged := authPost(h, "/api/auth/login", map[string]string{"email": email, "password": "Test-only-password123!"})
	requireStatus(t, logged, 200)
	if len(logged.Result().Cookies()) == 0 || logged.Result().Cookies()[0].Value == cookie.Value {
		t.Fatal("login did not rotate session")
	}
}

func TestCodeExpiresAtExactlyTwoMinutes(t *testing.T) {
	auth, h, mail := authTestServer(t)
	now := time.Now()
	auth.now = func() time.Time { return now }
	email := "expires@qq.com"
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 200)
	code := mail.code(email)
	now = now.Add(120 * time.Second)
	requireStatus(t, authPost(h, "/api/auth/register", registerBody(email, code)), 400)
	if numberOfAccounts(t, auth) != 0 {
		t.Fatal("expired code created account")
	}
}

func TestCodeWorksJustBeforeExpiryAndSessionExpires(t *testing.T) {
	auth, h, mail := authTestServer(t)
	now := time.Now()
	auth.now = func() time.Time { return now }
	email := "validtime@qq.com"
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 200)
	now = now.Add(119 * time.Second)
	registered := authPost(h, "/api/auth/register", registerBody(email, mail.code(email)))
	requireStatus(t, registered, 201)
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.AddCookie(registered.Result().Cookies()[0])
	now = now.Add(sessionTTL * time.Second)
	user, err := auth.Current(req)
	if err != nil || user != nil {
		t.Fatal("expired session authenticated")
	}
}

func TestResendInvalidatesPreviousCodeAndPersists(t *testing.T) {
	auth, h, mail := authTestServer(t)
	now := time.Now()
	auth.now = func() time.Time { return now }
	email := "resends@qq.com"
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 200)
	old := mail.code(email)
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 429)
	now = now.Add(60 * time.Second)
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 200)
	fresh := mail.code(email)
	// A rare random collision is valid; rotate again in that case rather than creating a flaky test.
	if old == fresh {
		now = now.Add(60 * time.Second)
		requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 200)
		fresh = mail.code(email)
		if old == fresh {
			t.Skip("random OTP collision")
		}
	}
	requireStatus(t, authPost(h, "/api/auth/register", registerBody(email, old)), 400)
	var sequence int
	var name, databasePath string
	if err := auth.Store.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &name, &databasePath); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewAuth(context.Background(), auth.Store, filepath.Dir(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = func() time.Time { return now }
	mux := http.NewServeMux()
	restarted.Routes(mux)
	requireStatus(t, authPost(mux, "/api/auth/register", registerBody(email, fresh)), 201)
}

func TestWrongCodeAttemptsAndMailboxBinding(t *testing.T) {
	auth, h, mail := authTestServer(t)
	email := "attempts@qq.com"
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 200)
	code := mail.code(email)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		requireStatus(t, authPost(h, "/api/auth/register", registerBody(email, wrong)), 400)
	}
	requireStatus(t, authPost(h, "/api/auth/register", registerBody(email, code)), 429)
	requireStatus(t, authPost(h, "/api/auth/register", registerBody("another@qq.com", code)), 400)
	if numberOfAccounts(t, auth) != 0 {
		t.Fatal("wrong code created account")
	}
}

func TestMailFailureAndMissingConfigurationNeverAllowSignup(t *testing.T) {
	auth, h, mail := authTestServer(t)
	mail.failure = true
	requireStatus(t, authPost(h, "/api/auth/code", codeBody("failure@qq.com")), 502)
	var codes int
	_ = auth.Store.DB.QueryRow("SELECT COUNT(*) FROM email_verifications").Scan(&codes)
	if codes != 0 {
		t.Fatal("failed email left usable code")
	}
	auth.Mailer = nil
	requireStatus(t, authPost(h, "/api/auth/code", codeBody("nomailer@qq.com")), 503)
	requireStatus(t, authPost(h, "/api/auth/register", registerBody("nomailer@qq.com", "123456")), 400)
	if numberOfAccounts(t, auth) != 0 {
		t.Fatal("unverified account created")
	}
}

func TestAuthCrossSiteMalformedRequestsAndRateLimits(t *testing.T) {
	auth, h, _ := authTestServer(t)
	req := httptest.NewRequest("POST", "http://example.com/api/auth/code", strings.NewReader(`{"email":"someone@qq.com"}`))
	req.Header.Set("Origin", "https://attacker.invalid")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	requireStatus(t, res, 403)
	req = httptest.NewRequest("POST", "/api/auth/code", strings.NewReader(`{"email":"someone@qq.com","extra":"unexpected"}`))
	req.Header.Set("Content-Type", "application/json")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	requireStatus(t, res, 400)
	requireStatus(t, authPost(h, "/api/auth/code", codeBody("someone@madeup.invalid")), 400)
	for i := 0; i < 5; i++ {
		res = authPost(h, "/api/auth/code", codeBody("ratelimit@qq.com"))
		if res.Code != 200 && res.Code != 429 {
			t.Fatal(res.Code)
		}
	}
	requireStatus(t, authPost(h, "/api/auth/code", codeBody("ratelimit@qq.com")), 429)
	if numberOfAccounts(t, auth) != 0 {
		t.Fatal("invalid request created account")
	}
}

func TestConcurrentCodeUseOnlyCreatesOneAccount(t *testing.T) {
	auth, h, mail := authTestServer(t)
	email := "concurrent@qq.com"
	requireStatus(t, authPost(h, "/api/auth/code", codeBody(email)), 200)
	body := registerBody(email, mail.code(email))
	responses := make(chan int, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() { defer wait.Done(); responses <- authPost(h, "/api/auth/register", body).Code }()
	}
	wait.Wait()
	close(responses)
	successes := 0
	for status := range responses {
		if status == 201 {
			successes++
		}
	}
	if successes != 1 || numberOfAccounts(t, auth) != 1 {
		t.Fatal("concurrent OTP replay succeeded")
	}
}

func TestAuthProtectedPublishing(t *testing.T) {
	_, h, _ := authTestServer(t)
	requireStatus(t, authPost(h, "/api/listings", validListing()), 401)
}
