package atlas

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const codeTTL = 120
const resendWait = 60
const maxCodeAttempts = 5
const authCookie = "atlas_account"
const sessionTTL = 7 * 24 * 60 * 60

var defaultDomains = []string{"qq.com", "163.com", "126.com", "gmail.com", "outlook.com", "hotmail.com", "icloud.com"}
var emailLocal = regexp.MustCompile(`^[a-z0-9]+(?:[._+-][a-z0-9]+)*$`)
var sixDigits = regexp.MustCompile(`^[0-9]{6}$`)

type Account struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
type Auth struct {
	Store        *Store
	Mailer       CodeMailer
	Domains      []string
	secret       []byte
	now          func() time.Time
	CookieSecure bool
	dummyHash    []byte
	nextCleanup  atomic.Int64
}

func NewAuth(ctx context.Context, store *Store, directory string) (*Auth, error) {
	schema := `CREATE TABLE IF NOT EXISTS accounts (
 id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, verified_at BIGINT NOT NULL, created_at BIGINT NOT NULL
 );
 CREATE TABLE IF NOT EXISTS email_verifications (
 email TEXT PRIMARY KEY, challenge TEXT NOT NULL, code_hash TEXT NOT NULL, expires_at BIGINT NOT NULL,
 sent_at BIGINT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, state TEXT NOT NULL
 );
 CREATE TABLE IF NOT EXISTS account_sessions (
 token_hash TEXT PRIMARY KEY, account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 expires_at BIGINT NOT NULL, created_at BIGINT NOT NULL
 );
 CREATE INDEX IF NOT EXISTS idx_account_session_expiry ON account_sessions(expires_at);
 CREATE TABLE IF NOT EXISTS auth_rate_limits (
 key_hash TEXT PRIMARY KEY, window_start BIGINT NOT NULL, hits INTEGER NOT NULL
 );`
	if _, err := store.DB.ExecContext(ctx, schema); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(directory, "runtime"), 0700); err != nil {
		return nil, err
	}
	secretFile := filepath.Join(directory, "runtime", "auth-secret.key")
	secret, err := os.ReadFile(secretFile)
	if errors.Is(err, os.ErrNotExist) {
		secret = make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return nil, err
		}
		file, e := os.OpenFile(secretFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(e, os.ErrExist) {
			secret, err = os.ReadFile(secretFile)
		} else if e != nil {
			return nil, e
		} else {
			_, err = file.Write(secret)
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if len(secret) != 32 {
		return nil, errors.New("invalid local authentication secret")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("unused-password-for-timing-only"), 12)
	if err != nil {
		return nil, err
	}
	domains := append([]string(nil), defaultDomains...)
	if configured := strings.TrimSpace(os.Getenv("AUTH_ALLOWED_EMAIL_DOMAINS")); configured != "" {
		permitted := map[string]bool{}
		for _, domain := range defaultDomains {
			permitted[domain] = true
		}
		domains = nil
		for _, domain := range strings.Split(configured, ",") {
			domain = strings.ToLower(strings.TrimSpace(domain))
			if !permitted[domain] {
				return nil, errors.New("AUTH_ALLOWED_EMAIL_DOMAINS contains an unsupported domain")
			}
			if !containsString(domains, domain) {
				domains = append(domains, domain)
			}
		}
	}
	sort.Strings(domains)
	auth := &Auth{Store: store, Domains: domains, secret: secret, now: time.Now, CookieSecure: os.Getenv("AUTH_COOKIE_SECURE") == "true", dummyHash: dummy}
	sender := strings.ToLower(strings.TrimSpace(os.Getenv("SMTP_FROM")))
	password := strings.TrimSpace(os.Getenv("SMTP_AUTH_CODE"))
	if sender != "" && !validQQSender(sender) {
		return nil, errors.New("SMTP_FROM must be a numeric QQ email address")
	}
	if password != "" && sender == "" {
		return nil, errors.New("SMTP_FROM is required when SMTP_AUTH_CODE is configured")
	}
	if sender != "" && password != "" && os.Getenv("SMTP_DISABLED") != "true" {
		auth.Mailer = &QQMailer{Sender: sender, Password: password}
	}
	return auth, nil
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func (a *Auth) digest(purpose, value string) string {
	hash := hmac.New(sha256.New, a.secret)
	hash.Write([]byte(purpose + "\x00" + value))
	return hex.EncodeToString(hash.Sum(nil))
}
func randomToken(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
func verificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return strings.Repeat("0", 6-len(n.String())) + n.String(), nil
}

func (a *Auth) normalizeEmail(input string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(input))
	if len(email) > 254 || strings.Count(email, "@") != 1 {
		return "", errors.New("请输入有效邮箱地址")
	}
	parts := strings.Split(email, "@")
	local, domain := parts[0], parts[1]
	if !containsString(a.Domains, domain) {
		return "", errors.New("暂不支持该邮箱域名，请使用页面列出的邮箱")
	}
	if len(local) > 64 || !emailLocal.MatchString(local) {
		return "", errors.New("邮箱用户名格式不正确")
	}
	switch domain {
	case "qq.com":
		if len(local) < 3 {
			return "", errors.New("QQ 邮箱用户名不正确")
		}
		if regexp.MustCompile(`^[0-9]+$`).MatchString(local) && (!validQQSender(email)) {
			return "", errors.New("QQ 号码邮箱格式不正确")
		}
	case "163.com", "126.com":
		if !regexp.MustCompile(`^[a-z][a-z0-9_]{5,17}$`).MatchString(local) {
			return "", errors.New("网易邮箱用户名应为 6–18 位字母、数字或下划线，并以字母开头")
		}
	case "gmail.com":
		if !regexp.MustCompile(`^[a-z0-9]+(?:\.[a-z0-9]+)*$`).MatchString(local) || len(strings.ReplaceAll(local, ".", "")) < 6 || len(strings.ReplaceAll(local, ".", "")) > 30 {
			return "", errors.New("Gmail 用户名格式不正确")
		}
	}
	return email, nil
}

func validPassword(password string) bool {
	if len(password) < 10 || len(password) > 72 {
		return false
	}
	letter, digit := false, false
	for _, r := range password {
		if r < 32 || r == 127 {
			return false
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			letter = true
		}
		if r >= '0' && r <= '9' {
			digit = true
		}
	}
	return letter && digit
}

func (a *Auth) allowed(ctx context.Context, key string, limit int, window int64) (bool, error) {
	now := a.now().Unix()
	bucket := now / window * window
	next := a.nextCleanup.Load()
	if now >= next && a.nextCleanup.CompareAndSwap(next, now+3600) {
		_, _ = a.Store.DB.ExecContext(ctx, a.Store.bind("DELETE FROM auth_rate_limits WHERE window_start < ?"), now-86400)
		_, _ = a.Store.DB.ExecContext(ctx, a.Store.bind("DELETE FROM email_verifications WHERE expires_at < ?"), now-86400)
		_, _ = a.Store.DB.ExecContext(ctx, a.Store.bind("DELETE FROM account_sessions WHERE expires_at <= ?"), now)
	}
	result, err := a.Store.DB.ExecContext(ctx, a.Store.bind(`INSERT INTO auth_rate_limits(key_hash,window_start,hits) VALUES(?,?,1)
 ON CONFLICT(key_hash) DO UPDATE SET window_start=excluded.window_start,
 hits=CASE WHEN auth_rate_limits.window_start=excluded.window_start THEN auth_rate_limits.hits+1 ELSE 1 END
 WHERE auth_rate_limits.window_start<>excluded.window_start OR auth_rate_limits.hits<?`), a.digest("rate", key), bucket, limit)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
func sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		fail(w, 403, "不允许跨站操作")
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
			fail(w, 403, "不允许跨站操作")
			return false
		}
	}
	return true
}
func readAuthJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if !sameOrigin(w, r) {
		return false
	}
	media := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
	if media != "application/json" {
		fail(w, 415, "请使用 JSON 提交")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(w, 400, "提交内容格式错误或过大")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		fail(w, 400, "提交内容格式错误")
		return false
	}
	return true
}
func (a *Auth) rate(w http.ResponseWriter, r *http.Request, key string, limit int, window int64) bool {
	allowed, err := a.allowed(r.Context(), key, limit, window)
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return false
	}
	if !allowed {
		w.Header().Set("Retry-After", strconv.FormatInt(window, 10))
		fail(w, 429, "操作过于频繁，请稍后重试")
		return false
	}
	return true
}

func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/config", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]any{"allowedDomains": a.Domains, "codeExpiresIn": codeTTL, "resendAfter": resendWait, "mailAvailable": a.Mailer != nil, "passwordMinLength": 10, "passwordMaxBytes": 72})
	})
	mux.HandleFunc("POST /api/auth/code", a.sendCode)
	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		user, err := a.Current(r)
		if err != nil {
			fail(w, 503, "账号服务暂不可用")
			return
		}
		send(w, 200, map[string]any{"user": user})
	})
}

func (a *Auth) sendCode(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if !readAuthJSON(w, r, &input) {
		return
	}
	email, err := a.normalizeEmail(input.Email)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if a.Mailer == nil {
		fail(w, 503, "邮件服务尚未配置，暂时无法发送验证码")
		return
	}
	if !a.rate(w, r, "send-global", 100, 3600) || !a.rate(w, r, "send-ip:"+clientAddress(r), 5, 600) || !a.rate(w, r, "send-email:"+email, 10, 3600) {
		return
	}
	var exists int
	err = a.Store.DB.QueryRowContext(r.Context(), a.Store.bind("SELECT COUNT(*) FROM accounts WHERE email=?"), email).Scan(&exists)
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	if exists > 0 {
		fail(w, 409, "该邮箱已注册，请直接登录")
		return
	}
	code, err := verificationCode()
	if err != nil {
		fail(w, 500, "验证码生成失败")
		return
	}
	challenge, err := randomToken(16)
	if err != nil {
		fail(w, 500, "验证码生成失败")
		return
	}
	now := a.now().Unix()
	codeHash := a.digest("code", email+":"+challenge+":"+code)
	result, err := a.Store.DB.ExecContext(r.Context(), a.Store.bind(`INSERT INTO email_verifications(email,challenge,code_hash,expires_at,sent_at,attempts,state) VALUES(?,?,?,?,?,0,'pending')
 ON CONFLICT(email) DO UPDATE SET challenge=excluded.challenge,code_hash=excluded.code_hash,expires_at=excluded.expires_at,sent_at=excluded.sent_at,attempts=0,state='pending'
 WHERE email_verifications.sent_at<=?`), email, challenge, codeHash, now+codeTTL, now, now-resendWait)
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	if affected != 1 {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "请等待 60 秒后再发送验证码")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err = a.Mailer.SendCode(ctx, email, code); err != nil {
		// Never log SMTP errors, credentials, recipients or verification codes.
		_, _ = a.Store.DB.ExecContext(context.Background(), a.Store.bind("DELETE FROM email_verifications WHERE email=? AND challenge=?"), email, challenge)
		fail(w, 502, "验证码邮件发送失败，请稍后重试或联系管理员检查邮件配置")
		return
	}
	now = a.now().Unix()
	result, err = a.Store.DB.ExecContext(r.Context(), a.Store.bind("UPDATE email_verifications SET state='sent',expires_at=?,sent_at=? WHERE email=? AND challenge=?"), now+codeTTL, now, email, challenge)
	if err != nil {
		fail(w, 503, "验证码未能生效，请稍后重新发送")
		return
	}
	affected, _ = result.RowsAffected()
	if affected != 1 {
		fail(w, 409, "验证码已被新的发送请求替换，请重新获取")
		return
	}
	send(w, 200, map[string]any{"message": "验证码已发送，请查收邮件", "expiresIn": codeTTL, "resendAfter": resendWait})
}

func (a *Auth) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !readAuthJSON(w, r, &input) {
		return
	}
	email, err := a.normalizeEmail(input.Email)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if !validPassword(input.Password) {
		fail(w, 400, "密码需为 10–72 字节，包含英文字母与数字")
		return
	}
	if !sixDigits.MatchString(input.Code) {
		fail(w, 400, "请输入 6 位数字验证码")
		return
	}
	if !a.rate(w, r, "register-ip:"+clientAddress(r), 20, 600) || !a.rate(w, r, "register-email:"+email, 15, 600) {
		return
	}
	id, err := randomToken(16)
	if err != nil {
		fail(w, 500, "注册暂不可用")
		return
	}
	token, err := randomToken(32)
	if err != nil {
		fail(w, 500, "注册暂不可用")
		return
	}
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	defer tx.Rollback()
	query := "SELECT challenge,code_hash,expires_at,attempts,state FROM email_verifications WHERE email=?"
	if a.Store.Postgres {
		query += " FOR UPDATE"
	}
	var challenge, expected, state string
	var expires int64
	var attempts int
	err = tx.QueryRowContext(r.Context(), a.Store.bind(query), email).Scan(&challenge, &expected, &expires, &attempts, &state)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 400, "验证码无效，请先获取验证码")
		return
	}
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	now := a.now().Unix()
	if state != "sent" || now >= expires {
		fail(w, 400, "验证码已过期或尚未发送成功，请重新获取")
		return
	}
	if attempts >= maxCodeAttempts {
		fail(w, 429, "验证码错误次数已达上限，请重新获取")
		return
	}
	actual := a.digest("code", email+":"+challenge+":"+input.Code)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		_, err = tx.ExecContext(r.Context(), a.Store.bind("UPDATE email_verifications SET attempts=attempts+1 WHERE email=? AND challenge=?"), email, challenge)
		if err != nil {
			fail(w, 503, "账号服务暂不可用")
			return
		}
		if err = tx.Commit(); err != nil {
			fail(w, 503, "账号服务暂不可用")
			return
		}
		fail(w, 400, "验证码错误，请检查邮件中的验证码")
		return
	}
	// Reject missing/expired/incorrect OTPs before doing expensive password work.
	// Valid registrations hold this transaction briefly; admission limits bound bcrypt concurrency.
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		fail(w, 500, "注册暂不可用")
		return
	}
	result, err := tx.ExecContext(r.Context(), a.Store.bind("INSERT INTO accounts(id,email,password_hash,verified_at,created_at) VALUES(?,?,?,?,?) ON CONFLICT(email) DO NOTHING"), id, email, string(passwordHash), now, now)
	if err != nil {
		fail(w, 503, "注册失败，请重试")
		return
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		fail(w, 409, "该邮箱已注册，请直接登录")
		return
	}
	if _, err = tx.ExecContext(r.Context(), a.Store.bind("DELETE FROM email_verifications WHERE email=? AND challenge=?"), email, challenge); err != nil {
		fail(w, 503, "注册失败，请重试")
		return
	}
	if _, err = tx.ExecContext(r.Context(), a.Store.bind("INSERT INTO account_sessions(token_hash,account_id,expires_at,created_at) VALUES(?,?,?,?)"), a.digest("session", token), id, now+sessionTTL, now); err != nil {
		fail(w, 503, "注册失败，请重试")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 503, "注册失败，请重试")
		return
	}
	a.cookie(w, r, token, sessionTTL)
	send(w, 201, map[string]any{"user": Account{ID: id, Email: email}})
}

func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readAuthJSON(w, r, &input) {
		return
	}
	email, err := a.normalizeEmail(input.Email)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if len(input.Password) > 72 || len(input.Password) < 1 {
		fail(w, 400, "邮箱或密码错误")
		return
	}
	if !a.rate(w, r, "login-ip:"+clientAddress(r), 30, 900) || !a.rate(w, r, "login-email:"+email, 10, 900) {
		return
	}
	var user Account
	var passwordHash string
	err = a.Store.DB.QueryRowContext(r.Context(), a.Store.bind("SELECT id,email,password_hash FROM accounts WHERE email=?"), email).Scan(&user.ID, &user.Email, &passwordHash)
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	hash := []byte(passwordHash)
	if missing {
		hash = a.dummyHash
	}
	compare := bcrypt.CompareHashAndPassword(hash, []byte(input.Password))
	if missing || compare != nil {
		fail(w, 401, "邮箱或密码错误")
		return
	}
	token, err := randomToken(32)
	if err != nil {
		fail(w, 500, "登录失败，请重试")
		return
	}
	now := a.now().Unix()
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), a.Store.bind("DELETE FROM account_sessions WHERE expires_at<=?"), now); err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	if cookie, e := r.Cookie(authCookie); e == nil {
		if _, err = tx.ExecContext(r.Context(), a.Store.bind("DELETE FROM account_sessions WHERE token_hash=?"), a.digest("session", cookie.Value)); err != nil {
			fail(w, 503, "账号服务暂不可用")
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), a.Store.bind("INSERT INTO account_sessions(token_hash,account_id,expires_at,created_at) VALUES(?,?,?,?)"), a.digest("session", token), user.ID, now+sessionTTL, now); err != nil {
		fail(w, 503, "登录失败，请重试")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 503, "登录失败，请重试")
		return
	}
	a.cookie(w, r, token, sessionTTL)
	send(w, 200, map[string]any{"user": user})
}

func (a *Auth) Current(r *http.Request) (*Account, error) {
	cookie, err := r.Cookie(authCookie)
	if err != nil || len(cookie.Value) != 64 {
		return nil, nil
	}
	if _, err = hex.DecodeString(cookie.Value); err != nil {
		return nil, nil
	}
	var user Account
	err = a.Store.DB.QueryRowContext(r.Context(), a.Store.bind(`SELECT accounts.id,accounts.email FROM account_sessions JOIN accounts ON accounts.id=account_sessions.account_id WHERE account_sessions.token_hash=? AND account_sessions.expires_at>?`), a.digest("session", cookie.Value), a.now().Unix()).Scan(&user.ID, &user.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
func (a *Auth) cookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: authCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.CookieSecure || r.TLS != nil, MaxAge: maxAge})
}
func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	if cookie, err := r.Cookie(authCookie); err == nil {
		if _, err = a.Store.DB.ExecContext(r.Context(), a.Store.bind("DELETE FROM account_sessions WHERE token_hash=?"), a.digest("session", cookie.Value)); err != nil {
			fail(w, 503, "退出失败，请重试")
			return
		}
	}
	a.cookie(w, r, "", -1)
	send(w, 200, map[string]bool{"ok": true})
}
