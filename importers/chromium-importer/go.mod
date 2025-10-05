module github.com/rubiojr/ergs/importers/chromium-importer

go 1.24.2

require (
	github.com/ncruces/go-sqlite3 v0.27.1
	github.com/rubiojr/ergs v0.0.0
)

replace github.com/rubiojr/ergs => ../..

require (
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/tetratelabs/wazero v1.9.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
)
