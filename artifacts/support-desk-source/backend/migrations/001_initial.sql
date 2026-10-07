-- +goose Up
CREATE TABLE users (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
 email TEXT NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
 password_hash TEXT NOT NULL,
 role TEXT NOT NULL CHECK (role IN ('user','operator')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_ci ON users (lower(email));
CREATE TABLE sessions (
 token_hash TEXT PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expiry ON sessions(expires_at);
CREATE TABLE categories (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
 description TEXT NOT NULL DEFAULT '' CHECK (length(description)<=1000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX categories_name_ci ON categories(lower(name));
CREATE TABLE tickets (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 requester_id BIGINT NOT NULL REFERENCES users(id),
 assignee_id BIGINT REFERENCES users(id),
 category_id BIGINT NOT NULL REFERENCES categories(id),
 title TEXT NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
 description TEXT NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 10000),
 priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low','medium','high','urgent')),
 status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','in_progress','resolved','closed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tickets_requester ON tickets(requester_id);
CREATE INDEX tickets_assignee ON tickets(assignee_id);
CREATE INDEX tickets_category ON tickets(category_id);
CREATE INDEX tickets_status ON tickets(status);
CREATE INDEX tickets_created ON tickets(created_at DESC,id DESC);
CREATE TABLE comments (
 id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 ticket_id BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
 author_id BIGINT NOT NULL REFERENCES users(id),
 body TEXT NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 5000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX comments_order ON comments(ticket_id,created_at,id);
-- +goose Down
DROP TABLE comments,tickets,categories,sessions,users;
