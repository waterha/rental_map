package atlas

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB       *sql.DB
	Postgres bool
}

type Listing struct {
	ListingDetails
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Type         string   `json:"type"`
	Price        int      `json:"price"`
	Area         float64  `json:"area"`
	Rooms        int      `json:"rooms"`
	Floor        string   `json:"floor"`
	Orientation  string   `json:"orientation"`
	Community    string   `json:"community"`
	ProvinceCode string   `json:"provinceCode"`
	CityCode     string   `json:"cityCode"`
	DistrictCode string   `json:"districtCode"`
	City         string   `json:"city"`
	District     string   `json:"district"`
	Address      string   `json:"address"`
	Longitude    float64  `json:"longitude"`
	Latitude     float64  `json:"latitude"`
	Image        string   `json:"image"`
	Tags         []string `json:"tags"`
	Contact      string   `json:"contact"`
	CreatedAt    string   `json:"createdAt"`
	IsDemo       bool     `json:"isDemo"`
	Owner        string   `json:"-"`
	CanManage    bool     `json:"canManage"`
}

// Separate metadata keeps upgrades compatible with existing SQLite and PostgreSQL databases.
type ListingDetails struct {
	Images       []string `json:"images,omitempty"`
	RentalSource string   `json:"rentalSource,omitempty"`
	PropertyType string   `json:"propertyType,omitempty"`
	Halls        int      `json:"halls"`
	Bathrooms    int      `json:"bathrooms"`
}

func OpenStore(ctx context.Context, directory, dsn string) (*Store, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, err
	}
	driver := "sqlite"
	pg := dsn != ""
	if pg {
		driver = "pgx"
	} else {
		dsn = filepath.Join(directory, "rental.db")
		file, err := os.OpenFile(dsn, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		file.Close()
		if err = os.Chmod(dsn, 0600); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db, Postgres: pg}
	if pg {
		db.SetMaxOpenConns(12)
		db.SetMaxIdleConns(4)
		db.SetConnMaxLifetime(30 * time.Minute)
	} else {
		db.SetMaxOpenConns(1)
		if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;"); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if os.Getenv("APP_ENV") == "production" {
		if !pg {
			db.Close()
			return nil, fmt.Errorf("production requires PostgreSQL")
		}
		var privileged bool
		// MEMBER also catches roles reachable through SET ROLE even if NOINHERIT
		// currently hides their privileges. Built-in pg_* roles grant capabilities
		// outside this application's schema and are unnecessary for the service.
		if err = db.QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM pg_roles WHERE pg_has_role(current_user,oid,'MEMBER')
 AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls OR LEFT(rolname,3)='pg_')
 )`).Scan(&privileged); err != nil || privileged {
			db.Close()
			return nil, fmt.Errorf("production requires a restricted database role")
		}
	}
	schema := `CREATE TABLE IF NOT EXISTS listings (
 id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL, type TEXT NOT NULL,
 price INTEGER NOT NULL CHECK(price>0), area REAL NOT NULL CHECK(area>0), rooms INTEGER NOT NULL,
 floor TEXT NOT NULL, orientation TEXT NOT NULL, community TEXT NOT NULL,
 province_code TEXT NOT NULL, city_code TEXT NOT NULL, district_code TEXT NOT NULL,
 city TEXT NOT NULL, district TEXT NOT NULL, address TEXT NOT NULL,
 longitude DOUBLE PRECISION NOT NULL, latitude DOUBLE PRECISION NOT NULL,
 image TEXT NOT NULL, tags TEXT NOT NULL, contact TEXT NOT NULL,
 created_at TEXT NOT NULL, is_demo INTEGER NOT NULL, owner TEXT NOT NULL
 );
 CREATE INDEX IF NOT EXISTS idx_listings_province ON listings(province_code, type, price);
 CREATE INDEX IF NOT EXISTS idx_listings_city ON listings(city_code, type, price);
 CREATE INDEX IF NOT EXISTS idx_listings_district ON listings(district_code, type, price);
 CREATE INDEX IF NOT EXISTS idx_listings_created ON listings(created_at);
 CREATE INDEX IF NOT EXISTS idx_listings_owner ON listings(owner);
 CREATE TABLE IF NOT EXISTS listing_details (
 listing_id TEXT PRIMARY KEY REFERENCES listings(id) ON DELETE CASCADE, details TEXT NOT NULL
 );`
	if _, err = db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) bind(query string) string {
	if !s.Postgres {
		return query
	}
	n := 0
	return replaceBinds(query, &n)
}

func replaceBinds(query string, n *int) string {
	var out strings.Builder
	for _, c := range query {
		if c == '?' {
			*n++
			fmt.Fprintf(&out, "$%d", *n)
		} else {
			out.WriteRune(c)
		}
	}
	return out.String()
}

func (s *Store) Insert(ctx context.Context, l Listing) error {
	tags, _ := json.Marshal(l.Tags)
	demo := 0
	if l.IsDemo {
		demo = 1
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, s.bind(`INSERT INTO listings (id,title,description,type,price,area,rooms,floor,orientation,community,province_code,city_code,district_code,city,district,address,longitude,latitude,image,tags,contact,created_at,is_demo,owner) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		l.ID, l.Title, l.Description, l.Type, l.Price, l.Area, l.Rooms, l.Floor, l.Orientation, l.Community, l.ProvinceCode, l.CityCode, l.DistrictCode, l.City, l.District, l.Address, l.Longitude, l.Latitude, l.Image, string(tags), l.Contact, l.CreatedAt, demo, l.Owner)
	if err != nil {
		return err
	}
	details, err := json.Marshal(l.ListingDetails)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.bind("INSERT INTO listing_details(listing_id,details) VALUES(?,?)"), l.ID, string(details)); err != nil {
		return err
	}
	return tx.Commit()
}

const listingColumns = `l.id,l.title,l.description,l.type,l.price,l.area,l.rooms,l.floor,l.orientation,l.community,l.province_code,l.city_code,l.district_code,l.city,l.district,l.address,l.longitude,l.latitude,l.image,l.tags,l.contact,l.created_at,l.is_demo,l.owner,COALESCE(d.details,'{}')`
const listingJoin = ` FROM listings l LEFT JOIN listing_details d ON d.listing_id=l.id`

type listingScanner interface{ Scan(...any) error }

func scanListing(row listingScanner) (Listing, error) {
	var l Listing
	var tags, details string
	var demo int
	err := row.Scan(&l.ID, &l.Title, &l.Description, &l.Type, &l.Price, &l.Area, &l.Rooms, &l.Floor, &l.Orientation, &l.Community, &l.ProvinceCode, &l.CityCode, &l.DistrictCode, &l.City, &l.District, &l.Address, &l.Longitude, &l.Latitude, &l.Image, &tags, &l.Contact, &l.CreatedAt, &demo, &l.Owner, &details)
	if err != nil {
		return l, err
	}
	if err = json.Unmarshal([]byte(tags), &l.Tags); err != nil {
		return l, err
	}
	if err = json.Unmarshal([]byte(details), &l.ListingDetails); err != nil {
		return l, err
	}
	if len(l.Images) == 0 && l.Image != "" {
		l.Images = []string{l.Image}
	}
	l.IsDemo = demo == 1
	return l, nil
}
func (s *Store) readListings(ctx context.Context, tail string, args ...any) ([]Listing, error) {
	rows, err := s.DB.QueryContext(ctx, s.bind("SELECT "+listingColumns+listingJoin+tail), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	listings := []Listing{}
	for rows.Next() {
		l, err := scanListing(rows)
		if err != nil {
			return nil, err
		}
		listings = append(listings, l)
	}
	return listings, rows.Err()
}
func (s *Store) All(ctx context.Context) ([]Listing, error) {
	return s.readListings(ctx, " ORDER BY l.created_at DESC,l.id ASC")
}
func (s *Store) Find(ctx context.Context, id string) (Listing, error) {
	return scanListing(s.DB.QueryRowContext(ctx, s.bind("SELECT "+listingColumns+listingJoin+" WHERE l.id=?"), id))
}
