package atlas

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type managementFixture struct {
	server  *Server
	auth    *Auth
	handler http.Handler
	listing Listing
	cookies map[string]*http.Cookie
}

func newManagementFixture(t *testing.T) *managementFixture {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("SMTP_AUTH_CODE", "")
	t.Setenv("AUTH_ALLOWED_EMAIL_DOMAINS", "")
	t.Setenv("AUTH_ADMIN_EMAILS", "900000003@qq.com")
	t.Setenv("AUTH_ADMIN_BOOTSTRAP_EMAIL", "")
	t.Setenv("AUTH_ADMIN_BOOTSTRAP_PASSWORD", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "regions.json"), []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	maps, err := NewMaps(dir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.DB.Close() })
	auth, err := NewAuth(context.Background(), store, dir)
	if err != nil {
		t.Fatal(err)
	}
	f := &managementFixture{auth: auth, cookies: map[string]*http.Cookie{}}
	for index, id := range []string{"publisher", "stranger", "administrator"} {
		email := []string{"900000001@qq.com", "900000002@qq.com", "900000003@qq.com"}[index]
		_, err = store.DB.Exec(`INSERT INTO accounts(id,email,password_hash,verified_at,created_at) VALUES(?,?,?,1,1)`, id, email, "test-hash")
		if err != nil {
			t.Fatal(err)
		}
		token := strings.Repeat(string(rune('a'+index)), 64)
		_, err = store.DB.Exec(`INSERT INTO account_sessions(token_hash,account_id,expires_at,created_at) VALUES(?,?,?,1)`, auth.digest("session", token), id, time.Now().Unix()+3600)
		if err != nil {
			t.Fatal(err)
		}
		f.cookies[id] = &http.Cookie{Name: authCookie, Value: token}
	}
	upload := "/uploads/" + strings.Repeat("a", 48) + ".jpg"
	if err = os.MkdirAll(filepath.Join(dir, "runtime", "uploads"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "runtime", "uploads", filepath.Base(upload)), []byte("old photo"), 0600); err != nil {
		t.Fatal(err)
	}
	f.listing = Listing{ID: "managed-home", Owner: "publisher", Title: "西湖区阳光房源", Description: "采光很好交通方便随时可以入住", Type: "整租", Price: 3000, Area: 45, Rooms: 1, ProvinceCode: "330000", CityCode: "330100", DistrictCode: "330106", City: "杭州市", District: "西湖区", Address: "杭州市西湖区", Longitude: 120.13, Latitude: 30.27, Tags: []string{}, Contact: "test-contact", CreatedAt: "2026-11-18T00:00:00Z", Image: upload, ListingDetails: ListingDetails{Images: []string{upload}, RentalSource: "房东直租", PropertyType: "住宅", Bathrooms: 1}}
	if err = store.Insert(context.Background(), f.listing); err != nil {
		t.Fatal(err)
	}
	f.server = &Server{Store: store, Maps: maps, Auth: auth, AllowPublish: true, StaticDir: t.TempDir()}
	f.handler = f.server.Handler()
	return f
}

func (f *managementFixture) request(method, path, actor string, body *bytes.Buffer, contentType, origin string) *httptest.ResponseRecorder {
	if body == nil {
		body = &bytes.Buffer{}
	}
	r := httptest.NewRequest(method, path, body)
	if cookie := f.cookies[actor]; cookie != nil {
		r.AddCookie(cookie)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func managementBody(t *testing.T, listing Listing, addPhoto bool) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	data, err := json.Marshal(listing)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.WriteField("listing", string(data)); err != nil {
		t.Fatal(err)
	}
	if addPhoto {
		part, err := w.CreateFormFile("photos", "new-photo.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if err = jpeg.Encode(part, image.NewRGBA(image.Rect(0, 0, 12, 12)), nil); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return body, w.FormDataContentType()
}

func TestListingManagementPermissions(t *testing.T) {
	f := newManagementFixture(t)
	for _, actor := range []string{"", "publisher", "stranger", "administrator"} {
		t.Run("detail-"+actor, func(t *testing.T) {
			w := f.request("GET", "/api/listings/managed-home", actor, nil, "", "")
			if w.Code != 200 {
				t.Fatalf("detail returned %d: %s", w.Code, w.Body)
			}
			var l Listing
			if err := json.Unmarshal(w.Body.Bytes(), &l); err != nil {
				t.Fatal(err)
			}
			if l.CanManage != (actor == "publisher" || actor == "administrator") {
				t.Fatal("incorrect management permission")
			}
			if strings.Contains(w.Body.String(), `"owner"`) || strings.Contains(w.Body.String(), "900000001@qq.com") {
				t.Fatal("owner identity exposed")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("personal permission may be cached")
			}
		})
	}
	for _, actor := range []string{"publisher", "administrator"} {
		w := f.request("GET", "/api/auth/me", actor, nil, "", "")
		var result struct {
			User Account `json:"user"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		want := "user"
		if actor == "administrator" {
			want = "admin"
		}
		if result.User.Role != want {
			t.Fatal("incorrect authenticated role", result.User.Role)
		}
	}
	for _, method := range []string{"PUT", "DELETE"} {
		for _, actor := range []string{"", "stranger"} {
			w := f.request(method, "/api/listings/managed-home", actor, nil, "", "")
			want := 403
			if actor == "" {
				want = 401
			}
			if w.Code != want {
				t.Fatalf("%s by %q: %d, want %d", method, actor, w.Code, want)
			}
		}
		w := f.request(method, "/api/listings/managed-home", "publisher", nil, "", "https://attacker.example")
		if w.Code != 403 {
			t.Fatalf("cross origin %s accepted", method)
		}
	}
	if w := f.request("DELETE", "/api/listings/missing", "publisher", nil, "", ""); w.Code != 404 {
		t.Fatal("missing listing did not return 404")
	}
	for _, actor := range []string{"publisher", "administrator"} {
		updated := f.listing
		updated.Title, updated.Price = "修改后的西湖区房源", 3500
		updated.ID, updated.CreatedAt, updated.DistrictCode = "forged-id", "forged-date", "330110"
		updated.Longitude, updated.Latitude = 0, 0
		body, contentType := managementBody(t, updated, false)
		w := f.request("PUT", "/api/listings/managed-home", actor, body, contentType, "")
		if w.Code != 200 {
			t.Fatalf("update by %s: %d %s", actor, w.Code, w.Body)
		}
		actual, err := f.server.Store.Find(context.Background(), f.listing.ID)
		if err != nil {
			t.Fatal(err)
		}
		if actual.Owner != f.listing.Owner || actual.ID != f.listing.ID || actual.CreatedAt != f.listing.CreatedAt || actual.DistrictCode != f.listing.DistrictCode || actual.Longitude != f.listing.Longitude || actual.Price != 3500 {
			t.Fatal("immutable fields changed or update lost")
		}
	}
	// Database operations themselves reject a non-owner, even without the HTTP layer.
	if err := f.server.Store.UpdateListing(context.Background(), f.listing, Account{ID: "stranger", Role: "user"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("SQL update lacked ownership check", err)
	}
	if err := f.server.Store.DeleteListing(context.Background(), f.listing.ID, Account{ID: "stranger", Role: "user"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("SQL delete lacked ownership check", err)
	}
	w := f.request("DELETE", "/api/listings/managed-home", "administrator", nil, "", "")
	if w.Code != 200 {
		t.Fatalf("admin deletion: %d %s", w.Code, w.Body)
	}
	if _, err := f.server.Store.Find(context.Background(), f.listing.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("listing survived deletion")
	}
	var count int
	if err := f.server.Store.DB.QueryRow("SELECT COUNT(*) FROM listing_details").Scan(&count); err != nil || count != 0 {
		t.Fatal("orphan metadata", count, err)
	}
	if _, err := os.Stat(filepath.Join(f.server.Maps.Dir, "runtime", "uploads", filepath.Base(f.listing.Image))); !os.IsNotExist(err) {
		t.Fatal("photo survived deletion", err)
	}
	if w = f.request("GET", "/api/listings/managed-home", "", nil, "", ""); w.Code != 404 {
		t.Fatal("deleted listing still visible")
	}
}

func TestListingPhotoOwnershipAndCleanup(t *testing.T) {
	f := newManagementFixture(t)
	invalid := f.listing
	invalid.Images = []string{"/uploads/" + strings.Repeat("b", 48) + ".jpg"}
	body, contentType := managementBody(t, invalid, false)
	if w := f.request("PUT", "/api/listings/managed-home", "administrator", body, contentType, ""); w.Code != 400 {
		t.Fatal("foreign photo accepted", w.Code, w.Body)
	}
	invalid.Images = []string{f.listing.Image, f.listing.Image}
	body, contentType = managementBody(t, invalid, false)
	if w := f.request("PUT", "/api/listings/managed-home", "publisher", body, contentType, ""); w.Code != 400 {
		t.Fatal("duplicate retained photo accepted")
	}
	invalid.Images = nil
	body, contentType = managementBody(t, invalid, false)
	if w := f.request("PUT", "/api/listings/managed-home", "publisher", body, contentType, ""); w.Code != 400 {
		t.Fatal("empty photo collection accepted")
	}
	body, contentType = managementBody(t, invalid, true)
	w := f.request("PUT", "/api/listings/managed-home", "publisher", body, contentType, "")
	if w.Code != 200 {
		t.Fatalf("replacement failed: %d %s", w.Code, w.Body)
	}
	actual, err := f.server.Store.Find(context.Background(), f.listing.ID)
	if err != nil || len(actual.Images) != 1 || actual.Image == f.listing.Image {
		t.Fatal("photo not replaced", err)
	}
	if _, err = os.Stat(filepath.Join(f.server.Maps.Dir, "runtime", "uploads", filepath.Base(f.listing.Image))); !os.IsNotExist(err) {
		t.Fatal("removed photo not cleaned up")
	}
	if w = f.request("DELETE", "/api/listings/managed-home", "publisher", nil, "", ""); w.Code != 200 {
		t.Fatal("owner deletion failed", w.Code)
	}
}

func TestAdministratorProvisioningAndRevocation(t *testing.T) {
	f := newManagementFixture(t)
	t.Setenv("AUTH_ADMIN_BOOTSTRAP_EMAIL", "900000003@qq.com")
	t.Setenv("AUTH_ADMIN_BOOTSTRAP_PASSWORD", "temporary-initial-password")
	if _, err := f.server.Store.DB.Exec("DELETE FROM accounts WHERE id='administrator'"); err != nil {
		t.Fatal(err)
	}
	if err := f.auth.bootstrapAdministrator(context.Background()); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := f.server.Store.DB.QueryRow("SELECT password_hash FROM accounts WHERE email=?", "900000003@qq.com").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("temporary-initial-password")) != nil {
		t.Fatal("provisioned hash mismatch")
	}
	t.Setenv("AUTH_ADMIN_BOOTSTRAP_PASSWORD", "changed-should-not-overwrite")
	if err := f.auth.bootstrapAdministrator(context.Background()); err != nil {
		t.Fatal(err)
	}
	var unchanged string
	f.server.Store.DB.QueryRow("SELECT password_hash FROM accounts WHERE email=?", "900000003@qq.com").Scan(&unchanged)
	if unchanged != hash {
		t.Fatal("existing password overwritten")
	}
	if f.auth.role("900000002@qq.com") != "user" {
		t.Fatal("stranger became administrator")
	}
	delete(f.auth.adminEmails, "900000003@qq.com")
	if f.auth.role("900000003@qq.com") != "user" {
		t.Fatal("revocation failed")
	}
	if err := f.auth.bootstrapAdministrator(context.Background()); err == nil {
		t.Fatal("bootstrap allowed non-admin identity")
	}
}
