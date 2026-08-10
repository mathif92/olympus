// Command console is the Olympus web console: it serves the built React SPA
// and reverse-proxies the /api/<service>/* paths to each backend service so
// the browser talks to a single origin (no CORS changes on the backends).
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// service targets: name -> base URL env key + default.
type target struct {
	url string
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := getenv("CONSOLE_ADDR", ":8090")

	services := map[string]string{
		"amphora":    getenv("AMPHORA_URL", "http://localhost:8080"),
		"paramdora":  getenv("PARAMDORA_URL", "http://localhost:8083"),
		"hephaestus": getenv("HEPHAESTUS_URL", "http://localhost:8084"),
		"orpheus":    getenv("ORPHEUS_URL", "http://localhost:8086"),
		"clio":       getenv("CLIO_URL", "http://localhost:8087"),
		"mneme":      getenv("MNEME_URL", "http://localhost:8088"),
		"iris":       getenv("IRIS_URL", "http://localhost:8089"),
	}

	proxies := make(map[string]*httputil.ReverseProxy, len(services))
	for name, base := range services {
		u, err := url.Parse(base)
		if err != nil {
			log.Fatalf("invalid base URL for %s (%s): %v", name, base, err)
		}
		proxies[name] = newServiceProxy(name, u)
		log.Printf("proxy /api/%s/* -> %s", name, base)
	}

	uiDir := getenv("CONSOLE_UI_DIR", filepath.Join("web", "console"))
	mux := http.NewServeMux()

	mux.Handle("/api/health", healthHandler(proxies))
	mux.Handle("/api/", serviceProxyHandler(proxies))
	mux.Handle("/", spaHandler(uiDir))

	log.Printf("🖥️  Olympus console running on %s (UI: %s)...", addr, uiDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Could not start server: %v", err)
	}
}

// newServiceProxy strips the /api/<service>/ prefix and forwards the rest of
// the path to the backend, preserving headers, query, body and response.
func newServiceProxy(name string, target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		prefix := "/api/" + name + "/"
		if p := r.URL.Path; strings.HasPrefix(p, prefix) {
			r.URL.Path = strings.TrimPrefix(p, prefix)
		} else if p == "/api/"+name {
			r.URL.Path = "/"
		}
		r.URL.RawPath = ""
		originalDirector(r)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[console] %s %s -> %s error: %v", r.Method, r.URL.Path, name, err)
		http.Error(w, "console: upstream "+name+" unreachable: "+err.Error(), http.StatusBadGateway)
	}
	return proxy
}

// serviceProxyHandler routes /api/<service>/... and /api/<service> to the
// matching reverse proxy, returning 404 for unknown service names.
func serviceProxyHandler(proxies map[string]*httputil.ReverseProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/")
		name := rest
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[:i]
		}
		proxy, ok := proxies[name]
		if !ok {
			http.Error(w, "console: unknown service "+name, http.StatusNotFound)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}

// healthHandler pings every backend's /health concurrently and reports the
// aggregated status, so the console dashboard can show per-service health.
func healthHandler(proxies map[string]*httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type result struct {
			name string
			ok   bool
		}
		results := make(chan result, len(proxies))
		var wg sync.WaitGroup
		for name, proxy := range proxies {
			wg.Add(1)
			go func(name string, proxy *httputil.ReverseProxy) {
				defer wg.Done()
				probe := r.Clone(r.Context())
				probe.Method = http.MethodGet
				probe.URL.Path = "/health"
				probe.URL.RawQuery = ""
				rr := &recorder{header: make(http.Header)}
				proxy.ServeHTTP(rr, probe)
				results <- result{name: name, ok: rr.status == http.StatusOK}
			}(name, proxy)
		}
		wg.Wait()
		close(results)

		out := struct {
			Status   string            `json:"status"`
			Services map[string]string `json:"services"`
		}{Status: "healthy", Services: make(map[string]string)}
		for res := range results {
			status := "ok"
			if !res.ok {
				status = "down"
				out.Status = "degraded"
			}
			out.Services[res.name] = status
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":%q,"services":{%s}}`, out.Status, statusMapJSON(out.Services))
	}
}

func statusMapJSON(m map[string]string) string {
	var parts []string
	for name, status := range m {
		parts = append(parts, fmt.Sprintf("%q:%q", name, status))
	}
	return strings.Join(parts, ",")
}

type recorder struct {
	header http.Header
	status int
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}
func (r *recorder) Write(b []byte) (int, error) { return len(b), nil }

// spaHandler serves the built SPA from disk with client-side routing: unknown
// non-/api GET paths fall back to index.html so React Router takes over.
func spaHandler(uiDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(uiDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := filepath.Join(uiDir, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(uiDir, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.Error(w, "console: UI not built (run `npm run build` in web/)", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, index)
	})
}
