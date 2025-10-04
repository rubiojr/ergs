package renderers

// Import all renderer packages to ensure they register themselves
// Note: Default renderer is not imported here - it's used as a fallback in the registry
import (
	_ "github.com/rubiojr/ergs/cmd/web/renderers/codeberg"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/firefox"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/gasstations"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/github"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/hackernews"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/rss"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/rtve"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/timestamp"
	_ "github.com/rubiojr/ergs/cmd/web/renderers/zedthreads"
)
