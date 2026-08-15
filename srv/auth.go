package srv

// Auth: bearer-token API auth, cookie-session UI auth, token generation.
// Stored hashed (sha256) so a leaked DB does not leak credentials.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"srv.exe.dev/db/dbgen"
)

const sessionCookie = "rh_session"
const sessionTTL = 30 * 24 * time.Hour

// hashToken returns the hex sha256 of a raw secret.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}

func randomToken(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%x", prefix, b), nil
}

// validPassword checks the single admin password stored in config.
func (s *Server) validPassword(password string) bool {
	q := dbgen.New(s.DB)
	ctx := context.Background()
	hash, err := q.GetConfig(ctx, "admin_password_hash")
	if err != nil || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// requireUI is middleware for browser pages: session cookie → login redirect.
func (s *Server) requireUI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// requireAPI is middleware for programmatic endpoints: Bearer token → 401.
func (s *Server) requireAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="release-hub"`)
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, prefix)
		q := dbgen.New(s.DB)
		if _, err := q.ApiTokenByHash(r.Context(), hashToken(token)); err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		now := time.Now()
		_ = q.TouchApiToken(r.Context(), dbgen.TouchApiTokenParams{LastUsedAt: &now, TokenHash: hashToken(token)})
		next(w, r)
	}
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return false
	}
	q := dbgen.New(s.DB)
	sess, err := q.SessionByHash(r.Context(), hashToken(c.Value))
	if err != nil {
		return false
	}
	return time.Now().Before(sess.ExpiresAt)
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request) error {
	token, err := randomToken("sess")
	if err != nil {
		return err
	}
	q := dbgen.New(s.DB)
	now := time.Now()
	if err := q.CreateSession(r.Context(), dbgen.CreateSessionParams{TokenHash: hashToken(token), CreatedAt: now, ExpiresAt: now.Add(sessionTTL)}); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(s.baseURL, "https://"),
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}

func (s *Server) clearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = dbgen.New(s.DB).DeleteSession(r.Context(), hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}
