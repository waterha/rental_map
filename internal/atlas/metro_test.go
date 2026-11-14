package atlas

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type metroTransport func(*http.Request) (*http.Response, error)

func (f metroTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testMetro(t *testing.T) *Metro {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "metro"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metro", "cities.json"), []byte(`[{"spell":"hangzhou","adcode":"3301","cityname":"杭州市"},{"spell":"beijing","adcode":"1100","cityname":"北京市"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewMetro(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

const metroFixture = `{"l":[{"ls":"line-1","ln":"1号线","cl":"FA2020","su":"1","lo":"0","st":[{"si":"s1","n":"西湖文化广场","sl":"120.163859,30.279887","t":"1"},{"si":"s2","n":"武林广场","sl":"120.163221,30.272975","t":"0"}]},{"ls":"future","su":"0"}]}`

func TestMetroFetchCoalescesAndCaches(t *testing.T) {
	m := testMetro(t)
	var calls atomic.Int32
	m.client.Transport = metroTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.String() != metroBaseURL+"3301_drw_hangzhou.json" || r.URL.RawQuery != "" {
			t.Error("unexpected source or credentials")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(metroFixture))}, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := m.Read(context.Background(), "330100")
			if err != nil {
				t.Error(err)
				return
			}
			if len(data.Lines) != 1 || data.Lines[0].Color != "#FA2020" || data.Lines[0].Stations[0].Position != [2]float64{120.163859, 30.279887} || !data.Schematic {
				t.Error("invalid converted network")
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("got %d upstream calls", calls.Load())
	}
	if _, err := os.Stat(filepath.Join(m.dir, "runtime", "metro", "330100.json")); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewMetro(m.dir)
	if err != nil {
		t.Fatal(err)
	}
	restarted.client.Transport = metroTransport(func(*http.Request) (*http.Response, error) {
		t.Error("disk cache should avoid network")
		return nil, errors.New("offline")
	})
	if _, err := restarted.Read(context.Background(), "330100"); err != nil {
		t.Fatal(err)
	}
}

func TestMetroOfflineFallbackAndInvalidUpstream(t *testing.T) {
	m := testMetro(t)
	for _, body := range []string{`{}`, strings.ReplaceAll(metroFixture, "120.163859,30.279887", "NaN,30"), strings.ReplaceAll(metroFixture, "120.163859,30.279887", "bad")} {
		m.client.Transport = metroTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		if _, err := m.Read(context.Background(), "330100"); err == nil {
			t.Fatal("accepted malformed source")
		}
	}
	m.client.Transport = metroTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(metroFixture))}, nil
	})
	data, err := m.Read(context.Background(), "330100")
	if err != nil {
		t.Fatal(err)
	}
	data.FetchedAt = time.Now().Add(-48 * time.Hour)
	raw, _ := json.Marshal(data)
	if err := os.WriteFile(filepath.Join(m.dir, "runtime", "metro", "330100.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	m.cache = map[string]metroEntry{}
	m.client.Transport = metroTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	fallback, err := m.Read(context.Background(), "330100")
	if err != nil || !fallback.Stale || len(fallback.Lines) != 1 {
		t.Fatal("offline fallback failed")
	}
}

func TestMetroEndpointScopesAndNoCoverage(t *testing.T) {
	s, _ := testServer(t)
	s.Metro = testMetro(t)
	delete(s.Metro.cities, "330100")
	delete(s.Metro.cities, "310000")
	h := s.Handler()
	for _, item := range []struct {
		path   string
		status int
	}{{"/api/metro/no", 400}, {"/api/metro/999999", 404}, {"/api/metro/100000", 422}, {"/api/metro/330000", 422}, {"/api/metro/330106", 200}, {"/api/metro/330100", 422}, {"/api/metro/310000", 422}, {"/api/metro/310104", 200}} {
		res := get(t, h, item.path)
		if res.Code != item.status {
			t.Errorf("%s: got %d expected %d", item.path, res.Code, item.status)
		}
		if item.status == 200 {
			var data MetroNetwork
			if json.Unmarshal(res.Body.Bytes(), &data) != nil || data.Lines == nil || len(data.Lines) != 0 {
				t.Fatal("no coverage must be an explicit empty list")
			}
		}
	}
}
