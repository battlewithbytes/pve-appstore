package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/battlewithbytes/pve-appstore/internal/config"
)

const (
	sessionCookieName = "pve-appstore-session"
	sessionMaxAge     = 24 * time.Hour
)

type session struct {
	token   string
	expires time.Time
}

var (
	sessions   = make(map[string]*session)
	sessionsMu sync.Mutex
)

// withAuth wraps a handler to require authentication (if auth is enabled).
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth.Mode == config.AuthModeNone {
			next(w, r)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		sessionsMu.Lock()
		sess, ok := sessions[cookie.Value]
		if ok && time.Now().After(sess.expires) {
			delete(sessions, cookie.Value)
			ok = false
		}
		sessionsMu.Unlock()

		if !ok {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}

		next(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(s.cfg.Auth.PasswordHash), []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	// Generate session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate session")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	sessionsMu.Lock()
	sessions[token] = &session{
		token:   token,
		expires: time.Now().Add(sessionMaxAge),
	}
	sessionsMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		sessionsMu.Lock()
		delete(sessions, cookie.Value)
		sessionsMu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
			"auth_required": s.cfg.Auth.Mode == config.AuthModePassword,
		})
		return
	}

	sessionsMu.Lock()
	sess, ok := sessions[cookie.Value]
	valid := ok && time.Now().Before(sess.expires)
	sessionsMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": valid,
		"auth_required": s.cfg.Auth.Mode == config.AuthModePassword,
	})
}
