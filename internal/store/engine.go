package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Engine names.
const (
	EnginePostgres = "postgres"
	EngineSQLite   = "sqlite"
)

// engine is the one place the two databases differ. Every statement in the
// store goes through it. It rewrites placeholders for SQLite, converts
// arguments the drivers would otherwise disagree on, and nothing else: the
// SQL itself is the same on both (ADR 004 stage 2).
type engine struct {
	db   *sql.DB
	kind string
}

// timeLayout is how SQLite stores every timestamp: UTC, fixed width to the
// microsecond, so text ordering is time ordering and the precision matches
// what PostgreSQL keeps. The two engines therefore return equal values for
// equal inputs.
const timeLayout = "2006-01-02T15:04:05.000000Z"

func (e *engine) sql(q string) string {
	if e.kind != EngineSQLite {
		return q
	}
	// $N becomes ?N: SQLite's explicitly numbered positional parameter,
	// which binds argument N wherever it appears and however often.
	var b strings.Builder
	b.Grow(len(q))
	for i := 0; i < len(q); i++ {
		if q[i] == '$' && i+1 < len(q) && q[i+1] >= '0' && q[i+1] <= '9' {
			b.WriteByte('?')
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

func (e *engine) args(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = e.arg(a)
	}
	return out
}

func (e *engine) arg(a any) any {
	switch v := a.(type) {
	case time.Time:
		if e.kind == EngineSQLite {
			return v.UTC().Format(timeLayout)
		}
		return v.UTC()
	case *time.Time:
		if v == nil {
			return nil
		}
		return e.arg(*v)
	case []string:
		if v == nil {
			v = []string{}
		}
		b, _ := json.Marshal(v)
		return string(b)
	case []byte:
		// JSON documents. Neither engine has a bytea column in this schema,
		// and PostgreSQL's driver would otherwise send bytes as bytea.
		if v == nil {
			return nil
		}
		return string(v)
	case *string:
		if v == nil {
			return nil
		}
		return *v
	case *int64:
		if v == nil {
			return nil
		}
		return *v
	}
	return a
}

func (e *engine) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return e.db.QueryContext(ctx, e.sql(q), e.args(args)...)
}

func (e *engine) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return e.db.QueryRowContext(ctx, e.sql(q), e.args(args)...)
}

func (e *engine) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return e.db.ExecContext(ctx, e.sql(q), e.args(args)...)
}

func (e *engine) begin(ctx context.Context) (*tx, error) {
	t, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &tx{t: t, e: e}, nil
}

// tx is a transaction through the engine.
type tx struct {
	t *sql.Tx
	e *engine
}

func (x *tx) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return x.t.QueryContext(ctx, x.e.sql(q), x.e.args(args)...)
}

func (x *tx) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return x.t.QueryRowContext(ctx, x.e.sql(q), x.e.args(args)...)
}

func (x *tx) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return x.t.ExecContext(ctx, x.e.sql(q), x.e.args(args)...)
}

func (x *tx) Commit() error   { return x.t.Commit() }
func (x *tx) Rollback() error { return x.t.Rollback() }

// querier is what fingerprintRows and sinceIn need: an engine or a tx.
type querier interface {
	query(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

// ---------------------------------------------------------------------------
// Scanners: the read side of the same seams.
// ---------------------------------------------------------------------------

// tsv scans a timestamp into a time.Time from either driver's
// representation.
type tsv struct{ p *time.Time }

func ts(p *time.Time) tsv { return tsv{p} }

func (s tsv) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*s.p = time.Time{}
		return nil
	case time.Time:
		*s.p = v.UTC()
		return nil
	case string:
		return s.parse(v)
	case []byte:
		return s.parse(string(v))
	}
	return fmt.Errorf("cannot scan %T into time", src)
}

func (s tsv) parse(v string) error {
	for _, layout := range []string{timeLayout, time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999"} {
		if t, err := time.Parse(layout, v); err == nil {
			*s.p = t.UTC()
			return nil
		}
	}
	return fmt.Errorf("cannot parse time %q", v)
}

// tsp scans a nullable timestamp into a *time.Time.
type tspv struct{ p **time.Time }

func tsp(p **time.Time) tspv { return tspv{p} }

func (s tspv) Scan(src any) error {
	if src == nil {
		*s.p = nil
		return nil
	}
	var t time.Time
	if err := (tsv{&t}).Scan(src); err != nil {
		return err
	}
	*s.p = &t
	return nil
}

// jsl scans a JSON array of strings into a []string.
type jslv struct{ p *[]string }

func jsl(p *[]string) jslv { return jslv{p} }

func (s jslv) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*s.p = nil
		return nil
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return fmt.Errorf("cannot scan %T into []string", src)
	}
	return json.Unmarshal(raw, s.p)
}

var _ driver.Valuer = (*noValuer)(nil)

type noValuer struct{}

func (noValuer) Value() (driver.Value, error) { return nil, nil }

// ---------------------------------------------------------------------------
// Opening
// ---------------------------------------------------------------------------

// openEngine picks the engine from the DSN. postgres:// and postgresql://
// open PostgreSQL through pgx; sqlite:<path> and sqlite://<path> open a
// SQLite file, creating it and its schema if needed. Anything else is
// handed to pgx unchanged, which is the historical default.
func openEngine(ctx context.Context, dsn string) (*engine, error) {
	switch {
	case strings.HasPrefix(dsn, "sqlite:"):
		path := strings.TrimPrefix(strings.TrimPrefix(dsn, "sqlite:"), "//")
		return openSQLite(ctx, path)
	default:
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, err
		}
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("connect %s: %w", dsn, err)
		}
		return &engine{db: db, kind: EnginePostgres}, nil
	}
}

func openSQLite(ctx context.Context, path string) (*engine, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite: a file path is required, e.g. sqlite:gyst.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Foreign keys give the ON DELETE CASCADE the projections rely on. WAL
	// lets a reader (the report) run while a scan writes. One connection
	// keeps the single writer honest; the solo profile has one user.
	db, err := sql.Open("sqlite", "file:"+path+
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	e := &engine{db: db, kind: EngineSQLite}
	if err := e.migrateSQLite(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return e, nil
}

// migrateSQLite applies the embedded schema. It is written with IF NOT
// EXISTS throughout, so applying it to an existing file is a no-op; a
// schema_migrations table records what has been applied for when a second
// file exists.
func (e *engine) migrateSQLite(ctx context.Context) error {
	if _, err := e.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	for _, m := range sqliteMigrations {
		var n int
		if err := e.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE name=?1`, m.name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if _, err := e.db.ExecContext(ctx, m.body); err != nil {
			return fmt.Errorf("sqlite migration %s: %w", m.name, err)
		}
		if _, err := e.db.ExecContext(ctx, `INSERT INTO schema_migrations (name, applied_at) VALUES (?1, ?2)`,
			m.name, time.Now().UTC().Format(timeLayout)); err != nil {
			return err
		}
	}
	return nil
}

// Engine reports which database this store is on.
func (s *Store) Engine() string { return s.db.kind }

// affected reads a result's row count, treating a driver that cannot say
// as zero.
func affected(res sql.Result) int64 {
	if res == nil {
		return 0
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0
	}
	return n
}

func itoa(n int) string { return strconv.Itoa(n) }
