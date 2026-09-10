module github.com/rubiojr/ergs/importers/chromium-importer

go 1.27.1

require (
	github.com/ncruces/go-sqlite3 v0.35.4
	github.com/rubiojr/ergs v0.0.0
)

replace github.com/rubiojr/ergs => ../..

require (
	github.com/ncruces/go-sqlite3-wasm/v5 v5.0.35304 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)
