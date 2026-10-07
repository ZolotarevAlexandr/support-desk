package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrReference = errors.New("referenced record")

type Store struct{ DB *bun.DB }

func Open(dsn string, pool int) *Store {
	raw := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	raw.SetMaxOpenConns(pool)
	raw.SetMaxIdleConns(pool)
	raw.SetConnMaxLifetime(30 * time.Minute)
	return &Store{DB: bun.NewDB(raw, pgdialect.New())}
}

func Translate(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	var pg pgdriver.Error
	if errors.As(err, &pg) {
		switch pg.Field('C') {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrReference
		}
	}

	return err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.DB.NewSelect().Model(&u).Where("lower(email)=lower(?)", email).Scan(ctx)
	return u, Translate(err)
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := s.DB.NewSelect().Model(&u).Where("id=?", id).Scan(ctx)
	return u, Translate(err)
}

func (s *Store) Register(ctx context.Context, u *User, session *Session) error {
	return Translate(s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(u).Returning("*").Exec(ctx); err != nil {
			return err
		}

		session.UserID = u.ID
		_, err := tx.NewInsert().Model(session).Exec(ctx)
		return err
	}))
}

func (s *Store) CreateSession(ctx context.Context, v *Session) error {
	_, err := s.DB.NewInsert().Model(v).Exec(ctx)
	return Translate(err)
}

func (s *Store) SessionUser(ctx context.Context, hash string) (User, error) {
	var u User
	err := s.DB.NewSelect().Model(&u).Join("JOIN sessions AS s ON s.user_id=u.id").Where("s.token_hash=? AND s.expires_at>now()", hash).Scan(ctx)
	return u, Translate(err)
}

func (s *Store) Revoke(ctx context.Context, hash string) error {
	_, err := s.DB.NewDelete().Model((*Session)(nil)).Where("token_hash=?", hash).Exec(ctx)
	return err
}

func (s *Store) Cleanup(ctx context.Context) error {
	_, err := s.DB.NewDelete().Model((*Session)(nil)).Where("expires_at<=now()").Exec(ctx)
	return err
}

func (s *Store) Operators(ctx context.Context) ([]User, error) {
	items := []User{}
	err := s.DB.NewSelect().Model(&items).Where("role='operator'").Order("name", "id").Scan(ctx)
	return items, Translate(err)
}

func (s *Store) Categories(ctx context.Context) ([]Category, error) {
	v := []Category{}
	err := s.DB.NewSelect().Model(&v).Order("name", "id").Scan(ctx)
	return v, Translate(err)
}

func (s *Store) Category(ctx context.Context, id int64) (Category, error) {
	var v Category
	err := s.DB.NewSelect().Model(&v).Where("id=?", id).Scan(ctx)
	return v, Translate(err)
}

func (s *Store) SaveCategory(ctx context.Context, v *Category) error {
	if v.ID == 0 {
		_, err := s.DB.NewInsert().Model(v).Returning("*").Exec(ctx)
		return Translate(err)
	}

	v.UpdatedAt = time.Now().UTC()
	r, err := s.DB.NewUpdate().Model(v).Column("name", "description", "updated_at").WherePK().Returning("*").Exec(ctx)
	return affected(r, err)
}

func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	r, err := s.DB.NewDelete().Model((*Category)(nil)).Where("id=?", id).Exec(ctx)
	return affected(r, err)
}

func (s *Store) Seed(ctx context.Context) error {
	for _, name := range []string{"Hardware", "Software", "Network", "Access"} {
		v := Category{Name: name}
		if _, err := s.DB.NewInsert().Model(&v).On("CONFLICT DO NOTHING").Exec(ctx); err != nil {
			return err
		}
	}

	return nil
}

func affected(r sql.Result, err error) error {
	if err != nil {
		return Translate(err)
	}

	n, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return ErrNotFound
	}

	return nil
}

func applyFilter(q *bun.SelectQuery, u User, f Filter) *bun.SelectQuery {
	if u.Role != "operator" {
		q = q.Where("t.requester_id=?", u.ID)
	}

	if f.Search != "" {
		q = q.Where("(t.title ILIKE ? OR t.description ILIKE ?)", "%"+f.Search+"%", "%"+f.Search+"%")
	}

	if f.Status != "" {
		q = q.Where("t.status=?", f.Status)
	}

	if f.Priority != "" {
		q = q.Where("t.priority=?", f.Priority)
	}

	if f.CategoryID != 0 {
		q = q.Where("t.category_id=?", f.CategoryID)
	}

	if f.Assignment == "me" {
		q = q.Where("t.assignee_id=?", u.ID)
	}

	if f.Assignment == "unassigned" {
		q = q.Where("t.assignee_id IS NULL")
	}

	return q
}

func (s *Store) Tickets(ctx context.Context, u User, f Filter) (Page[Ticket], error) {
	p := Page[Ticket]{Items: []Ticket{}, Page: f.Page, PageSize: f.PageSize}

	// A single repeatable-read snapshot keeps items and total consistent under concurrent writes.
	err := s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}, func(ctx context.Context, tx bun.Tx) error {
		q := applyFilter(ticketNames(tx.NewSelect().Model(&p.Items)), u, f)
		count, err := q.Count(ctx)
		if err != nil {
			return err
		}

		p.Total = count
		return q.OrderExpr("t.created_at DESC,t.id DESC").Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).Scan(ctx)
	})

	return p, Translate(err)
}

// Names are projected for display without exposing account lists to requesters.
func ticketNames(q *bun.SelectQuery) *bun.SelectQuery {
	return q.ColumnExpr("t.*").
		ColumnExpr("requester.name AS requester_name").
		ColumnExpr("COALESCE(assignee.name, '') AS assignee_name").
		Join("JOIN users AS requester ON requester.id=t.requester_id").
		Join("LEFT JOIN users AS assignee ON assignee.id=t.assignee_id")
}

func TicketQuery(db bun.IDB, u User, id int64, lock bool) (*bun.SelectQuery, *Ticket) {
	v := new(Ticket)
	q := ticketNames(db.NewSelect().Model(v)).Where("t.id=?", id)
	if u.Role != "operator" {
		q = q.Where("t.requester_id=?", u.ID)
	}

	if lock {
		q = q.For("UPDATE OF t")
	}

	return q, v
}

func (s *Store) Ticket(ctx context.Context, u User, id int64) (Ticket, error) {
	q, v := TicketQuery(s.DB, u, id, false)
	err := q.Scan(ctx)
	return *v, Translate(err)
}

func (s *Store) CreateTicket(ctx context.Context, v *Ticket) error {
	_, err := s.DB.NewInsert().Model(v).Returning("*").Exec(ctx)
	return Translate(err)
}

// Mutations run rules after acquiring the same parent lock used by comment writes.
func (s *Store) MutateTicket(ctx context.Context, u User, id int64, fn func(context.Context, bun.Tx, *Ticket) error) error {
	return Translate(s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		q, v := TicketQuery(tx, u, id, true)
		if err := q.Scan(ctx); err != nil {
			return err
		}

		return fn(ctx, tx, v)
	}))
}

func SaveTicket(ctx context.Context, tx bun.Tx, v *Ticket) error {
	v.UpdatedAt = time.Now().UTC()
	_, err := tx.NewUpdate().Model(v).Column("title", "description", "category_id", "priority", "status", "assignee_id", "updated_at").WherePK().Exec(ctx)
	return err
}

func DeleteTicket(ctx context.Context, tx bun.Tx, id int64) error {
	_, err := tx.NewDelete().Model((*Ticket)(nil)).Where("id=?", id).Exec(ctx)
	return err
}

func (s *Store) Comments(ctx context.Context, u User, id int64, page, size int) (Page[Comment], error) {
	p := Page[Comment]{Items: []Comment{}, Page: page, PageSize: size}
	err := s.DB.RunInTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}, func(ctx context.Context, tx bun.Tx) error {
		q, _ := TicketQuery(tx, u, id, false)
		if err := q.Scan(ctx); err != nil {
			return err
		}

		base := tx.NewSelect().Model(&p.Items).Where("c.ticket_id=?", id)
		n, err := base.Count(ctx)
		if err != nil {
			return err
		}

		p.Total = n
		if err = base.OrderExpr("c.created_at,c.id").Limit(size).Offset((page - 1) * size).Scan(ctx); err != nil {
			return err
		}

		for i := range p.Items {
			var author User
			if err := tx.NewSelect().Model(&author).Where("id=?", p.Items[i].AuthorID).Scan(ctx); err != nil {
				return err
			}

			p.Items[i].AuthorName = author.Name
			p.Items[i].AuthorRole = author.Role
		}

		return nil
	})

	return p, Translate(err)
}

func InsertComment(ctx context.Context, tx bun.Tx, v *Comment) error {
	_, err := tx.NewInsert().Model(v).Returning("*").Exec(ctx)
	return err
}

func GetComment(ctx context.Context, tx bun.Tx, ticket, id int64) (Comment, error) {
	var v Comment
	err := tx.NewSelect().Model(&v).Where("id=? AND ticket_id=?", id, ticket).Scan(ctx)
	return v, err
}

func SaveComment(ctx context.Context, tx bun.Tx, v *Comment) error {
	v.UpdatedAt = time.Now().UTC()
	_, err := tx.NewUpdate().Model(v).Column("body", "updated_at").WherePK().Exec(ctx)
	return err
}

func DeleteComment(ctx context.Context, tx bun.Tx, id int64) error {
	_, err := tx.NewDelete().Model((*Comment)(nil)).Where("id=?", id).Exec(ctx)
	return err
}
