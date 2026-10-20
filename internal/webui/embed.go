// Package webui embeds and serves the compiled React single-page application.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var assets embed.FS

const notBuiltHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>IWMMD - frontend not built</title>
  <style>
    body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #0b0d10; color: #e7e9ee; font: 16px/1.5 system-ui, sans-serif; }
    main { max-width: 36rem; padding: 2rem; border: 1px solid #262b33; border-radius: 1rem; background: #11151a; }
    code { color: #9cc4ff; }
    h1 { margin-top: 0; font-size: 1.4rem; }
  </style>
</head>
<body>
  <main>
    <h1>IWMMD frontend is not built</h1>
    <p>Run <code>pnpm install &amp;&amp; pnpm build</code> in the <code>web</code> directory, then restart the server.</p>
    <p>The API is available at <code>/healthz</code> and <code>/api/v1/meta</code>.</p>
  </main>
</body>
</html>
`

// Handler serves embedded static files with SPA history fallback.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveFallback(w, r)
		})
	}
	index, indexErr := fs.ReadFile(sub, "index.html")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if rel == "" || rel == "." {
			serveIndex(w, r, index, indexErr)
			return
		}

		file, err := sub.Open(rel)
		if err == nil {
			info, statErr := file.Stat()
			_ = file.Close()
			if statErr == nil && !info.IsDir() && fs.ValidPath(rel) {
				if strings.HasPrefix(rel, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if strings.HasPrefix(rel, "assets/") {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, r, index, indexErr)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, index []byte, indexErr error) {
	if indexErr != nil {
		serveFallback(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(index)
}

func serveFallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(notBuiltHTML))
}
