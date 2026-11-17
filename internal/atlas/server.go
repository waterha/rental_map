package atlas

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Server struct {
	photos       *photoBudget
	Security     *Security
	Store        *Store
	Maps         *Maps
	StaticDir    string
	AllowPublish bool
	Auth         *Auth
	Metro        *Metro
}
type RegionCount struct {
	Code    string `json:"code"`
	Count   int    `json:"count"`
	Average int    `json:"average"`
	Name    string `json:"name"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if s.Auth != nil {
		s.Auth.Routes(mux)
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.DB.PingContext(r.Context()); err != nil {
			fail(w, 503, "数据库暂不可用")
			return
		}
		send(w, 200, map[string]any{"status": "ok", "publishing": s.AllowPublish})
	})
	mux.HandleFunc("GET /api/regions", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, s.Maps.List(strings.TrimSpace(r.URL.Query().Get("q"))))
	})
	mux.HandleFunc("GET /api/maps/{code}", s.mapData)
	mux.HandleFunc("GET /api/metro/{code}", s.metro)
	mux.HandleFunc("GET /api/listings", s.listings)
	mux.HandleFunc("GET /api/listings/{id}", s.detail)
	mux.HandleFunc("POST /api/listings", s.create)
	mux.HandleFunc("GET /uploads/{name}", s.photo)
	mux.HandleFunc("GET /uploads/", http.NotFound)
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		send(w, 200, map[string]bool{"publishing": s.AllowPublish})
	})
	mux.HandleFunc("GET /images/{name}", func(w http.ResponseWriter, r *http.Request) {
		servePublicFile(w, r, filepath.Join(s.Maps.Dir, "images"), r.PathValue("name"))
	})
	mux.HandleFunc("GET /images/", http.NotFound)
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "接口不存在") })
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimPrefix(r.URL.Path, "/")
		if requested == "" {
			requested = "index.html"
		}
		if requested != "index.html" && requested != "favicon.svg" && !strings.HasPrefix(requested, "assets/") {
			http.NotFound(w, r)
			return
		}
		servePublicFile(w, r, s.StaticDir, requested)
	})
	guard := s.Security
	if guard == nil {
		guard = NewSecurity()
	}
	if s.photos == nil {
		s.photos = newPhotoBudget(s.Maps.Dir, guard.StorageBytes)
	}
	return guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		defer func() {
			if p := recover(); p != nil {
				log.Print("request panic recovered")
				fail(w, 500, "服务暂时不可用")
			}
		}()
		mux.ServeHTTP(w, r)
	}))
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
	sortMode := q.Get("sort")
	orders := map[string]string{"": "l.created_at DESC,l.id ASC", "newest": "l.created_at DESC,l.id ASC", "price-asc": "l.price ASC,l.id ASC", "price-desc": "l.price DESC,l.id ASC", "area-desc": "l.area DESC,l.id ASC"}
	order, ok := orders[sortMode]
	if !ok {
		fail(w, 400, "排序方式无效")
		return
	}
	if utf8.RuneCountInString(q.Get("tag")) > 12 || len(strings.Join(q["ids"], ",")) > 10000 {
		fail(w, 400, "筛选条件过长")
		return
	}
	owner := ""
	if q.Get("mine") == "true" && s.Auth != nil {
		user, err := s.Auth.Current(r)
		if err != nil {
			fail(w, 503, "账号服务暂不可用")
			return
		}
		if user != nil {
			owner = user.ID
		}
	}
	result, err := s.Store.Search(r.Context(), q, owner, min, max, rooms, page, order)
	if errors.Is(err, errTooManyIDs) {
		fail(w, 400, "一次最多查询 150 条房源")
		return
	}
	if err != nil {
		fail(w, 503, "房源加载失败，请稍后重试")
		return
	}
	for i := range result.Regions {
		if region, ok := s.Maps.Get(result.Regions[i].Code); ok {
			result.Regions[i].Name = region.Name
		}
	}
	send(w, 200, result)
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if len(id) > 64 {
		fail(w, 404, "房源不存在或已下架")
		return
	}
	listing, err := s.Store.Find(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "房源不存在或已下架")
		return
	}
	if err != nil {
		fail(w, 503, "房源加载失败")
		return
	}
	send(w, 200, listing)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if s.Auth == nil {
		fail(w, 401, "请先登录后再发布房源")
		return
	}
	user, err := s.Auth.Current(r)
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	if user == nil {
		fail(w, 401, "请先登录后再发布房源")
		return
	}
	accountID := user.ID
	if !s.AllowPublish {
		fail(w, 403, "当前部署未启用发布功能，请联系管理员")
		return
	}
	if !sameOrigin(w, r) {
		return
	}
	if !s.Auth.rate(w, r, "publish:"+accountID, 20, 3600) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 26<<20)
	defer r.Body.Close()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
		fail(w, 415, "请同时提交房源资料和 1–5 张照片")
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		fail(w, 400, "上传内容无效或超过 26 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.Value["listing"]) != 1 || len(r.FormValue("listing")) > 32<<10 {
		fail(w, 400, "房源资料无效或过大")
		return
	}
	var l Listing
	dec := json.NewDecoder(strings.NewReader(r.FormValue("listing")))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		fail(w, 400, "房源内容格式无效")
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		fail(w, 400, "请只提交一条房源")
		return
	}
	if len(r.MultipartForm.File["photos"]) < 1 || len(r.MultipartForm.File["photos"]) > 5 {
		fail(w, 400, "请上传 1–5 张房源照片")
		return
	}
	if l.RentalSource != "房东直租" && l.RentalSource != "个人转租" && l.RentalSource != "合租招室友" {
		fail(w, 400, "请选择出租身份")
		return
	}
	if l.PropertyType != "住宅" && l.PropertyType != "公寓" && l.PropertyType != "其他" {
		fail(w, 400, "请选择房屋类型")
		return
	}
	if l.Halls < 0 || l.Halls > 10 || l.Bathrooms < 1 || l.Bathrooms > 10 {
		fail(w, 400, "请填写有效厅数与卫生间数")
		return
	}
	l.Title = strings.TrimSpace(l.Title)
	l.Description = strings.TrimSpace(l.Description)
	l.Community = strings.TrimSpace(l.Community)
	l.Contact = strings.TrimSpace(l.Contact)
	if utf8.RuneCountInString(l.Title) < 5 || utf8.RuneCountInString(l.Title) > 60 {
		fail(w, 400, "房源需为 5–60 个字")
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
	if utf8.RuneCountInString(l.Community) > 60 {
		fail(w, 400, "小区名称过长")
		return
	}
	if utf8.RuneCountInString(l.Contact) < 3 || utf8.RuneCountInString(l.Contact) > 100 {
		fail(w, 400, "请填写有效联系方式")
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
	inside, err := s.locationInDistrict(r.Context(), district, [2]float64{l.Longitude, l.Latitude})
	if err != nil {
		fail(w, 503, "区县边界暂不可用，请稍后重试")
		return
	}
	if !inside {
		fail(w, 400, "请在所选区县内点选房源位置")
		return
	}
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

	l.IsDemo = false
	l.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fail(w, 500, "无法生成房源编号")
		return
	}
	l.ID = hex.EncodeToString(b)
	l.Owner = accountID
	images, err := s.savePhotos(r.MultipartForm.File["photos"])
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	l.Images = images
	l.Image = images[0]
	saved := false
	defer func() {
		if !saved {
			s.removePhotos(images)
		}
	}()
	if err = s.Store.Insert(r.Context(), l); err != nil {
		log.Printf("insert: %v", err)
		fail(w, 500, "保存失败，请重试")
		return
	}
	saved = true
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
