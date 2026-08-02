package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testDist(index string) fstest.MapFS {
	return fstest.MapFS{
		"index.html":            {Data: []byte(index)},
		"assets/index-abc.js":   {Data: []byte("console.log(1)")},
		"favicon.svg":           {Data: []byte("<svg/>")},
		"assets/index-abc.css":  {Data: []byte("body{}")},
		"nested/other/file.txt": {Data: []byte("x")},
	}
}

const indexHTML = `<!doctype html><html><head><base href="/" /><title>Headboard</title></head><body></body></html>`

func serve(t *testing.T, dist fstest.MapFS, basePath, target string) *httptest.ResponseRecorder {
	t.Helper()

	h, err := Handler(dist, "", basePath, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

	return rec
}

// The build ships relative asset URLs, so the <base href> is what decides
// whether they resolve. It has to be the deployment's path, and it has to be
// there on a deep client-side route too — that is the reload case.
func TestIndexCarriesTheDeploymentBase(t *testing.T) {
	for _, tc := range []struct {
		name, basePath, target, want string
	}{
		{"root", "", "/", `<base href="/"`},
		{"prefixed", "/manage", "/", `<base href="/manage/"`},
		{"prefixed deep route", "/manage", "/devices/12", `<base href="/manage/"`},
		{"nested prefix", "/a/b", "/acl", `<base href="/a/b/"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(t, testDist(indexHTML), tc.basePath, tc.target)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}

			if body := rec.Body.String(); !strings.Contains(body, tc.want) {
				t.Errorf("index does not contain %s:\n%s", tc.want, body)
			}
		})
	}
}

// A build without the placeholder would serve an app whose assets 404 under a
// prefix, and only under a prefix. Failing at construction turns that into a
// boot error somebody sees.
func TestAMissingBaseTagIsRefused(t *testing.T) {
	_, err := Handler(testDist(`<!doctype html><html><head></head></html>`), "", "/manage",
		slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("Handler accepted an index.html with no <base> tag")
	}
}

func TestAssetsAreServedAndCachedForever(t *testing.T) {
	rec := serve(t, testDist(indexHTML), "/manage", "/assets/index-abc.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	if string(body) != "console.log(1)" {
		t.Errorf("body = %q, want the asset", body)
	}

	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable", cc)
	}
}

// index.html itself must stay revalidated, or a deploy leaves browsers holding
// HTML that points at assets which no longer exist.
func TestIndexIsNotCached(t *testing.T) {
	rec := serve(t, testDist(indexHTML), "", "/")

	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
}

func TestAnUnbuiltUIReportsItself(t *testing.T) {
	h, err := Handler(fstest.MapFS{}, "", "", slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "pnpm build") {
		t.Errorf("body does not say how to fix it: %s", rec.Body.String())
	}
}
