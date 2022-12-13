package psql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/lib/pq"
	"github.com/pkg/errors"

	internaldb "terraform-percona/internal/db"
)

type DB struct {
	*sql.DB
}

func NewClient(host, user, pass string) (*DB, error) {
	connStr := url.URL{
		Scheme:     "postgres",
		User:       url.UserPassword(user, pass),
		Host:       host,
		ForceQuery: false,
		RawQuery:   "connect_timeout=5",
	}
	db, err := sql.Open("postgres", connStr.String())
	if err != nil {
		return nil, errors.Wrap(err, "sql open")
	}
	return &DB{DB: db}, nil
}

func (db *DB) CreateUser(ctx context.Context, user, pass string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE USER %s WITH PASSWORD %s`, pq.QuoteIdentifier(user), pq.QuoteLiteral(pass)))
	if err != nil {
		return errors.Wrap(err, "exec")
	}
	return nil
}

func (db *DB) setMaxUserConnections(ctx context.Context, user string, connections int) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER USER %s CONNECTION LIMIT %d`, pq.QuoteIdentifier(user), connections))
	if err != nil {
		return errors.Wrap(err, "exec")
	}
	return nil
}

func (db *DB) CreatePMMUserForRDS(ctx context.Context, pass string) error {
	if err := db.CreateUser(ctx, internaldb.UserPMM, pass); err != nil {
		return errors.Wrap(err, "create user")
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`GRANT rds_superuser TO %s`, pq.QuoteIdentifier(internaldb.UserPMM))); err != nil {
		return errors.Wrap(err, "grant rds_superuser to pmm")
	}
	if err := db.setMaxUserConnections(ctx, internaldb.UserPMM, 10); err != nil {
		return errors.Wrap(err, "set max user connections")
	}
	return nil
}
