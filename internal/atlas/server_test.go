package atlas

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	regions := []Region{
		{Code: "100000", Name: "全国", Level: "country", HasChildren: true},
		{Code: "330000", Name: "浙江省", Level: "province", Parent: "100000", HasChildren: true},
		{Code: "330100", Name: "杭州市", Level: "city", Parent: "330000", HasChildren: true},
		{Code: "330106", Name: "西湖区", Level: "district", Parent: "330100", Center: [2]float64{120.1, 30.2}},
		{Code: "310000", Name: "上海市", Level: "province", Parent: "100000", HasChildren: true},
		{Code: "310104", Name: "徐汇区", Level: "district", Parent: "310000", Center: [2]float64{121.4, 31.1}},
	}
	data, _ := json.Marshal(regions)
	if err := os.WriteFile(filepath.Join(dir, "regions.json"), data, 0600); err != nil {
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
	s := &Server{Store: store, Maps: maps, StaticDir: t.TempDir(), AllowPublish: true}
	for _, l := range []Listing{
		{ID: "a", Title: "西湖阳光两居", Description: "示例描述", Type: "整租", Price: 3000, Area: 60, Rooms: 2, ProvinceCode: "330000", CityCode: "330100", DistrictCode: "330106", City: "杭州市", District: "西湖区", Community: "西湖花园", Tags: []string{"近地铁"}, IsDemo: true, CreatedAt: "2026-11-01T00:00:00Z", Owner: "demo"},
		{ID: "b", Title: "西湖温馨合租", Description: "示例描述", Type: "合租", Price: 1500, Area: 20, Rooms: 1, ProvinceCode: "330000", CityCode: "330100", DistrictCode: "330106", City: "杭州市", District: "西湖区", Community: "西湖花园", Tags: []string{"朝南"}, IsDemo: true, CreatedAt: "2026-11-02T00:00:00Z", Owner: "demo"},
		{ID: "c", Title: "徐汇精装两居", Description: "示例描述", Type: "转租", Price: 5000, Area: 70, Rooms: 2, ProvinceCode: "310000", CityCode: "310000", DistrictCode: "310104", City: "上海市", District: "徐汇区", Tags: []string{"近地铁"}, IsDemo: true, CreatedAt: "2026-11-03T00:00:00Z", Owner: "demo"},
	} {
		if err := store.Insert(context.Background(), l); err != nil {
			t.Fatal(err)
		}
	}
	return s, s.Handler()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
	return r
}

func TestFiltersAndAggregates(t *testing.T) {
	_, h := testServer(t)
	for _, test := range []struct {
		path  string
		total int
		id    string
	}{
		{"/api/listings", 3, "c"},
		{"/api/listings?region=330000", 2, "b"},
		{"/api/listings?region=330106&type=整租&minPrice=2500&maxPrice=3500&rooms=2&tag=近地铁", 1, "a"},
		{"/api/listings?ids=b,c&region=310000", 1, "c"},
		{"/api/listings?ids=", 0, ""},
		{"/api/listings?q=西湖&sort=price-asc", 2, "b"},
		{"/api/listings?mine=true", 0, ""},
	} {
		t.Run(test.path, func(t *testing.T) {
			res := get(t, h, test.path)
			if res.Code != 200 {
				t.Fatalf("status %d: %s", res.Code, res.Body.String())
			}
			var body struct {
				Items   []Listing     `json:"items"`
				Total   int           `json:"total"`
				Regions []RegionCount `json:"regions"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Total != test.total {
				t.Fatalf("total=%d want=%d", body.Total, test.total)
			}
			if test.id != "" && body.Items[0].ID != test.id {
				t.Fatalf("first=%s want=%s", body.Items[0].ID, test.id)
			}
			for _, r := range body.Regions {
				if r.Code == "310000" && r.Count != 1 {
					t.Fatalf("municipality counted twice: %d", r.Count)
				}
			}
			if bytes.Contains(res.Body.Bytes(), []byte(`"owner"`)) {
				t.Fatal("private session leaked")
			}
		})
	}
}

func TestBadRequestsAndMissingResources(t *testing.T) {
	_, h := testServer(t)
	for _, path := range []string{"/api/listings?minPrice=-1", "/api/listings?minPrice=no", "/api/listings?minPrice=500&maxPrice=100", "/api/listings?page=-1", "/api/listings?sort=evil", "/api/listings?region=999999", "/api/listings?type=unknown", "/api/maps/not-a-code"} {
		if res := get(t, h, path); res.Code != 400 {
			t.Errorf("%s: got=%d want=400", path, res.Code)
		}
	}
	for _, path := range []string{"/api/listings/missing", "/api/unknown", "/api/maps/999999"} {
		if res := get(t, h, path); res.Code != 404 {
			t.Errorf("%s got=%d want=404", path, res.Code)
		}
	}
	if res := get(t, h, "/api/maps/330106"); res.Code != 422 {
		t.Errorf("leaf map got=%d", res.Code)
	}
}

func validListing() Listing {
	return Listing{ListingDetails: ListingDetails{RentalSource: "房东直租", PropertyType: "住宅", Bathrooms: 1}, Longitude: 120.11, Latitude: 30.21, Title: "地铁旁南向精装两居室", Description: "采光通透，交通便利，周边生活配套完善，随时可以看房。", Type: "整租", Price: 3200, Area: 60, Rooms: 2, DistrictCode: "330106", Contact: "test@example.com", Tags: []string{"近地铁"}}
}
func testPhoto() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 50, G: 100, B: 150, A: 255})
	return img
}
func publicationAccount(t *testing.T, s *Server) (http.Handler, *http.Cookie) {
	t.Helper()
	auth, err := NewAuth(context.Background(), s.Store, s.Maps.Dir)
	if err != nil {
		t.Fatal(err)
	}
	mail := &capturedMail{codes: map[string]string{}}
	auth.Mailer = mail
	s.Auth = auth
	h := s.Handler()
	requireStatus(t, authPost(h, "/api/auth/code", codeBody("publisher@gmail.com")), 200)
	registration := authPost(h, "/api/auth/register", registerBody("publisher@gmail.com", mail.code("publisher@gmail.com")))
	requireStatus(t, registration, 201)
	os.MkdirAll(filepath.Join(s.Maps.Dir, "maps"), 0700)
	boundary := `{"type":"FeatureCollection","features":[{"properties":{"adcode":330106},"geometry":{"type":"Polygon","coordinates":[[[120,30],[120.4,30],[120.4,30.4],[120,30.4],[120,30]]]}}]}`
	if err := os.WriteFile(filepath.Join(s.Maps.Dir, "maps", "330100.json"), []byte(boundary), 0600); err != nil {
		t.Fatal(err)
	}
	return h, registration.Result().Cookies()[0]
}

func post(h http.Handler, l Listing, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	data, _ := json.Marshal(l)
	_ = writer.WriteField("listing", string(data))
	photo, _ := writer.CreateFormFile("photos", "private-original-name.png")
	_ = png.Encode(photo, testPhoto())
	writer.Close()
	r := httptest.NewRequest("POST", "http://example.com/api/listings", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, r)
	return res
}

func TestPublishSessionAndPersistence(t *testing.T) {
	s, _ := testServer(t)
	h, cookie := publicationAccount(t, s)
	res := post(h, validListing(), "http://example.com", cookie)
	if res.Code != 201 {
		t.Fatalf("publish: %d %s", res.Code, res.Body.String())
	}
	var l Listing
	if err := json.Unmarshal(res.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	if l.IsDemo || l.ProvinceCode != "330000" || l.CityCode != "330100" || l.City != "杭州市" || l.ID == "" {
		t.Fatalf("bad derived listing: %+v", l)
	}
	if l.Longitude != 120.11 || l.Latitude != 30.21 || len(l.Images) != 1 || l.Bathrooms != 1 || l.RentalSource != "房东直租" {
		t.Fatal("selected location or publication details lost")
	}
	photo := get(t, h, l.Images[0])
	if photo.Code != 200 || photo.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal("uploaded photo not publicly served")
	}
	req := httptest.NewRequest("GET", "/api/listings?mine=true", nil)
	req.AddCookie(cookie)
	mine := httptest.NewRecorder()
	h.ServeHTTP(mine, req)
	var payload struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(mine.Body.Bytes(), &payload)
	if payload.Total != 1 {
		t.Fatalf("mine=%d", payload.Total)
	}
	stranger := get(t, h, "/api/listings?mine=true")
	_ = json.Unmarshal(stranger.Body.Bytes(), &payload)
	if payload.Total != 0 {
		t.Fatal("another browser can access local ownership")
	}
	if get(t, h, "/api/listings/"+l.ID).Code != 200 {
		t.Fatal("listing was not persisted")
	}
	var count int
	if err := s.Store.DB.QueryRow("SELECT COUNT(*) FROM listings WHERE id=?", l.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("publication not in database")
	}
}

func TestPublicationValidationAndOrigin(t *testing.T) {
	s, _ := testServer(t)
	h, cookie := publicationAccount(t, s)
	if res := post(h, validListing(), "https://attacker.example", cookie); res.Code != 403 {
		t.Fatalf("cross-origin post allowed: %d", res.Code)
	}
	for _, test := range []struct {
		name   string
		change func(*Listing)
	}{
		{"outside district", func(l *Listing) { l.Longitude = 121 }}, {"missing source", func(l *Listing) { l.RentalSource = "" }}, {"bathrooms", func(l *Listing) { l.Bathrooms = 0 }}, {"short title", func(l *Listing) { l.Title = "短" }}, {"invalid district", func(l *Listing) { l.DistrictCode = "330000" }}, {"negative price", func(l *Listing) { l.Price = -1 }}, {"invalid type", func(l *Listing) { l.Type = "其他" }}, {"short contact", func(l *Listing) { l.Contact = "a" }}, {"short description", func(l *Listing) { l.Description = "短" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			l := validListing()
			test.change(&l)
			if res := post(h, l, "", cookie); res.Code != 400 {
				t.Fatalf("got %d: %s", res.Code, res.Body.String())
			}
		})
	}
	s.AllowPublish = false
	if res := post(h, validListing(), "", cookie); res.Code != 403 {
		t.Fatalf("publishing switch ignored: %d", res.Code)
	}
}

func TestSQLBinding(t *testing.T) {
	s := &Store{Postgres: true}
	if result := s.bind("a=? AND b=? AND c=?"); result != "a=$1 AND b=$2 AND c=$3" {
		t.Fatal(result)
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (fn transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestDownloadedDistrictIndexSurvivesRestart(t *testing.T) {
	s, _ := testServer(t)
	s.Maps.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "geo.datav.aliyun.com" || r.URL.Path != "/areas_v3/bound/330100_full.json" {
			t.Fatalf("unexpected upstream: %s", r.URL)
		}
		body := `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"adcode":330102,"name":"上城区","level":"district","center":[120.2,30.25],"childrenNum":0},"geometry":{"type":"Polygon","coordinates":[[[120,30],[121,30],[121,31],[120,30]]]}}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	if err := os.MkdirAll(filepath.Join(s.Maps.Dir, "maps"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Maps.Read(context.Background(), "330100"); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewMaps(s.Maps.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if district, exists := restarted.Get("330102"); !exists || district.Parent != "330100" {
		t.Fatal("new district index was lost after restart")
	}
	if _, err := os.Stat(filepath.Join(s.Maps.Dir, "maps", "330100.json")); err != nil {
		t.Fatal("downloaded map was not cached")
	}
}
