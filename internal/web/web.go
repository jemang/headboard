// Package web serves the single-page application.
//
// In production it serves the build embedded in the binary. In development it
// reverse-proxies to the Vite dev server so hot module reload keeps working
// while the Go API is the single origin the browser talks to.
package web

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
)

// Handler returns the SPA handler. When devProxy is non-empty, requests are
// proxied there instead of served from embedded assets.
func Handler(dist fs.FS, devProxy string, log *slog.Logger) (http.Handler, error) {
	if devProxy != "" {
		target, err := url.Parse(devProxy)
		if err != nil {
			return nil, err
		}

		log.Info("serving UI from Vite dev server", "target", target)

		return httputil.NewSingleHostReverseProxy(target), nil
	}

	return &spa{files: http.FS(dist), fsys: dist}, nil
}

// spa serves static assets, falling back to index.html so client-side routes
// survive a page reload. The fallback is deliberately not applied to /api or
// /auth, which are mounted ahead of this handler.
type spa struct {
	files http.FileSystem
	fsys  fs.FS
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}

	f, err := s.fsys.Open(name)
	if err != nil {
		// Unknown path: hand it to the client router.
		s.serveIndex(w, r)

		return
	}

	stat, err := f.Stat()
	_ = f.Close()

	if err != nil || stat.IsDir() {
		s.serveIndex(w, r)

		return
	}

	// Vite emits content-hashed filenames under /assets, so those are safe to
	// cache indefinitely. Everything else must be revalidated or a deploy
	// would leave stale HTML pointing at assets that no longer exist.
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	http.FileServer(s.files).ServeHTTP(w, r)
}

func (s *spa) serveIndex(w http.ResponseWriter, r *http.Request) {
	index, err := s.fsys.Open("index.html")
	if err != nil {
		http.Error(w, "UI not built: run `pnpm build` in ui/", http.StatusInternalServerError)

		return
	}
	defer index.Close()

	stat, err := index.Stat()
	if err != nil {
		http.Error(w, "UI not built: run `pnpm build` in ui/", http.StatusInternalServerError)

		return
	}

	rs, ok := index.(interface {
		Seek(offset int64, whence int) (int64, error)
		Read(p []byte) (int, error)
	})
	if !ok {
		http.Error(w, "UI not built: run `pnpm build` in ui/", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", stat.ModTime(), rs)
}
