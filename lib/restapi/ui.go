package restapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:webui
var webUI embed.FS

func registerUI(mux *http.ServeMux) error {
	root, err := fs.Sub(webUI, "webui")
	if err != nil {
		return err
	}
	fileServer := http.FileServer(http.FS(root))
	mux.HandleFunc("GET /ui", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/ui/", http.StatusMovedPermanently)
	})
	mux.Handle("GET /ui/", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		http.StripPrefix("/ui/", fileServer).ServeHTTP(writer, request)
	}))
	return nil
}
