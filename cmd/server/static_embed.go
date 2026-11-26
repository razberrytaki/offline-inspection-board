//go:build embed

package main

import (
	"io/fs"

	webassets "offline-inspection-board/web"
)

func staticFiles() (fs.FS, error) {
	return fs.Sub(webassets.Build, "build")
}
