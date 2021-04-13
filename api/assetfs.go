package api

import (
	"io/fs"
	"net/http"
	"os"

	frontend "github.com/ashb/slackarchive/frontend"
)

func AssetFS() *assetFS {
	subFS, err := fs.Sub(frontend.AssetFS, frontend.Prefix)
	if err != nil {
		panic(err)
	}
	return &assetFS{
		FileSystem: http.FS(subFS),
		DefaultDoc: "index.html",
	}
}

type assetFS struct {
	http.FileSystem

	DefaultDoc string
}

func (fs assetFS) Open(name string) (http.File, error) {
	f, err := fs.FileSystem.Open(name)
	if err == nil {
		return f, err
	} else if !os.IsNotExist(err) {
		return f, err
	}

	return fs.FileSystem.Open(fs.DefaultDoc)
}
