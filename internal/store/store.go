// Package store is the SQLite schema and the read-side queries shared by the
// web API, the MCP server and the ingesters. It reads and writes the same
// database file as the Python roles, so both can run against one archive.
package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Row is one result row keyed by column name. Responses are built from rows
// directly, so the JSON shape matches the Python API's dict(row) output.
type Row = map[string]any

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// Init creates the schema and applies the column migrations from db.py.
func (s *Store) Init() error {
	if _, err := s.DB.Exec(schema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	cols, err := s.columns("files")
	if err != nil {
		return err
	}
	for _, c := range [][2]string{
		{"remote_modified", "TEXT"}, {"submitters", "TEXT"},
		{"fts_id", "INTEGER"}, {"vorlage", "TEXT"},
		{"paper_type", "TEXT"}, {"paper_reference", "TEXT"},
		{"geo_status", "TEXT"}, {"district", "TEXT"}, {"district_conf", "REAL"},
	} {
		if !cols[c[0]] {
			if _, err := s.DB.Exec("ALTER TABLE files ADD COLUMN " + c[0] + " " + c[1]); err != nil {
				return err
			}
		}
	}
	for _, q := range []string{
		"UPDATE files SET fts_id = rowid WHERE fts_id IS NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_files_ftsid ON files(fts_id)",
		"CREATE INDEX IF NOT EXISTS idx_files_district ON files(district)",
	} {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}
	vcols, err := s.columns("votes")
	if err != nil {
		return err
	}
	if !vcols["members_ok"] {
		if _, err := s.DB.Exec("ALTER TABLE votes ADD COLUMN members_ok INTEGER"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) columns(table string) (map[string]bool, error) {
	rows, err := s.rows("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, r := range rows {
		out[str(r["name"])] = true
	}
	return out, nil
}

func (s *Store) rows(q string, args ...any) ([]Row, error) {
	rs, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	out := []Row{}
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := make(Row, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

func (s *Store) row(q string, args ...any) (Row, error) {
	rs, err := s.rows(q, args...)
	if err != nil || len(rs) == 0 {
		return nil, err
	}
	return rs[0], nil
}

func (s *Store) count(q string, args ...any) (int64, error) {
	var n int64
	err := s.DB.QueryRow(q, args...).Scan(&n)
	return n, err
}

// decodeJSON replaces the JSON-text columns topics/entities/submitters with
// their parsed value, leaving unparseable text as is.
func decodeJSON(r Row, keys ...string) Row {
	if len(keys) == 0 {
		keys = []string{"topics", "entities", "submitters"}
	}
	for _, k := range keys {
		s, ok := r[k].(string)
		if !ok || s == "" {
			continue
		}
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			r[k] = v
		}
	}
	return r
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}

// fold is Python's str.casefold for the characters that occur in names.
func fold(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "ß", "ss")
}

// where joins filter clauses into a WHERE suffix.
type where struct {
	clauses []string
	args    []any
}

func (w *where) add(clause string, args ...any) {
	w.clauses = append(w.clauses, clause)
	w.args = append(w.args, args...)
}

func (w *where) sql() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.clauses, " AND ")
}

func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-'
}

// FTSQuery quotes each token of a user query and marks it as a prefix, so
// user input can never be FTS5 syntax.
func FTSQuery(q string) string {
	toks := strings.FieldsFunc(q, func(r rune) bool { return !isTokenRune(r) })
	if len(toks) == 0 {
		return `""`
	}
	for i, t := range toks {
		toks[i] = `"` + t + `"*`
	}
	return strings.Join(toks, " ")
}
