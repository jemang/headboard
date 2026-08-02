// Package web serves the single-page application.
//
// In production it serves the build embedded in the binary. In development it
// reverse-proxies to the Vite dev server so hot module reload keeps working
// while the Go API is the single origin the browser talks to.
package web

import (
	"bytes"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

// Handler returns the SPA handler. When devProxy is non-empty, requests are
// proxied there instead of served from embedded assets.
//
// basePath is the path Headboard is served under, empty for a site root. The
// prefix is already stripped from the request by the time this handler runs;
// what it needs the value for is the <base href> written into index.html, which
// is how the browser resolves the build's relative asset URLs — and it must
// resolve them the same way from /manage as from /manage/devices/12.
func Handler(dist fs.FS, devProxy, basePath string, log *slog.Logger) (http.Handler, error) {
	if devProxy != "" {
		target, err := url.Parse(devProxy)
		if err != nil {
			return nil, err
		}

		log.Info("serving UI from Vite dev server", "target", target)

		return httputil.NewSingleHostReverseProxy(target), nil
	}

	index, err := indexWithBase(dist, basePath)
	if err != nil {
		return nil, err
	}

	return &spa{files: http.FS(dist), fsys: dist, index: index}, nil
}

// indexWithBase reads index.html once and rewrites its <base> tag.
//
// The build ships a placeholder `<base href="/">` that Vite passes through, so
// this is a substitution rather than an insertion — no HTML parsing. Only the
// href is rewritten, because the tag's exact spelling is the bundler's business
// (it currently emits the self-closing form). A build with no tag at all fails
// here rather than shipping an app whose assets 404, and only under a prefix.
func indexWithBase(dist fs.FS, basePath string) ([]byte, error) {
	raw, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		// Not fatal: the handler reports "UI not built" per request, which
		// is the existing behaviour and a better message than a boot crash.
		return nil, nil //nolint:nilnil // absent UI is handled at serve time
	}

	if !baseTag.Match(raw) {
		return nil, fmt.Errorf("ui/index.html has no <base href> tag to rewrite")
	}

	return baseTag.ReplaceAll(raw, []byte(`<base href="`+basePath+`/"`)), nil
}

var baseTag = regexp.MustCompile(`(?i)<base\s+href="[^"]*"`)

// spa serves static assets, falling back to index.html so client-side routes
// survive a page reload. The fallback is deliberately not applied to /api or
// /auth, which are mounted ahead of this handler.
type spa struct {
	files http.FileSystem
	fsys  fs.FS

	// index is index.html with its <base href> pointing at the deployment's
	// path, held in memory because every client-side route serves it.
	index []byte
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")

	// index.html always comes from serveIndex, never from the file system:
	// the copy on disk still carries the placeholder <base href>.
	if name == "" || name == "index.html" {
		s.serveIndex(w, r)

		return
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
	if len(s.index) == 0 {
		http.Error(w, "UI not built: run `pnpm build` in ui/", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(s.index))
}
