package atlas

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

func canManageListing(user *Account, listing Listing) bool {
	return user != nil && (user.Role == "admin" || listing.Owner != "" && listing.Owner == user.ID)
}

func (s *Server) managedListing(w http.ResponseWriter, r *http.Request, user *Account) (Listing, bool) {
	id := r.PathValue("id")
	if len(id) == 0 || len(id) > 64 {
		fail(w, 404, "房源不存在或已下架")
		return Listing{}, false
	}
	listing, err := s.Store.Find(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "房源不存在或已下架")
		return Listing{}, false
	}
	if err != nil {
		fail(w, 503, "房源加载失败")
		return Listing{}, false
	}
	if !canManageListing(user, listing) {
		fail(w, 403, "你无权编辑或删除此房源")
		return Listing{}, false
	}
	return listing, true
}

func (s *Server) deleteListing(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(w, r) {
		return
	}
	if s.Auth == nil {
		fail(w, 401, "请先登录")
		return
	}
	user, err := s.Auth.Current(r)
	if err != nil {
		fail(w, 503, "账号服务暂不可用")
		return
	}
	if user == nil {
		fail(w, 401, "请先登录")
		return
	}
	if !s.AllowPublish {
		fail(w, 403, "当前部署未启用房源管理")
		return
	}
	if !s.Auth.rate(w, r, "manage:"+user.ID, 60, 3600) {
		return
	}
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	listing, ok := s.managedListing(w, r, user)
	if !ok {
		return
	}
	if err = s.Store.DeleteListing(r.Context(), listing.ID, *user); err != nil {
		fail(w, 503, "删除失败，请重试")
		return
	}
	s.removePhotos(listing.Images)
	send(w, 200, map[string]bool{"ok": true})
}

// The SQL predicate is a second ownership check. Clients never select an owner or role.
func managementWhere(user Account) (string, []any) {
	if user.Role == "admin" {
		return "id=?", nil
	}
	return "id=? AND owner=?", []any{user.ID}
}

func (s *Store) UpdateListing(ctx context.Context, l Listing, user Account) error {
	tags, err := json.Marshal(l.Tags)
	if err != nil {
		return err
	}
	details, err := json.Marshal(l.ListingDetails)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	where, owner := managementWhere(user)
	args := []any{l.Title, l.Description, l.Type, l.Price, l.Area, l.Rooms, l.Floor, l.Orientation, l.Community, l.Address, l.Image, string(tags), l.Contact, l.ID}
	args = append(args, owner...)
	result, err := tx.ExecContext(ctx, s.bind(`UPDATE listings SET title=?,description=?,type=?,price=?,area=?,rooms=?,floor=?,orientation=?,community=?,address=?,image=?,tags=?,contact=? WHERE `+where), args...)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, s.bind(`INSERT INTO listing_details(listing_id,details) VALUES(?,?) ON CONFLICT(listing_id) DO UPDATE SET details=excluded.details`), l.ID, string(details)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteListing(ctx context.Context, id string, user Account) error {
	where, owner := managementWhere(user)
	args := append([]any{id}, owner...)
	result, err := s.DB.ExecContext(ctx, s.bind("DELETE FROM listings WHERE "+where), args...)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
