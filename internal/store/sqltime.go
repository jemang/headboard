package store

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// timeFormat is RFC3339 with milliseconds, in UTC, matching the
// strftime('%Y-%m-%dT%H:%M:%fZ','now') defaults in the schema.
//
// Text rather than a numeric epoch so the database is readable with the sqlite3
// CLI, and UTC rather than local so it sorts lexicographically — which is what
// makes ORDER BY created_at DESC correct.
const timeFormat = "2006-01-02T15:04:05.000Z"

// sqlTime converts between time.Time and the schema's text representation.
//
// Explicit rather than relying on the driver to recognise a column as a date:
// modernc's conversion depends on the declared column type, and a silent
// mismatch would surface as a zero timestamp in the audit log rather than as an
// error.
type sqlTime struct{ t *time.Time }

// intoTime returns a Scanner writing into dst.
func intoTime(dst *time.Time) sqlTime { return sqlTime{t: dst} }

func (s sqlTime) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*s.t = time.Time{}

		return nil
	case time.Time:
		*s.t = v.UTC()

		return nil
	case []byte:
		return s.parse(string(v))
	case string:
		return s.parse(v)
	default:
		return fmt.Errorf("cannot read %T as a time", src)
	}
}

func (s sqlTime) parse(v string) error {
	if v == "" {
		*s.t = time.Time{}

		return nil
	}

	// Written by Go through fromTime, or by the schema default. Both are
	// covered by the first layout; the others are what the sqlite3 CLI or a
	// hand-written INSERT would produce.
	for _, layout := range []string{timeFormat, time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, v); err == nil {
			*s.t = t.UTC()

			return nil
		}
	}

	return fmt.Errorf("cannot parse %q as a time", v)
}

// nullTime scans a nullable timestamp into **time.Time.
type nullTime struct{ t **time.Time }

func intoNullTime(dst **time.Time) nullTime { return nullTime{t: dst} }

func (n nullTime) Scan(src any) error {
	if src == nil {
		*n.t = nil

		return nil
	}

	var t time.Time
	if err := (sqlTime{t: &t}).Scan(src); err != nil {
		return err
	}

	if t.IsZero() {
		*n.t = nil

		return nil
	}

	*n.t = &t

	return nil
}

// fromTime renders a time for storage.
func fromTime(t time.Time) driver.Value { return t.UTC().Format(timeFormat) }

// nowText is the current time in the schema's format, for columns written by Go
// rather than by a DEFAULT.
func nowText() string { return time.Now().UTC().Format(timeFormat) }
