package ps

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"terraform-percona/internal/cloud"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

type client struct {
	*sql.DB
}

func (m *manager) newClient(instance cloud.Instance, user, pass string) (*client, error) {
	cfg := mysql.Config{
		User:   user,
		Passwd: pass,
		Net:    "tcp",
		Addr:   instance.PublicIpAddress + ":" + strconv.Itoa(m.port),
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, errors.Wrap(err, "sql open")
	}
	return &client{DB: db}, nil
}

func (db *client) InstallPerconaServerUDF(ctx context.Context) error {
	_, err := db.ExecContext(ctx, "CREATE FUNCTION fnv1a_64 RETURNS INTEGER SONAME 'libfnv1a_udf.so'")
	if err != nil {
		return errors.Wrap(err, "create function fnv1a_64")
	}
	_, err = db.ExecContext(ctx, "CREATE FUNCTION fnv_64 RETURNS INTEGER SONAME 'libfnv_udf.so'")
	if err != nil {
		return errors.Wrap(err, "create function fnv_64")
	}
	_, err = db.ExecContext(ctx, "CREATE FUNCTION murmur_hash RETURNS INTEGER SONAME 'libmurmur_udf.so'")
	if err != nil {
		return errors.Wrap(err, "create function murmur_hash")
	}
	return nil
}

const (
	userReplica = "replica_user"
	userRoot    = "root"
)

func (db *client) ChangeReplicationSource(ctx context.Context, sourceHost string, sourcePort int, sourceUser, sourcePassword, sourceLogFile string, sourceLogPos int64) error {
	query := fmt.Sprintf(`CHANGE REPLICATION SOURCE TO SOURCE_HOST="%s", SOURCE_PORT=%d, SOURCE_USER="%s", SOURCE_PASSWORD="%s", SOURCE_LOG_FILE="%s", SOURCE_LOG_POS=%d`, sourceHost, sourcePort, sourceUser, sourcePassword, sourceLogFile, sourceLogPos)
	_, err := db.ExecContext(ctx, query)
	if err != nil {
		return errors.Wrap(err, "grant replication slave")
	}
	return nil
}

func (db *client) StartReplica(ctx context.Context) error {
	_, err := db.ExecContext(ctx, "START REPLICA")
	if err != nil {
		return errors.Wrap(err, "grant replication slave")
	}
	return nil
}

func (db *client) binlogFileAndPosition(ctx context.Context) (string, int64, error) {
	var (
		file            string
		position        int64
		binlogDoDB      string
		binlogIgnoreDB  string
		executedGtidSet string
	)
	row := db.QueryRowContext(ctx, "SHOW MASTER STATUS")
	row.Scan(&file, &position, &binlogDoDB, &binlogIgnoreDB, &executedGtidSet)
	if err := row.Err(); err != nil {
		return "", 0, errors.Wrap(err, "show master status")
	}
	return file, position, nil
}
