//go:build !embed

package main

import (
	"io/fs"
	"os"
)

func staticFiles() (fs.FS, error) {
	dir := os.Getenv("STATIC_DIR")
	if dir == "" {
		dir = "web/build"
	}
	return staticDirectory(dir), nil
}
