package renderers

// Import all renderer packages to ensure they register themselves
// Note: Default renderer is not imported here - it's used as a fallback in the registry
import (
	_ "github.com/rubiojr/ergs/pkg/datasources/chromium/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/codeberg/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/firefox/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/gasstations/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/github/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/hackernews/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/rss/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/rtve/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp/renderer"
	_ "github.com/rubiojr/ergs/pkg/datasources/zedthreads/renderer"
)
