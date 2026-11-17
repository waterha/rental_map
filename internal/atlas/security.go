package atlas

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limits are local to this process. A bounded table and fail-fast admission keep
// anonymous requests from allocating unlimited state or waiting on the database.
type Security struct {
	StorageBytes int64
	Origin       *url.URL
	Proxies      []netip.Prefix
	mu           sync.Mutex
	clients      map[string]*clientBudget
	now          func() time.Time
	swept        time.Time
	maxClients   int
	generalRate  float64
	burst        float64
	global       bucket
	requests     chan struct{}
	auth         chan struct{}
	uploads      chan struct{}
}
type bucket struct {
	tokens  float64
	updated time.Time
}
type clientBudget struct {
	read, write bucket
	seen        time.Time
}

func NewSecurity() *Security {
	return &Security{StorageBytes: 1024 << 20, clients: map[string]*clientBudget{}, now: time.Now, maxClients: 16384, generalRate: 10, burst: 60,
		requests: make(chan struct{}, 64), auth: make(chan struct{}, 4), uploads: make(chan struct{}, 2)}
}

func SecurityFromEnv() (*Security, error) {
	s := NewSecurity()
	if raw := strings.TrimSpace(os.Getenv("PUBLIC_ORIGIN")); raw != "" {
		origin, err := url.Parse(raw)
		if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
			return nil, errors.New("PUBLIC_ORIGIN must be the HTTPS origin, without a path")
		}
		s.Origin = origin
	}
	if os.Getenv("APP_ENV") == "production" && s.Origin == nil {
		return nil, errors.New("production requires PUBLIC_ORIGIN")
	}
	for _, raw := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			if addr, e := netip.ParseAddr(raw); e == nil {
				prefix = netip.PrefixFrom(addr, addr.BitLen())
				err = nil
			}
		}
		if err != nil || prefix.Bits() == 0 {
			return nil, errors.New("TRUSTED_PROXIES must contain specific proxy IPs or CIDRs, never all addresses")
		}
		s.Proxies = append(s.Proxies, prefix)
	}
	settings := []struct {
		name          string
		target        *chan struct{}
		fallback, max int
	}{
		{"MAX_CONCURRENT_REQUESTS", &s.requests, 64, 1024}, {"MAX_CONCURRENT_AUTH", &s.auth, 4, 32}, {"MAX_CONCURRENT_UPLOADS", &s.uploads, 2, 16},
	}
	for _, setting := range settings {
		n, err := positiveEnv(setting.name, setting.fallback, setting.max)
		if err != nil {
			return nil, err
		}
		*setting.target = make(chan struct{}, n)
	}
	n, err := positiveEnv("RATE_LIMIT_RPS", 10, 1000)
	if err != nil {
		return nil, err
	}
	s.generalRate = float64(n)
	n, err = positiveEnv("RATE_LIMIT_BURST", 60, 5000)
	if err != nil {
		return nil, err
	}
	s.burst = float64(n)
	n, err = positiveEnv("UPLOAD_STORAGE_MB", 1024, 102400)
	if err != nil {
		return nil, err
	}
	s.StorageBytes = int64(n) << 20
	return s, nil
}
func positiveEnv(name string, fallback, max int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > max {
		return 0, errors.New(name + " is outside its allowed range")
	}
	return n, nil
}
func (s *Security) trusted(addr netip.Addr) bool {
	for _, prefix := range s.Proxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
func remoteIP(remote string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	addr, err := netip.ParseAddr(host)
	return addr.Unmap(), err == nil
}
func (s *Security) identity(r *http.Request) (netip.Addr, bool) {
	peer, ok := remoteIP(r.RemoteAddr)
	secure := r.TLS != nil
	if !ok {
		return netip.Addr{}, secure
	}
	if s.trusted(peer) {
		// Nginx must OVERWRITE these headers, not append a client-supplied chain.
		if forwarded, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
			peer = forwarded.Unmap()
		}
		secure = secure || r.Header.Get("X-Forwarded-Proto") == "https"
	}
	return peer, secure
}
func (b *bucket) take(now time.Time, rate, burst float64) bool {
	if b.updated.IsZero() {
		b.tokens = burst
	} else {
		b.tokens = min(burst, b.tokens+max(0, now.Sub(b.updated).Seconds())*rate)
	}
	b.updated = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
func (s *Security) admit(ip netip.Addr, mutation bool) bool {
	// IPv6 privacy addresses within a /64 share one budget.
	key := "unknown"
	if ip.IsValid() {
		key = ip.String()
		if ip.Is6() {
			key = netip.PrefixFrom(ip, 64).Masked().String()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.global.take(now, 200, 400) {
		return false
	}
	if s.swept.IsZero() || now.Sub(s.swept) >= time.Minute {
		for key, client := range s.clients {
			if now.Sub(client.seen) > 5*time.Minute {
				delete(s.clients, key)
			}
		}
		s.swept = now
	}
	client := s.clients[key]
	if client == nil {
		if len(s.clients) >= s.maxClients {
			return false
		}
		client = &clientBudget{}
		s.clients[key] = client
	}
	client.seen = now
	if !client.read.take(now, s.generalRate, s.burst) {
		return false
	}
	if mutation && !client.write.take(now, 0.5, 30) {
		return false
	}
	return true
}
func acquire(slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Security) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// AMap needs remote scripts, workers and dynamic styles. Avoid unsafe-eval in
		// our own bundle, but allow it in the SDK policy until AMap removes that need.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self' 'unsafe-eval' https://*.amap.com https://*.autonavi.com; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https://*.amap.com https://*.autonavi.com; connect-src 'self' https://*.amap.com https://*.autonavi.com; worker-src 'self' blob:; frame-src https://*.amap.com https://*.autonavi.com")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		ip, secure := s.identity(r)
		if s.Origin != nil {
			if !strings.EqualFold(r.Host, s.Origin.Host) {
				fail(w, 421, "访问域名无效")
				return
			}
			if !secure {
				fail(w, 400, "请使用 HTTPS 访问")
				return
			}
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if len(r.RequestURI) > 4096 {
			fail(w, 414, "请求地址过长")
			return
		}
		cleaned := path.Clean(r.URL.Path)
		for _, part := range strings.Split(cleaned, "/") {
			lower := strings.ToLower(part)
			if strings.HasPrefix(part, ".") || lower == "runtime" || lower == "node_modules" || lower == "data" || strings.HasSuffix(lower, ".map") || strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".sql") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".bak") {
				http.NotFound(w, r)
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete {
			fail(w, 405, "请求方法不支持")
			return
		}
		mutation := r.Method != http.MethodGet && r.Method != http.MethodHead
		if mutation {
			if !sameOrigin(w, r) {
				return
			}
			if s.Origin != nil && r.Header.Get("Origin") != "" && strings.TrimSuffix(r.Header.Get("Origin"), "/") != strings.TrimSuffix(s.Origin.String(), "/") {
				fail(w, 403, "不允许跨站操作")
				return
			}
		}
		if !s.admit(ip, mutation) {
			w.Header().Set("Retry-After", "5")
			fail(w, 429, "访问过于频繁，请稍后重试")
			return
		}
		if !acquire(s.requests) {
			w.Header().Set("Retry-After", "2")
			fail(w, 503, "服务繁忙，请稍后重试")
			return
		}
		defer func() { <-s.requests }()
		var expensive chan struct{}
		if mutation && strings.HasPrefix(r.URL.Path, "/api/auth/") {
			expensive = s.auth
		}
		if mutation && (r.URL.Path == "/api/listings" || strings.HasPrefix(r.URL.Path, "/api/listings/")) {
			expensive = s.uploads
		}
		if expensive != nil {
			if !acquire(expensive) {
				w.Header().Set("Retry-After", "3")
				fail(w, 503, "操作繁忙，请稍后重试")
				return
			}
			defer func() { <-expensive }()
		}
		if ip.IsValid() {
			copy := r.Clone(r.Context())
			copy.RemoteAddr = net.JoinHostPort(ip.String(), "0")
			r = copy
		}
		timeout := 20 * time.Second
		if mutation && (r.URL.Path == "/api/listings" || strings.HasPrefix(r.URL.Path, "/api/listings/")) {
			timeout = 90 * time.Second
		}
		// Context cancellation alone does not unblock a slow HTTP body read/write.
		// Bound socket I/O as well so slow peers cannot hold admission slots forever.
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(timeout))
		_ = controller.SetWriteDeadline(time.Now().Add(timeout))
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
