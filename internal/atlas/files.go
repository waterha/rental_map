package atlas

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Root prevents symlinks or traversals from escaping the public directory. Never
// serve the project root, database, environment files or directory indexes.
func servePublicFile(w http.ResponseWriter, r *http.Request, directory, name string) {
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case ".html", ".js", ".css", ".json", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", ".ico", ".woff", ".woff2", ".ttf":
	default:
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}
