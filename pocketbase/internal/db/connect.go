// Package db wires PocketBase onto a SQLite build that has sqlite-vec
// compiled in, so vector search happens inside the same database and the same
// transaction as everything else.
package db

import (
	"fmt"
	"path/filepath"

	// Swaps the embedded SQLite WebAssembly build for one with sqlite-vec
	// statically linked. There is no registration call — importing this makes
	// vec0 available on every connection.
	//
	// Do NOT also import github.com/ncruces/go-sqlite3/embed: both try to
	// supply the SQLite binary.
	_ "github.com/asg017/sqlite-vec-go-bindings/ncruces"
	// Registers the database/sql driver named "sqlite3".
	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/pocketbase/dbx"
)

// Connect mirrors core.DefaultDBConnect from PocketBase v0.39.10, but opens
// through ncruces/go-sqlite3 instead of modernc.org/sqlite so that sqlite-vec
// is present.
//
// PocketBase calls this FOUR times per boot: a concurrent and a nonconcurrent
// handle for each of data.db and auxiliary.db. It is not possible to scope the
// override to just data.db, and it does not matter — the auxiliary database
// simply also runs on the vec-enabled build.
//
// Connection pool limits are deliberately NOT set here. PocketBase overwrites
// them immediately after this returns (nonconcurrent handles are forced to
// 1/1), so setting them would be a silent no-op that misleads whoever debugs
// pool exhaustion later.
func Connect(dbPath string) (*dbx.DB, error) {
	// busy_timeout has to come first: the connection must be set to block on
	// busy BEFORE WAL mode is set, in case another connection has not already
	// set it.
	const pragmas = "?_pragma=busy_timeout(10000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=journal_size_limit(200000000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=temp_store(MEMORY)" +
		"&_pragma=cache_size(-32000)"

	// Two things about this DSN are load-bearing:
	//
	//   1. The "file:" scheme. PocketBase's own form is `dbPath + "?_pragma=…"`
	//      with no scheme, which modernc parses and ncruces silently ignores.
	//      Getting this wrong yields a database with no WAL, no busy timeout
	//      and foreign keys OFF — and it looks like it worked.
	//   2. ToSlash. `file:C:\Users\…` is not a valid SQLite URI, so without
	//      this the whole thing fails on Windows only.
	dsn := "file:" + filepath.ToSlash(dbPath) + pragmas

	return dbx.Open("sqlite3", dsn)
}

// AssertVecAvailable fails fast if the database was opened without sqlite-vec.
//
// This is not paranoia. If a stock PocketBase binary — or one built without
// these bindings — ever opens the same pb_data, the app boots fine, serves
// collections, and every vector query fails with "no such module: vec0". The
// failure surfaces as "the assistant answered nothing useful", which is
// indistinguishable from a bad knowledge base. Check at startup instead.
// Takes a dbx.Builder rather than *dbx.DB so it can be called with
// core.App.DB(), which returns the interface.
func AssertVecAvailable(database dbx.Builder) error {
	var version string
	if err := database.NewQuery("SELECT vec_version()").Row(&version); err != nil {
		return fmt.Errorf(
			"sqlite-vec is not available in this build (%w); "+
				"the binary must be built from ./pocketbase with the vec bindings linked in, "+
				"and pb_data must not have been opened by a stock PocketBase binary", err)
	}
	return nil
}
