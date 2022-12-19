package mysql

import (
	"context"
	"database/sql"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"

	internaldb "terraform-percona/internal/db"
)

type DB struct {
	*sql.DB
	cfg mysql.Config
}

func NewClient(host, user, pass string) (*DB, error) {
	db := &DB{
		cfg: mysql.Config{
			User:   user,
			Passwd: pass,
			Net:    "tcp",
			Addr:   host,
			Params: map[string]string{
				"interpolateParams": "true",
			},
			Timeout:              time.Second * 135,
			ReadTimeout:          time.Second * 135,
			WriteTimeout:         time.Second * 135,
			AllowNativePasswords: true,
		},
	}
	err := db.Open()
	if err != nil {
		return nil, errors.Wrap(err, "failed to open connection")
	}
	return db, nil
}

func (db *DB) Open() error {
	sqldb, err := sql.Open("mysql", db.cfg.FormatDSN())
	if err != nil {
		return errors.Wrap(err, "sql open")
	}
	db.DB = sqldb
	return nil
}

func (db *DB) InstallPerconaServerUDF(ctx context.Context) error {
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

func (db *DB) ChangeReplicationSource(ctx context.Context, sourceHost string, sourcePort int, sourceUser, sourcePassword, sourceLogFile string, sourceLogPos int64) error {
	_, err := db.ExecContext(ctx, `CHANGE REPLICATION SOURCE TO SOURCE_HOST=?, SOURCE_PORT=?, SOURCE_USER=?, SOURCE_PASSWORD=?, SOURCE_LOG_FILE=?, SOURCE_LOG_POS=?`, sourceHost, sourcePort, sourceUser, sourcePassword, sourceLogFile, sourceLogPos)
	if err != nil {
		return errors.Wrap(err, "exec")
	}
	return nil
}

func (db *DB) StartReplica(ctx context.Context) error {
	_, err := db.ExecContext(ctx, "START REPLICA")
	if err != nil {
		return errors.Wrap(err, "exec")
	}
	return nil
}

func (db *DB) BinlogFileAndPosition(ctx context.Context) (string, int64, error) {
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

func (db *DB) createUser(ctx context.Context, user, host, pass string) error {
	_, err := db.ExecContext(ctx, `CREATE USER IF NOT EXISTS ?@? IDENTIFIED WITH mysql_native_password BY ?`, user, host, pass)
	if err != nil {
		return errors.Wrap(err, "exec")
	}
	return nil
}

func (db *DB) CreateReplicaUser(ctx context.Context, password string) error {
	if err := db.createUser(ctx, internaldb.UserReplica, "%", password); err != nil {
		return errors.Wrap(err, "create user")
	}
	if _, err := db.ExecContext(ctx, "GRANT REPLICATION SLAVE ON *.* TO ?@?", internaldb.UserReplica, "%"); err != nil {
		return errors.Wrap(err, "grant replication slave")
	}
	return nil
}

func (db *DB) setMaxUserConnections(ctx context.Context, user, host string, connections int) error {
	_, err := db.ExecContext(ctx, `ALTER USER ?@? WITH MAX_USER_CONNECTIONS ?`, user, host, connections)
	if err != nil {
		return errors.Wrap(err, "exec")
	}
	return nil
}

func (db *DB) CreatePMMUser(ctx context.Context, password string) error {
	if err := db.createUser(ctx, internaldb.UserPMM, "localhost", password); err != nil {
		return errors.Wrap(err, "create user")
	}
	if err := db.setMaxUserConnections(ctx, internaldb.UserPMM, "localhost", 10); err != nil {
		return errors.Wrap(err, "max user connections")
	}
	if _, err := db.ExecContext(ctx, "GRANT SELECT, PROCESS, REPLICATION CLIENT, RELOAD, BACKUP_ADMIN ON *.* TO ?@?", internaldb.UserPMM, "localhost"); err != nil {
		return errors.Wrap(err, "grant replication slave")
	}
	return nil
}

func (db *DB) CreatePMMUserForRDS(ctx context.Context, password string) error {
	if err := db.createUser(ctx, internaldb.UserPMM, "%", password); err != nil {
		return errors.Wrap(err, "create user")
	}
	if _, err := db.ExecContext(ctx, "GRANT SELECT, PROCESS, REPLICATION CLIENT ON *.* TO ?@?", internaldb.UserPMM, "%"); err != nil {
		return errors.Wrap(err, "grant replication slave")
	}
	if err := db.setMaxUserConnections(ctx, internaldb.UserPMM, "%", 10); err != nil {
		return errors.Wrap(err, "max user connections")
	}
	if _, err := db.ExecContext(ctx, "GRANT SELECT, UPDATE, DELETE, DROP ON performance_schema.* TO ?@?", internaldb.UserPMM, "%"); err != nil {
		return errors.Wrap(err, "grant replication slave")
	}
	return nil
}
