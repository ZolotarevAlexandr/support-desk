package store

import (
	"github.com/uptrace/bun"
	"time"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u"`
	ID            int64     `bun:",pk,autoincrement" json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	PasswordHash  string    `json:"-"`
	Role          string    `json:"role"`
	CreatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"updated_at"`
}

type Session struct {
	bun.BaseModel `bun:"table:sessions"`
	TokenHash     string `bun:",pk"`
	UserID        int64
	CreatedAt     time.Time `bun:",nullzero,default:current_timestamp"`
	ExpiresAt     time.Time
}

type Category struct {
	bun.BaseModel `bun:"table:categories,alias:cat"`
	ID            int64     `bun:",pk,autoincrement" json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	CreatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"updated_at"`
}

type Ticket struct {
	RequesterName string `bun:",scanonly" json:"requester_name"`
	AssigneeName  string `bun:",scanonly" json:"assignee_name"`
	bun.BaseModel `bun:"table:tickets,alias:t"`
	ID            int64     `bun:",pk,autoincrement" json:"id"`
	RequesterID   int64     `json:"requester_id"`
	AssigneeID    *int64    `json:"assignee_id"`
	CategoryID    int64     `json:"category_id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Priority      string    `json:"priority"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"updated_at"`
}

type Comment struct {
	bun.BaseModel `bun:"table:comments,alias:c"`
	ID            int64     `bun:",pk,autoincrement" json:"id"`
	TicketID      int64     `json:"ticket_id"`
	AuthorID      int64     `json:"author_id"`
	Body          string    `json:"body"`
	CreatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"created_at"`
	UpdatedAt     time.Time `bun:",nullzero,default:current_timestamp" json:"updated_at"`
	AuthorName    string    `bun:"-" json:"author_name"`
	AuthorRole    string    `bun:"-" json:"author_role"`
}

type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

type Filter struct {
	Search, Status, Priority, Assignment string
	CategoryID                           int64
	Page, PageSize                       int
}
