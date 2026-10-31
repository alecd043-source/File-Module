package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookieName = "heimdrive_session"
	sessionTTL        = 30 * 24 * time.Hour
	loginWindow       = 15 * time.Minute
	loginMaxFailures  = 8
)

type session struct {
	csrf    string
	expires time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]session)}
}

func (s *sessionStore) create() (string, session, error) {
	id, err := randomToken(32)
	if err != nil {
		return "", session{}, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return "", session{}, err
	}
	now := time.Now()
	sess := session{csrf: csrf, expires: now.Add(sessionTTL)}

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range s.sessions {
		if now.After(value.expires) {
			delete(s.sessions, key)
		}
	}
	s.sessions[id] = sess
	return id, sess, nil
}

func (s *sessionStore) get(id string) (session, bool) {
	if id == "" {
		return session{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return session{}, false
	}
	if time.Now().After(sess.expires) {
		delete(s.sessions, id)
		return session{}, false
	}
	return sess, true
}

func (s *sessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

type sessionContextKey struct{}

func (a *app) withAuth(next http.HandlerFunc, api bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := a.currentSession(r)
		if !ok {
			if api {
				writeAPIError(w, http.StatusUnauthorized, "Bitte erneut anmelden.")
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		if r.URL.Path != "/logout" && isMutating(r.Method) && !csrfValid(r.Header.Get("X-CSRF-Token"), sess.csrf) {
			writeAPIError(w, http.StatusForbidden, "CSRF-Token fehlt oder ist ungültig.")
			return
		}
		next(w, r.WithContext(contextWithSession(r, sess)))
	}
}

func (a *app) currentSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return session{}, false
	}
	return a.sessions.get(cookie.Value)
}

func contextWithSession(r *http.Request, sess session) context.Context {
	return context.WithValue(r.Context(), sessionContextKey{}, sess)
}

func sessionFromContext(r *http.Request) (session, bool) {
	sess, ok := r.Context().Value(sessionContextKey{}).(session)
	return sess, ok
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func csrfValid(got, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func secureEqual(got, want string) bool {
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

type loginRecord struct {
	failures int
	reset    time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginRecord
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginRecord)}
}

func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	rec, ok := l.attempts[ip]
	if !ok {
		return false
	}
	if time.Now().After(rec.reset) {
		delete(l.attempts, ip)
		return false
	}
	return rec.failures >= loginMaxFailures
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	rec := l.attempts[ip]
	if rec.reset.IsZero() || now.After(rec.reset) {
		rec = loginRecord{reset: now.Add(loginWindow)}
	}
	rec.failures++
	l.attempts[ip] = rec
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	delete(l.attempts, ip)
	l.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, ok := a.currentSession(r); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		a.renderLogin(w, "", http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		a.renderLogin(w, "Die Anfrage konnte nicht gelesen werden.", http.StatusBadRequest)
		return
	}

	ip := clientIP(r)
	if a.limiter.blocked(ip) {
		a.renderLogin(w, "Zu viele Fehlversuche. Bitte in einigen Minuten erneut versuchen.", http.StatusTooManyRequests)
		return
	}

	userOK := secureEqual(r.PostFormValue("username"), a.cfg.username)
	passOK := secureEqual(r.PostFormValue("password"), a.cfg.password)
	if !userOK || !passOK {
		a.limiter.fail(ip)
		time.Sleep(300 * time.Millisecond)
		a.renderLogin(w, "Benutzername oder Passwort ist falsch.", http.StatusUnauthorized)
		return
	}

	id, _, err := a.sessions.create()
	if err != nil {
		http.Error(w, "Anmeldung fehlgeschlagen", http.StatusInternalServerError)
		return
	}
	a.limiter.reset(ip)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL / time.Second),
		Expires:  time.Now().Add(sessionTTL),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *app) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültige Anfrage", http.StatusBadRequest)
		return
	}
	sess, ok := sessionFromContext(r)
	if !ok || !csrfValid(r.PostFormValue("_csrf"), sess.csrf) {
		http.Error(w, "CSRF-Token fehlt oder ist ungültig.", http.StatusForbidden)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.sessions.delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(1, 0),
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
