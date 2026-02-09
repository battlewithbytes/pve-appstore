package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/battlewithbytes/pve-appstore/internal/config"
)

const (
	sessionCookieName = "pve-appstore-session"
	sessionMaxAge     = 24 * time.Hour
)

// Short-lived tokens (for WebSocket terminal auth) still need in-memory storage
var (
	ephemeralTokens   = make(map[string]time.Time)
	ephemeralTokensMu sync.Mutex
)

// signToken creates an HMAC-signed token: "expiry_unix.signature"
// This survives server restarts since validation only needs the secret.
func signToken(secret string, expires time.Time) string {
	expStr := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(expStr))
	sig := hex.EncodeToString(mac.Sum(nil))
	return expStr + "." + sig
}

// verifyToken checks that the token is validly signed and not expired.
func verifyToken(secret, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > expUnix {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

// withAuth wraps a handler to require authentication (if auth is enabled).
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth.Mode == config.AuthModeNone {
			next(w, r)
			return
		}

		// Check cookie first, then fall back to query param (for WebSocket)
		var token string
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			token = cookie.Value
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		}

		if token == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}

		// Check HMAC-signed session token
		if verifyToken(s.cfg.Auth.PasswordHash, token) {
			next(w, r)
			return
		}

		// Check ephemeral tokens (for WebSocket terminal)
		ephemeralTokensMu.Lock()
		exp, ok := ephemeralTokens[token]
		if ok {
			if time.Now().After(exp) {
				delete(ephemeralTokens, token)
				ok = false
			} else {
				delete(ephemeralTokens, token) // one-time use
			}
		}
		ephemeralTokensMu.Unlock()

		if ok {
			next(w, r)
			return
		}

		writeError(w, http.StatusUnauthorized, "session expired")
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

	expires := time.Now().Add(sessionMaxAge)
	token := signToken(s.cfg.Auth.PasswordHash, expires)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleTerminalToken generates a short-lived one-time token for WebSocket auth.
func (s *Server) handleTerminalToken(w http.ResponseWriter, r *http.Request) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	token := hex.EncodeToString(tokenBytes)

	ephemeralTokensMu.Lock()
	ephemeralTokens[token] = time.Now().Add(30 * time.Second)
	ephemeralTokensMu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	authenticated := false
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		authenticated = verifyToken(s.cfg.Auth.PasswordHash, cookie.Value)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": authenticated,
		"auth_required": s.cfg.Auth.Mode == config.AuthModePassword,
	})
}

// cleanupEphemeralTokens removes expired ephemeral tokens periodically.
func cleanupEphemeralTokens() {
	for {
		time.Sleep(5 * time.Minute)
		now := time.Now()
		ephemeralTokensMu.Lock()
		for k, exp := range ephemeralTokens {
			if now.After(exp) {
				delete(ephemeralTokens, k)
			}
		}
		ephemeralTokensMu.Unlock()
	}
}

func init() {
	go cleanupEphemeralTokens()
}

// Ensure fmt is used
var _ = fmt.Sprint
