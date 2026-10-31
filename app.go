package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed web
var embeddedWeb embed.FS

const pageCSP = "default-src 'self'; base-uri 'none'; connect-src 'self'; font-src 'self'; form-action 'self'; frame-ancestors 'none'; frame-src 'self'; img-src 'self' data: blob:; manifest-src 'self'; media-src 'self' blob:; object-src 'none'; script-src 'self'; style-src 'self'"

type app struct {
	cfg      config
	root     string
	web      fs.FS
	indexTpl *template.Template
	loginTpl *template.Template
	sessions *sessionStore
	limiter  *loginLimiter
	uploadMu sync.Mutex
}

type indexData struct {
	Name      string
	CSRF      string
	Version   string
	MaxUpload int64
}

type loginData struct {
	Name    string
	Error   string
	Version string
}

func newApp(cfg config) (*app, error) {
	root, err := filepath.EvalSymlinks(cfg.dataDir)
	if err != nil {
		return nil, err
	}
	web, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return nil, err
	}
	indexTpl, err := template.New("index.html").ParseFS(web, "index.html")
	if err != nil {
		return nil, err
	}
	loginTpl, err := template.New("login.html").ParseFS(web, "login.html")
	if err != nil {
		return nil, err
	}
	return &app{
		cfg:      cfg,
		root:     root,
		web:      web,
		indexTpl: indexTpl,
		loginTpl: loginTpl,
		sessions: newSessionStore(),
		limiter:  newLoginLimiter(),
	}, nil
}

func (a *app) Handler() http.Handler {
	return http.HandlerFunc(a.route)
}

func (a *app) route(w http.ResponseWriter, r *http.Request) {
	setBaseSecurityHeaders(w)

	switch {
	case r.URL.Path == "/healthz":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, "GET, HEAD")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, "ok\n")
	case r.URL.Path == "/login":
		a.handleLogin(w, r)
	case strings.HasPrefix(r.URL.Path, "/assets/"):
		a.handleAsset(w, r)
	case r.URL.Path == "/logout":
		a.withAuth(a.handleLogout, false)(w, r)
	case r.URL.Path == "/":
		a.withAuth(a.handleIndex, false)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/"):
		a.withAuth(a.handleAPI, true)(w, r)
	default:
		http.NotFound(w, r)
	}
}

func setBaseSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, "GET, HEAD")
		return
	}
	sess, ok := sessionFromContext(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = a.indexTpl.ExecuteTemplate(w, "index.html", indexData{
		Name:      a.cfg.name,
		CSRF:      sess.csrf,
		Version:   version,
		MaxUpload: a.cfg.maxUpload,
	})
}

func (a *app) renderLogin(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = a.loginTpl.ExecuteTemplate(w, "login.html", loginData{
		Name:    a.cfg.name,
		Error:   message,
		Version: version,
	})
}

func (a *app) handleAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, "GET, HEAD")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/assets/")
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(a.web, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", assetContentType(name))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func assetContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".webmanifest":
		return "application/manifest+json"
	case ".html":
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func (a *app) handleAPI(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/list":
		a.handleList(w, r)
	case "/api/upload":
		a.handleUpload(w, r)
	case "/api/download":
		a.handleDownload(w, r)
	case "/api/view":
		a.handleView(w, r)
	case "/api/mkdir":
		a.handleMkdir(w, r)
	case "/api/rename":
		a.handleRename(w, r)
	case "/api/delete":
		a.handleDelete(w, r)
	default:
		writeAPIError(w, http.StatusNotFound, "Unbekannter API-Endpunkt.")
	}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	allowed := method
	if method == http.MethodGet {
		allowed = "GET, HEAD"
	}
	if r.Method != method && !(method == http.MethodGet && r.Method == http.MethodHead) {
		methodNotAllowed(w, allowed)
		return false
	}
	return true
}

type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string { return e.message }

func newAPIError(status int, message string) error {
	return &apiError{status: status, message: message}
}

func writeRequestError(w http.ResponseWriter, err error) {
	var reqErr *apiError
	if errors.As(err, &reqErr) {
		writeAPIError(w, reqErr.status, reqErr.message)
		return
	}
	writeAPIError(w, http.StatusInternalServerError, "Interner Fehler.")
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return newAPIError(http.StatusBadRequest, "Ungültige Anfrage.")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return newAPIError(http.StatusBadRequest, "Ungültige Anfrage.")
	}
	return nil
}

func isHiddenName(name string) bool {
	return strings.HasPrefix(name, ".heimdrive-")
}
