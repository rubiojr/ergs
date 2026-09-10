package db

import (
	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

func init() {
	// FTS5 is a separate extension since go-sqlite3 v0.35. Register it for
	// every connection, including pooled connections and direct sql.Open calls.
	sqlite3.AutoExtension(fts5.Register)
}
