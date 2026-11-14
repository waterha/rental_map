package atlas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

type Region struct {
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Level       string     `json:"level"`
	Parent      string     `json:"parent"`
	Center      [2]float64 `json:"center"`
	HasChildren bool       `json:"hasChildren"`
}

type Maps struct {
	Dir     string
	mu      sync.RWMutex
	regions map[string]Region
	client  *http.Client
}

var validCode = regexp.MustCompile(`^\d{6}$`)

func NewMaps(directory string) (*Maps, error) {
	data, err := os.ReadFile(filepath.Join(directory, "regions.json"))
	if err != nil {
		return nil, fmt.Errorf("地图目录不存在，请先执行 node scripts/download-data.mjs: %w", err)
	}
	var regions []Region
	if err = json.Unmarshal(data, &regions); err != nil {
		return nil, err
	}
	m := &Maps{Dir: directory, regions: map[string]Region{}, client: &http.Client{Timeout: 15 * time.Second}}
	m.regions["100000"] = Region{Code: "100000", Name: "全国", Level: "country", Center: [2]float64{104, 35}, HasChildren: true}
	for _, r := range regions {
		m.regions[r.Code] = r
	}
	return m, nil
}

func (m *Maps) Get(code string) (Region, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.regions[code]
	return r, ok
}
func (m *Maps) List(query string) []Region {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := []Region{}
	for _, r := range m.regions {
		if query == "" || containsFold(r.Name, query) || r.Code == query {
			result = append(result, r)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Code < result[j].Code })
	return result
}

func (m *Maps) Read(ctx context.Context, code string) ([]byte, error) {
	if !validCode.MatchString(code) {
		return nil, errors.New("无效行政区代码")
	}
	r, ok := m.Get(code)
	if !ok {
		return nil, errors.New("未找到该行政区")
	}
	if !r.HasChildren {
		return nil, errors.New("已到达区县层级")
	}
	file := filepath.Join(m.Dir, "maps", code+".json")
	if data, err := os.ReadFile(file); err == nil {
		return data, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://geo.datav.aliyun.com/areas_v3/bound/"+code+"_full.json", nil)
	if err != nil {
		return nil, err
	}
	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("地图服务返回 %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if err != nil {
		return nil, err
	}
	var fc struct {
		Type     string `json:"type"`
		Features []struct {
			Properties struct {
				Code     json.Number `json:"adcode"`
				Name     string      `json:"name"`
				Level    string      `json:"level"`
				Center   [2]float64  `json:"center"`
				Children int         `json:"childrenNum"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err = json.Unmarshal(data, &fc); err != nil || fc.Type != "FeatureCollection" || len(fc.Features) == 0 {
		return nil, errors.New("地图数据格式错误")
	}
	m.mu.Lock()
	for _, f := range fc.Features {
		p := f.Properties
		c := string(p.Code)
		if validCode.MatchString(c) {
			m.regions[c] = Region{Code: c, Name: p.Name, Level: p.Level, Parent: code, Center: p.Center, HasChildren: p.Children > 0}
		}
	}
	// Persist newly discovered districts so search, published listings and deep links survive restarts.
	indexed := make([]Region, 0, len(m.regions))
	for _, item := range m.regions {
		indexed = append(indexed, item)
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].Code < indexed[j].Code })
	if encoded, encodeErr := json.Marshal(indexed); encodeErr == nil {
		if persistErr := atomicWrite(filepath.Join(m.Dir, "regions.json"), encoded); persistErr != nil {
			log.Printf("persist region index: %v", persistErr)
		}
	}
	m.mu.Unlock()
	// A unique temporary file and atomic rename prevent concurrent requests from seeing partial GeoJSON.
	temp, err := os.CreateTemp(filepath.Dir(file), code+"-*.tmp")
	if err == nil {
		tempName := temp.Name()
		if _, err = temp.Write(data); err == nil {
			err = temp.Close()
			if err == nil {
				_ = os.Rename(tempName, file)
			}
		} else {
			_ = temp.Close()
		}
		_ = os.Remove(tempName)
	}
	return data, nil
}

func atomicWrite(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), "regions-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func codeNumber(code string) int { n, _ := strconv.Atoi(code); return n }
