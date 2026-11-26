//go:build embed

package webassets

import "embed"

//go:embed all:build
var Build embed.FS
