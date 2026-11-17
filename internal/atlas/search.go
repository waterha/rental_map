package atlas

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
)

var errTooManyIDs = errors.New("too many listing IDs")

type SearchResults struct {
	Items    []Listing      `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Average  int            `json:"average"`
	Median   int            `json:"median"`
	Types    map[string]int `json:"types"`
	Regions  []RegionCount  `json:"regions"`
	Cities   []RegionCount  `json:"cities"`
	Markers  []searchMarker `json:"markers"`
	Demo     bool           `json:"demo"`
}
type searchMarker struct {
	ID        string  `json:"id"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Price     int     `json:"price"`
	Type      string  `json:"type"`
	IsDemo    bool    `json:"isDemo"`
}

func escapeLike(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(value)) + "%"
}

// Filtering and aggregation stay in SQL. Only nine details and 150 coordinates
// enter Go memory, even if the database contains millions of public listings.
func (s *Store) Search(ctx context.Context, q url.Values, owner string, minPrice, maxPrice, rooms, page int, order string) (SearchResults, error) {
	result := SearchResults{Items: []Listing{}, Page: page, PageSize: 9, Types: map[string]int{"整租": 0, "转租": 0, "合租": 0}, Regions: []RegionCount{}, Cities: []RegionCount{}, Markers: []searchMarker{}, Demo: true}
	switch order {
	case "l.created_at DESC,l.id ASC", "l.price ASC,l.id ASC", "l.price DESC,l.id ASC", "l.area DESC,l.id ASC":
	default:
		return result, errors.New("invalid listing order")
	}
	if page < 1 || page > 100000 {
		return result, errors.New("invalid listing page")
	}
	conditions := []string{"l.price>=?"}
	args := []any{minPrice}
	add := func(condition string, values ...any) {
		conditions = append(conditions, condition)
		args = append(args, values...)
	}
	if kind := q.Get("type"); kind != "" {
		add("l.type=?", kind)
	}
	if maxPrice > 0 {
		add("l.price<=?", maxPrice)
	}
	if rooms > 0 {
		if rooms >= 3 {
			add("l.rooms>=3")
		} else {
			add("l.rooms=?", rooms)
		}
	}
	if keyword := strings.TrimSpace(q.Get("q")); keyword != "" {
		add(`LOWER(l.title||' '||l.community||' '||l.city||' '||l.district||' '||l.tags) LIKE ? ESCAPE '\'`, escapeLike(keyword))
	}
	if tag := q.Get("tag"); tag != "" {
		add(`LOWER(l.tags) LIKE ? ESCAPE '\'`, escapeLike(tag))
	}
	if ids, ok := q["ids"]; ok {
		values := strings.Split(strings.Join(ids, ","), ",")
		if len(values) > 150 {
			return result, errTooManyIDs
		}
		binds := []string{}
		idArgs := []any{}
		for _, id := range values {
			binds = append(binds, "?")
			idArgs = append(idArgs, id)
		}
		add("l.id IN ("+strings.Join(binds, ",")+")", idArgs...)
	}
	if q.Get("mine") == "true" {
		if owner == "" {
			add("1=0")
		} else {
			add("l.owner=?", owner)
		}
	}
	baseWhere := " WHERE " + strings.Join(conditions, " AND ")
	baseArgs := append([]any(nil), args...)
	if region := q.Get("region"); region != "" && region != "100000" {
		add("(l.province_code=? OR l.city_code=? OR l.district_code=?)", region, region, region)
	}
	where := " WHERE " + strings.Join(conditions, " AND ")
	var totalPrice int64
	if err := s.DB.QueryRowContext(ctx, s.bind("SELECT COUNT(*),COALESCE(SUM(l.price),0) FROM listings l"+where), args...).Scan(&result.Total, &totalPrice); err != nil {
		return result, err
	}
	if result.Total > 0 {
		result.Average = int(totalPrice / int64(result.Total))
		offset := (result.Total - 1) / 2
		take := 1
		if result.Total%2 == 0 {
			take = 2
		}
		medianArgs := append(append([]any(nil), args...), take, offset)
		rows, err := s.DB.QueryContext(ctx, s.bind("SELECT l.price FROM listings l"+where+" ORDER BY l.price LIMIT ? OFFSET ?"), medianArgs...)
		if err != nil {
			return result, err
		}
		sum := 0
		for rows.Next() {
			var price int
			if err = rows.Scan(&price); err != nil {
				break
			}
			sum += price
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
		if rowErr != nil {
			return result, rowErr
		}
		result.Median = sum / take
	}
	itemsArgs := append(append([]any(nil), args...), 9, (page-1)*9)
	items, err := s.readListings(ctx, where+" ORDER BY "+order+" LIMIT ? OFFSET ?", itemsArgs...)
	if err != nil {
		return result, err
	}
	for i := range items {
		items[i].Contact = ""
	}
	result.Items = items
	markers, err := s.DB.QueryContext(ctx, s.bind("SELECT l.id,l.longitude,l.latitude,l.price,l.type,l.is_demo FROM listings l"+where+" ORDER BY "+order+" LIMIT 150"), args...)
	if err != nil {
		return result, err
	}
	for markers.Next() {
		var marker searchMarker
		var demo int
		if err = markers.Scan(&marker.ID, &marker.Longitude, &marker.Latitude, &marker.Price, &marker.Type, &demo); err != nil {
			break
		}
		marker.IsDemo = demo == 1
		result.Markers = append(result.Markers, marker)
	}
	rowErr := markers.Err()
	markers.Close()
	if err != nil {
		return result, err
	}
	if rowErr != nil {
		return result, rowErr
	}
	types, err := s.DB.QueryContext(ctx, s.bind("SELECT l.type,COUNT(*) FROM listings l"+where+" GROUP BY l.type"), args...)
	if err != nil {
		return result, err
	}
	for types.Next() {
		var kind string
		var count int
		if err = types.Scan(&kind, &count); err != nil {
			break
		}
		result.Types[kind] = count
	}
	rowErr = types.Err()
	types.Close()
	if err != nil {
		return result, err
	}
	if rowErr != nil {
		return result, rowErr
	}
	groups, err := s.DB.QueryContext(ctx, s.bind("SELECT l.province_code,l.city_code,l.district_code,COUNT(*),SUM(l.price) FROM listings l"+where+" GROUP BY l.province_code,l.city_code,l.district_code"), args...)
	if err != nil {
		return result, err
	}
	regions := map[string]*RegionCount{}
	for groups.Next() {
		var province, city, district string
		var count, sum int
		if err = groups.Scan(&province, &city, &district, &count, &sum); err != nil {
			break
		}
		seen := map[string]bool{}
		for _, code := range []string{province, city, district} {
			if seen[code] {
				continue
			}
			seen[code] = true
			if regions[code] == nil {
				regions[code] = &RegionCount{Code: code}
			}
			regions[code].Count += count
			regions[code].Average += sum
		}
	}
	rowErr = groups.Err()
	groups.Close()
	if err != nil {
		return result, err
	}
	if rowErr != nil {
		return result, rowErr
	}
	for _, region := range regions {
		region.Average /= region.Count
		result.Regions = append(result.Regions, *region)
	}
	sort.Slice(result.Regions, func(i, j int) bool { return result.Regions[i].Code < result.Regions[j].Code })
	cities, err := s.DB.QueryContext(ctx, s.bind("SELECT l.city_code,l.city,COUNT(*),SUM(l.price) FROM listings l"+baseWhere+" GROUP BY l.city_code,l.city ORDER BY COUNT(*) DESC,l.city_code LIMIT 6"), baseArgs...)
	if err != nil {
		return result, err
	}
	defer cities.Close()
	for cities.Next() {
		var city RegionCount
		if err = cities.Scan(&city.Code, &city.Name, &city.Count, &city.Average); err != nil {
			return result, err
		}
		city.Average /= city.Count
		result.Cities = append(result.Cities, city)
	}
	return result, cities.Err()
}
