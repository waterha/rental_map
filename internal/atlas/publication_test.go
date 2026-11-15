package atlas

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uploadRequest(t *testing.T, h http.Handler, cookie *http.Cookie, photos [][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	listing, _ := json.Marshal(validListing())
	writer.WriteField("listing", string(listing))
	for _, photo := range photos {
		file, _ := writer.CreateFormFile("photos", "original-with-location.png")
		file.Write(photo)
	}
	writer.Close()
	request := httptest.NewRequest("POST", "http://example.com/api/listings", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://example.com")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	result := httptest.NewRecorder()
	h.ServeHTTP(result, request)
	return result
}

func TestPublicationPhotoValidationAndCleanup(t *testing.T) {
	s, h := testServer(t)
	requireStatus(t, post(h, validListing(), "", nil), 401)
	h, cookie := publicationAccount(t, s)
	requireStatus(t, uploadRequest(t, h, nil, [][]byte{[]byte("bad")}), 401)
	var image bytes.Buffer
	if err := png.Encode(&image, testPhoto()); err != nil {
		t.Fatal(err)
	}
	valid := image.Bytes()
	// Add valid PNG ancillary metadata; published photos must contain pixels only.
	payload := []byte("GPS\x00private-location-metadata")
	var chunk bytes.Buffer
	binary.Write(&chunk, binary.BigEndian, uint32(len(payload)))
	chunk.WriteString("tEXt")
	chunk.Write(payload)
	binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("tEXt"), payload...)))
	valid = append(append(append([]byte{}, valid[:len(valid)-12]...), chunk.Bytes()...), valid[len(valid)-12:]...)
	for _, photos := range [][][]byte{
		nil, {valid, valid, valid, valid, valid, valid}, {[]byte("<svg><script>bad</script></svg>")}, {valid, []byte("bad image")}, {make([]byte, (5<<20)+1)},
	} {
		requireStatus(t, uploadRequest(t, h, cookie, photos), 400)
		files, _ := os.ReadDir(filepath.Join(s.Maps.Dir, "runtime", "uploads"))
		if len(files) != 0 {
			t.Fatal("failed upload left photos behind")
		}
	}
	result := uploadRequest(t, h, cookie, [][]byte{valid, valid, valid, valid, valid})
	requireStatus(t, result, 201)
	var listing Listing
	if err := json.Unmarshal(result.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Images) != 5 {
		t.Fatal("five-photo upload lost images")
	}
	for _, path := range listing.Images {
		photo := get(t, h, path)
		requireStatus(t, photo, 200)
		if photo.Header().Get("Content-Type") != "image/jpeg" || bytes.Contains(photo.Body.Bytes(), payload) {
			t.Fatal("image metadata survived re-encoding")
		}
	}
	if strings.Contains(result.Body.String(), "original-with-location") {
		t.Fatal("original filename leaked")
	}
	// Read through a freshly opened store to check real persistence and backwards-compatible metadata.
	reopened, err := OpenStore(context.Background(), s.Maps.Dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	records, err := reopened.All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record.ID == listing.ID {
			found = true
			if len(record.Images) != 5 || record.Contact != listing.Contact || record.Longitude != 120.11 || record.RentalSource != "房东直租" {
				t.Fatal("metadata lost after reopening database")
			}
		}
	}
	if !found {
		t.Fatal("new listing missing after restart")
	}
	public := get(t, h, "/api/listings/"+listing.ID)
	requireStatus(t, public, 200)
	if strings.Contains(public.Body.String(), "publisher@gmail.com") || strings.Contains(public.Body.String(), `"owner"`) {
		t.Fatal("account identity leaked")
	}
	for _, path := range []string{"/uploads/not-an-image.jpg", "/uploads/.env.server.local", "/uploads/"} {
		if response := get(t, h, path); response.Code == 200 {
			t.Fatalf("unexpected upload path served: %s", path)
		}
	}
	// A failed database insert must also remove already decoded images.
	if _, err := s.Store.DB.Exec("DROP TABLE listing_details"); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, uploadRequest(t, h, cookie, [][]byte{valid}), 500)
	files, _ := os.ReadDir(filepath.Join(s.Maps.Dir, "runtime", "uploads"))
	if len(files) != 5 {
		t.Fatal("failed database save leaked an image")
	}
}

func TestDistrictPositionRespectsHolesAndMultipolygons(t *testing.T) {
	s, _ := testServer(t)
	os.MkdirAll(filepath.Join(s.Maps.Dir, "maps"), 0700)
	data := `{"features":[{"properties":{"adcode":330106},"geometry":{"type":"MultiPolygon","coordinates":[[[[120,30],[121,30],[121,31],[120,31],[120,30]],[[120.2,30.2],[120.4,30.2],[120.4,30.4],[120.2,30.4],[120.2,30.2]]],[[[122,30],[123,30],[123,31],[122,31],[122,30]]]]}}]}`
	if err := os.WriteFile(filepath.Join(s.Maps.Dir, "maps", "330100.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	district, _ := s.Maps.Get("330106")
	for _, check := range []struct {
		point  [2]float64
		inside bool
	}{
		{[2]float64{120.1, 30.1}, true}, {[2]float64{120.3, 30.3}, false}, {[2]float64{122.5, 30.5}, true}, {[2]float64{121.5, 30.5}, false}, {[2]float64{120, 30.5}, true},
	} {
		inside, err := s.locationInDistrict(context.Background(), district, check.point)
		if err != nil || inside != check.inside {
			t.Errorf("%v inside=%v err=%v", check.point, inside, err)
		}
	}
}
