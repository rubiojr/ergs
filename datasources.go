package main

import (
	// Import all datasource modules to trigger their init() functions
	_ "github.com/rubiojr/ergs/pkg/datasources/codeberg"
	_ "github.com/rubiojr/ergs/pkg/datasources/firefox"
	_ "github.com/rubiojr/ergs/pkg/datasources/gasstations"
	_ "github.com/rubiojr/ergs/pkg/datasources/github"
	_ "github.com/rubiojr/ergs/pkg/datasources/hackernews"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp"
	_ "github.com/rubiojr/ergs/pkg/datasources/zedthreads"
)
