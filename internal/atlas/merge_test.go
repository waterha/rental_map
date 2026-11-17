package atlas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mergeTestHandler(t *testing.T) http.Handler {
	t.Helper()
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
	for _, item := range []struct{ id, district, kind string }{
		{"west", "330106", "整租"}, {"north", "330110", "合租"}, {"outside", "330102", "整租"}, {"other-city", "330782", "整租"},
	} {
		district, _ := maps.Get(item.district)
		listing := Listing{ID: item.id, Title: "合区验证房源", Type: item.kind, Price: 2000, Area: 30, Rooms: 1, ProvinceCode: "330000", CityCode: district.Parent, DistrictCode: item.district,
			City: "测试城市", District: district.Name, Tags: []string{}, CreatedAt: "2026-11-17T00:00:00Z", Contact: "private-contact", Owner: "private-owner"}
		if err := store.Insert(context.Background(), listing); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{Maps: maps, Store: store, StaticDir: t.TempDir()}
	return s.Handler()
}

func TestMergedListingsUseDistrictUnionAndFilters(t *testing.T) {
	h := mergeTestHandler(t)
	for _, item := range []struct {
		query string
		ids   []string
	}{
		{"districts=330106,330110", []string{"north", "west"}},
		{"districts=330106,330110&type=整租", []string{"west"}},
		{"districts=330106,330110&minPrice=3000", []string{}},
		{"region=330106", []string{"west"}},
	} {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest("GET", "/api/listings?"+item.query, nil))
		if res.Code != 200 {
			t.Fatalf("query %s returned %d: %s", item.query, res.Code, res.Body)
		}
		var result SearchResults
		if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Total != len(item.ids) || len(result.Markers) != len(item.ids) || len(result.Items) != len(item.ids) {
			t.Fatalf("wrong union count for %s: %d", item.query, result.Total)
		}
		for _, marker := range result.Markers {
			found := false
			for _, id := range item.ids {
				found = found || marker.ID == id
			}
			if !found {
				t.Fatal("unselected district returned", marker.ID)
			}
		}
		if strings.Contains(res.Body.String(), "private-contact") || strings.Contains(res.Body.String(), "private-owner") {
			t.Fatal("merged list exposed private fields")
		}
	}
}

func TestMergedListingsRejectInvalidScopes(t *testing.T) {
	h := mergeTestHandler(t)
	for _, query := range []string{
		"districts=", "districts=330106", "districts=330106,330106", "districts=330106,330782",
		"districts=330106,999999", "districts=330106,330100", "districts=330106,330110&districts=330102,330108",
		"districts=330106,%27%20OR%201%3D1--", "districts=" + strings.Repeat("330106,", 16) + "330110",
	} {
		res := httptest.NewRecorder()
		h.ServeHTTP(res, httptest.NewRequest("GET", "/api/listings?"+query, nil))
		if res.Code != 400 {
			t.Fatalf("unsafe scope accepted: %s (%d)", query, res.Code)
		}
	}
}
