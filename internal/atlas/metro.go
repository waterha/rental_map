package atlas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const metroBaseURL = "https://webapi.amap.com/subway/data/"

var metroCityCode = regexp.MustCompile(`^\d{4}$`)
var metroCitySpell = regexp.MustCompile(`^[a-z]+$`)
var metroColor = regexp.MustCompile(`^#[a-fA-F0-9]{6}$`)

type MetroStation struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Position [2]float64 `json:"position"`
	Transfer bool       `json:"transfer"`
}
type MetroLine struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Color    string         `json:"color"`
	Loop     bool           `json:"loop"`
	Stations []MetroStation `json:"stations"`
}
type MetroNetwork struct {
	CityCode  string      `json:"cityCode"`
	CityName  string      `json:"cityName"`
	Source    string      `json:"source"`
	SourceURL string      `json:"sourceUrl,omitempty"`
	FetchedAt time.Time   `json:"fetchedAt"`
	Schematic bool        `json:"schematic"`
	Stale     bool        `json:"stale"`
	Lines     []MetroLine `json:"lines"`
}
type metroCity struct {
	Spell string `json:"spell"`
	Code  string `json:"adcode"`
	Name  string `json:"cityname"`
}
type metroEntry struct {
	data  MetroNetwork
	until time.Time
}
type Metro struct {
	dir    string
	cities map[string]metroCity
	client *http.Client
	mu     sync.Mutex
	cache  map[string]metroEntry
	busy   map[string]chan struct{}
}

func NewMetro(dir string) (*Metro, error) {
	data, err := os.ReadFile(filepath.Join(dir, "metro", "cities.json"))
	if err != nil {
		return nil, err
	}
	var cities []metroCity
	if err := json.Unmarshal(data, &cities); err != nil {
		return nil, err
	}
	m := &Metro{dir: dir, cities: map[string]metroCity{}, client: &http.Client{Timeout: 12 * time.Second}, cache: map[string]metroEntry{}, busy: map[string]chan struct{}{}}
	for _, city := range cities {
		if metroCityCode.MatchString(city.Code) && metroCitySpell.MatchString(city.Spell) {
			m.cities[city.Code+"00"] = city
		}
	}
	return m, nil
}

func (m *Metro) Read(ctx context.Context, code string) (MetroNetwork, error) {
	city, ok := m.cities[code]
	if !ok {
		return MetroNetwork{CityCode: code, Source: "高德地铁", Schematic: true, Lines: []MetroLine{}}, nil
	}
	for {
		m.mu.Lock()
		if entry, ok := m.cache[code]; ok && time.Now().Before(entry.until) {
			m.mu.Unlock()
			return entry.data, nil
		}
		if done, ok := m.busy[code]; ok {
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return MetroNetwork{}, ctx.Err()
			case <-done:
				continue
			}
		}
		done := make(chan struct{})
		m.busy[code] = done
		m.mu.Unlock()
		data, err := m.load(ctx, code, city)
		m.mu.Lock()
		if err == nil {
			ttl := min(24*time.Hour, time.Until(data.FetchedAt.Add(24*time.Hour)))
			if data.Stale {
				ttl = 5 * time.Minute
			}
			m.cache[code] = metroEntry{data: data, until: time.Now().Add(ttl)}
		}
		delete(m.busy, code)
		close(done)
		m.mu.Unlock()
		return data, err
	}
}

func validMetro(data MetroNetwork, code string) bool {
	if data.CityCode != code || data.FetchedAt.IsZero() || len(data.Lines) == 0 || !data.Schematic {
		return false
	}
	for _, line := range data.Lines {
		if line.ID == "" || line.Name == "" || !metroColor.MatchString(line.Color) || len(line.Stations) < 2 {
			return false
		}
		for _, station := range line.Stations {
			p := station.Position
			if station.Name == "" || math.IsNaN(p[0]) || math.IsNaN(p[1]) || math.IsInf(p[0], 0) || math.IsInf(p[1], 0) || p[0] < 70 || p[0] > 140 || p[1] < 0 || p[1] > 60 {
				return false
			}
		}
	}
	return true
}

func (m *Metro) load(ctx context.Context, code string, city metroCity) (MetroNetwork, error) {
	var fallback *MetroNetwork
	for _, file := range []string{filepath.Join(m.dir, "runtime", "metro", code+".json"), filepath.Join(m.dir, "metro", code+".json")} {
		var data MetroNetwork
		if raw, err := os.ReadFile(file); err == nil && json.Unmarshal(raw, &data) == nil && validMetro(data, code) {
			if fallback == nil || data.FetchedAt.After(fallback.FetchedAt) {
				copy := data
				fallback = &copy
			}
		}
	}
	if fallback != nil && time.Since(fallback.FetchedAt) < 24*time.Hour {
		return *fallback, nil
	}
	data, err := m.fetch(ctx, code, city)
	if err != nil {
		if fallback != nil {
			fallback.Stale = true
			return *fallback, nil
		}
		return MetroNetwork{}, err
	}
	cacheDir := filepath.Join(m.dir, "runtime", "metro")
	if err := os.MkdirAll(cacheDir, 0700); err == nil {
		if raw, err := json.Marshal(data); err == nil {
			_ = atomicWrite(filepath.Join(cacheDir, code+".json"), raw)
		}
	}
	return data, nil
}

func (m *Metro) fetch(ctx context.Context, code string, city metroCity) (MetroNetwork, error) {
	url := metroBaseURL + city.Code + "_drw_" + city.Spell + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return MetroNetwork{}, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return MetroNetwork{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return MetroNetwork{}, fmt.Errorf("metro data status %d", res.StatusCode)
	}
	var source struct {
		Lines []struct {
			ID       string `json:"ls"`
			Name     string `json:"ln"`
			Branch   string `json:"la"`
			Color    string `json:"cl"`
			Active   string `json:"su"`
			Loop     string `json:"lo"`
			Stations []struct {
				ID       string `json:"si"`
				Name     string `json:"n"`
				Position string `json:"sl"`
				Transfer string `json:"t"`
			} `json:"st"`
		} `json:"l"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&source); err != nil {
		return MetroNetwork{}, err
	}
	data := MetroNetwork{CityCode: code, CityName: city.Name, Source: "高德地铁", SourceURL: url, FetchedAt: time.Now().UTC(), Schematic: true, Lines: []MetroLine{}}
	for _, item := range source.Lines {
		if item.Active != "1" {
			continue
		}
		line := MetroLine{ID: item.ID, Name: item.Name, Color: "#" + item.Color, Loop: item.Loop == "1", Stations: []MetroStation{}}
		if item.Branch != "" {
			line.Name += "（" + item.Branch + "）"
		}
		if !metroColor.MatchString(line.Color) {
			line.Color = "#4cddff"
		}
		for _, station := range item.Stations {
			parts := strings.Split(station.Position, ",")
			if len(parts) != 2 {
				return MetroNetwork{}, errors.New("invalid metro station position")
			}
			lng, err1 := strconv.ParseFloat(parts[0], 64)
			lat, err2 := strconv.ParseFloat(parts[1], 64)
			if err1 != nil || err2 != nil {
				return MetroNetwork{}, errors.New("invalid metro coordinates")
			}
			line.Stations = append(line.Stations, MetroStation{ID: station.ID, Name: station.Name, Position: [2]float64{lng, lat}, Transfer: station.Transfer == "1"})
		}
		if len(line.Stations) >= 2 {
			data.Lines = append(data.Lines, line)
		}
	}
	if !validMetro(data, code) {
		return MetroNetwork{}, errors.New("invalid metro network")
	}
	return data, nil
}

func metroScope(region Region) bool {
	return region.Level == "city" || region.Level == "province" && (region.Code == "110000" || region.Code == "120000" || region.Code == "310000" || region.Code == "500000" || region.Code == "810000" || region.Code == "820000")
}

func (s *Server) metro(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !validCode.MatchString(code) {
		fail(w, 400, "无效城市代码")
		return
	}
	region, ok := s.Maps.Get(code)
	if !ok {
		fail(w, 404, "未找到该城市")
		return
	}
	if region.Level != "district" {
		fail(w, 422, "请进入区县地图查看地铁")
		return
	}
	city, exists := s.Maps.Get(region.Parent)
	if !exists || !metroScope(city) {
		fail(w, 422, "未找到该区县所属城市")
		return
	}
	if s.Metro == nil {
		fail(w, 503, "地铁图层暂不可用")
		return
	}
	data, err := s.Metro.Read(r.Context(), city.Code)
	if err != nil {
		fail(w, 502, "地铁线路暂时加载失败，请重试")
		return
	}
	if data.CityName == "" {
		data.CityName = city.Name
	}
	send(w, 200, data)
}
