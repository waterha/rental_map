package atlas

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Server struct {
	Store        *Store
	Maps         *Maps
	StaticDir    string
	AllowPublish bool
}
type RegionCount struct {
	Code    string `json:"code"`
	Count   int    `json:"count"`
	Average int    `json:"average"`
	Name    string `json:"name"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.DB.PingContext(r.Context()); err != nil {
			fail(w, 503, "数据库暂不可用")
			return
		}
		send(w, 200, map[string]any{"status": "ok", "database": map[bool]string{true: "postgresql", false: "sqlite"}[s.Store.Postgres], "publishing": s.AllowPublish})
	})
	mux.HandleFunc("GET /api/regions", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, s.Maps.List(strings.TrimSpace(r.URL.Query().Get("q"))))
	})
	mux.HandleFunc("GET /api/maps/{code}", s.mapData)
	mux.HandleFunc("GET /api/listings", s.listings)
	mux.HandleFunc("GET /api/listings/{id}", s.detail)
	mux.HandleFunc("POST /api/listings", s.create)
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		_, err := session(w, r)
		if err != nil {
			fail(w, 500, "无法创建本地会话")
			return
		}
		send(w, 200, map[string]bool{"publishing": s.AllowPublish})
	})
	mux.Handle("GET /images/", http.StripPrefix("/images/", http.FileServer(http.Dir(filepath.Join(s.Maps.Dir, "images")))))
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "接口不存在") })
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(s.StaticDir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		if filepath.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(filepath.Join(s.StaticDir, "index.html")); err != nil {
			http.Error(w, "请先执行 npm run build，或访问前端开发服务 http://127.0.0.1:5173", 503)
			return
		}
		http.ServeFile(w, r, filepath.Join(s.StaticDir, "index.html"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		defer func() {
			if p := recover(); p != nil {
				log.Printf("request panic: %v", p)
				fail(w, 500, "服务暂时不可用")
			}
		}()
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) mapData(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !validCode.MatchString(code) {
		fail(w, 400, "无效行政区代码")
		return
	}
	region, ok := s.Maps.Get(code)
	if !ok {
		fail(w, 404, "行政区不存在")
		return
	}
	if !region.HasChildren {
		fail(w, 422, "已到达区县层级，请查看该区房源")
		return
	}
	data, err := s.Maps.Read(r.Context(), code)
	if err != nil {
		log.Printf("map %s: %v", code, err)
		fail(w, 502, "该区域的边界数据暂不可用，请重试或查看该区域房源")
		return
	}
	w.Header().Set("Content-Type", "application/geo+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

func parseBound(q url.Values, key string) (int, error) {
	v := q.Get(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 1000000 {
		return 0, fmt.Errorf("%s 参数无效", key)
	}
	return n, nil
}

func (s *Server) listings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	min, err := parseBound(q, "minPrice")
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	max, err := parseBound(q, "maxPrice")
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if max > 0 && min > max {
		fail(w, 400, "最低预算不能高于最高预算")
		return
	}
	region := q.Get("region")
	if region != "" && region != "100000" {
		if _, ok := s.Maps.Get(region); !ok {
			fail(w, 400, "行政区不存在")
			return
		}
	}
	kind := q.Get("type")
	if kind != "" && kind != "整租" && kind != "合租" && kind != "转租" {
		fail(w, 400, "租赁类型无效")
		return
	}
	rooms, err := parseBound(q, "rooms")
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	page := 1
	if q.Get("page") != "" {
		page, err = strconv.Atoi(q.Get("page"))
		if err != nil || page < 1 || page > 100000 {
			fail(w, 400, "页码无效")
			return
		}
	}
	keyword := strings.TrimSpace(q.Get("q"))
	if utf8.RuneCountInString(keyword) > 100 {
		fail(w, 400, "关键词过长")
		return
	}
	records, err := s.Store.All(r.Context())
	if err != nil {
		log.Printf("listings: %v", err)
		fail(w, 500, "房源加载失败")
		return
	}
	idSet := map[string]bool{}
	if raw, ok := q["ids"]; ok {
		for _, id := range strings.Split(strings.Join(raw, ","), ",") {
			idSet[id] = true
		}
	}
	owner := ""
	if cookie, e := r.Cookie("atlas_session"); e == nil {
		owner = cookie.Value
	}
	selected := []Listing{}
	counts := map[string]*RegionCount{}
	cities := map[string]*RegionCount{}
	totalPrice := 0
	types := map[string]int{"整租": 0, "合租": 0, "转租": 0}
	prices := []int{}
	for _, l := range records {
		if kind != "" && l.Type != kind || l.Price < min || max > 0 && l.Price > max || rooms > 0 && (rooms < 3 && l.Rooms != rooms || rooms >= 3 && l.Rooms < 3) {
			continue
		}
		if keyword != "" && !containsFold(l.Title+" "+l.Community+" "+l.City+" "+l.District+" "+strings.Join(l.Tags, " "), keyword) {
			continue
		}
		if tag := q.Get("tag"); tag != "" && !containsFold(strings.Join(l.Tags, ","), tag) {
			continue
		}
		if _, ok := q["ids"]; ok && !idSet[l.ID] {
			continue
		}
		if q.Get("mine") == "true" && (owner == "" || l.Owner != owner) {
			continue
		}
		if c := cities[l.CityCode]; c != nil {
			c.Count++
			c.Average += l.Price
		} else {
			cities[l.CityCode] = &RegionCount{Code: l.CityCode, Name: l.City, Count: 1, Average: l.Price}
		}
		if region != "" && region != "100000" && l.ProvinceCode != region && l.CityCode != region && l.DistrictCode != region {
			continue
		}
		selected = append(selected, l)
		totalPrice += l.Price
		prices = append(prices, l.Price)
		types[l.Type]++
		seenCodes := map[string]bool{}
		for _, code := range []string{l.ProvinceCode, l.CityCode, l.DistrictCode} {
			if seenCodes[code] {
				continue
			}
			seenCodes[code] = true
			if c := counts[code]; c != nil {
				c.Count++
				c.Average += l.Price
			} else {
				r, _ := s.Maps.Get(code)
				counts[code] = &RegionCount{Code: code, Name: r.Name, Count: 1, Average: l.Price}
			}
		}
	}
	sortMode := q.Get("sort")
	if sortMode != "" && sortMode != "newest" && sortMode != "price-asc" && sortMode != "price-desc" && sortMode != "area-desc" {
		fail(w, 400, "排序方式无效")
		return
	}
	sort.SliceStable(selected, func(i, j int) bool {
		a, b := selected[i], selected[j]
		switch sortMode {
		case "price-asc":
			return a.Price < b.Price
		case "price-desc":
			return a.Price > b.Price
		case "area-desc":
			return a.Area > b.Area
		default:
			return a.CreatedAt > b.CreatedAt
		}
	})
	regionCounts := []RegionCount{}
	for _, c := range counts {
		c.Average /= c.Count
		regionCounts = append(regionCounts, *c)
	}
	sort.Slice(regionCounts, func(i, j int) bool { return regionCounts[i].Code < regionCounts[j].Code })
	hot := []RegionCount{}
	for _, c := range cities {
		c.Average /= c.Count
		hot = append(hot, *c)
	}
	sort.Slice(hot, func(i, j int) bool {
		if hot[i].Count == hot[j].Count {
			return hot[i].Code < hot[j].Code
		}
		return hot[i].Count > hot[j].Count
	})
	if len(hot) > 6 {
		hot = hot[:6]
	}
	total := len(selected)
	avg := 0
	median := 0
	if total > 0 {
		avg = totalPrice / total
		sort.Ints(prices)
		median = prices[total/2]
		if total%2 == 0 {
			median = (prices[total/2-1] + prices[total/2]) / 2
		}
	}
	markers := []map[string]any{}
	for i, l := range selected {
		if i >= 150 {
			break
		}
		markers = append(markers, map[string]any{"id": l.ID, "longitude": l.Longitude, "latitude": l.Latitude, "price": l.Price, "type": l.Type, "isDemo": l.IsDemo})
	}
	start := int(math.Min(float64((page-1)*9), float64(total)))
	end := minInt(start+9, total)
	send(w, 200, map[string]any{"items": selected[start:end], "total": total, "page": page, "pageSize": 9, "average": avg, "median": median, "types": types, "regions": regionCounts, "cities": hot, "markers": markers, "demo": true})
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	records, err := s.Store.All(r.Context())
	if err != nil {
		fail(w, 500, "房源加载失败")
		return
	}
	for _, l := range records {
		if l.ID == r.PathValue("id") {
			send(w, 200, l)
			return
		}
	}
	fail(w, 404, "房源不存在或已下架")
}

func session(w http.ResponseWriter, r *http.Request) (string, error) {
	if c, err := r.Cookie("atlas_session"); err == nil && len(c.Value) == 64 {
		if _, err := hex.DecodeString(c.Value); err == nil {
			return c.Value, nil
		}
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: "atlas_session", Value: id, Path: "/", MaxAge: 60 * 60 * 24 * 365, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
	return id, nil
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if !s.AllowPublish {
		fail(w, 403, "当前部署未启用发布功能，请联系管理员")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			fail(w, 403, "不允许跨站发布")
			return
		}
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 415, "请使用 JSON 提交房源")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	defer r.Body.Close()
	var l Listing
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		fail(w, 400, "房源内容格式无效或过大")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		fail(w, 400, "请只提交一条房源")
		return
	}
	l.Title = strings.TrimSpace(l.Title)
	l.Description = strings.TrimSpace(l.Description)
	l.Community = strings.TrimSpace(l.Community)
	l.Contact = strings.TrimSpace(l.Contact)
	if utf8.RuneCountInString(l.Title) < 5 || utf8.RuneCountInString(l.Title) > 60 {
		fail(w, 400, "标题需为 5–60 个字")
		return
	}
	if l.Type != "整租" && l.Type != "合租" && l.Type != "转租" {
		fail(w, 400, "请选择整租、合租或转租")
		return
	}
	if l.Price < 100 || l.Price > 1000000 || l.Area < 5 || l.Area > 2000 || l.Rooms < 1 || l.Rooms > 10 {
		fail(w, 400, "请填写有效租金、面积与卧室数")
		return
	}
	if utf8.RuneCountInString(l.Description) < 10 || utf8.RuneCountInString(l.Description) > 2000 {
		fail(w, 400, "房源描述需为 10–2000 个字")
		return
	}
	if l.Community == "" || utf8.RuneCountInString(l.Community) > 60 || utf8.RuneCountInString(l.Contact) < 3 || utf8.RuneCountInString(l.Contact) > 100 {
		fail(w, 400, "请填写小区和有效联系方式")
		return
	}
	district, ok := s.Maps.Get(l.DistrictCode)
	if !ok || district.Level != "district" {
		fail(w, 400, "请选择具体区县")
		return
	}
	city, ok := s.Maps.Get(district.Parent)
	if !ok {
		fail(w, 400, "行政区层级无效")
		return
	}
	province := city.Code
	if city.Level != "province" {
		province = city.Parent
	}
	l.ProvinceCode = province
	l.CityCode = city.Code
	l.City = city.Name
	l.District = district.Name
	l.Address = city.Name + district.Name + l.Community
	l.Longitude = district.Center[0]
	l.Latitude = district.Center[1]
	if len(l.Tags) > 6 {
		fail(w, 400, "房源标签最多 6 个")
		return
	}
	for _, tag := range l.Tags {
		if utf8.RuneCountInString(tag) > 12 {
			fail(w, 400, "标签过长")
			return
		}
	}
	if utf8.RuneCountInString(l.Floor) > 30 || utf8.RuneCountInString(l.Orientation) > 20 {
		fail(w, 400, "楼层或朝向内容过长")
		return
	}
	l.Image = "/images/home-1.jpg"
	l.IsDemo = false
	l.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fail(w, 500, "无法生成房源编号")
		return
	}
	l.ID = hex.EncodeToString(b)
	owner, err := session(w, r)
	if err != nil {
		fail(w, 500, "无法创建本地会话")
		return
	}
	l.Owner = owner
	if err = s.Store.Insert(r.Context(), l); err != nil {
		log.Printf("insert: %v", err)
		fail(w, 500, "保存失败，请重试")
		return
	}
	send(w, 201, l)
}

func send(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message string) {
	send(w, status, map[string]string{"error": message})
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
