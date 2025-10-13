package cmd

// This file ensures that all datasource-specific renderers are linked into the
// final binary so their init() functions run and register themselves with the
// global renderer registry. Without these blank imports, the web UI would fall
// back to the default renderer for all blocks (appearing "broken" or generic).
//
// When adding a new datasource renderer at:
//   pkg/datasources/<name>/renderer
// add a corresponding blank import below.
//
// NOTE: Keep this list sorted alphabetically for readability.

import (
	_ "github.com/rubiojr/ergs/pkg/datasources/chromium/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/codeberg/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/datadis/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/firefox/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/gasstations/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/github/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/hackernews/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/homeassistant/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/openmeteo/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/rss/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/rtve/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/zedthreads/renderer"
)
