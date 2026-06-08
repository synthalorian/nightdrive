package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var static embed.FS

// Register mounts the static SPA handler.
func Register(mux *http.ServeMux) {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
}
