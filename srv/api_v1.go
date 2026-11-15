package srv

// /api/v1 — the JSON API the React UI (web/) talks to.
//
// It speaks the groot shell's wire contract so the vendored shell code
// (web/vendor/groot/shell: API client, auth store, layout) works unchanged:
//
//	2xx → {"success": true, "data": <payload>}
//	4xx/5xx → {"success": false, "error": {"code": "...", "message": "..."}}
//	401 from the session guard carries code SESSION_INVALID, which signs the
//	client out.
//
// Auth is the browser session cookie (rh_session), never a bearer token —
// the bearer API under /api/ stays exactly as CI/devices use it. Many v1
// routes reuse those bearer handlers: v1() adapts their legacy
// {"error": "..."} / bare-JSON responses into the envelope, and accepts JSON
// request bodies by mapping them onto r.Form (the handlers read FormValue).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"srv.exe.dev/db/dbgen"
)

// sessionUser is the shell's SessionUser wire type. release-hub has a single
// admin, so it is a constant.
type sessionUser struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

var adminUser = sessionUser{ID: 1, Email: "admin", Name: "Admin"}

// v1Routes registers every /api/v1 route on mux.
func (s *Server) v1Routes(mux *http.ServeMux) {
	// Auth (no session required).
	mux.HandleFunc("GET /api/v1/auth/setup", s.v1Public(s.handleV1SetupStatus))
	mux.HandleFunc("POST /api/v1/auth/setup", s.v1Public(s.handleV1Setup))
	mux.HandleFunc("POST /api/v1/auth/login", s.v1Public(s.handleV1Login))
	mux.HandleFunc("POST /api/v1/auth/logout", s.v1Public(s.handleV1Logout))
	mux.HandleFunc("GET /api/v1/auth/me", s.v1(s.handleV1Me))

	// Apps and platforms.
	mux.HandleFunc("GET /api/v1/apps", s.v1(s.handleV1ListApps))
	mux.HandleFunc("POST /api/v1/apps", s.v1(s.handleApiCreateApp))
	mux.HandleFunc("DELETE /api/v1/apps/{slug}", s.v1(s.handleApiDeleteApp))
	mux.HandleFunc("GET /api/v1/apps/{slug}", s.v1(s.handleV1AppDetail))
	mux.HandleFunc("POST /api/v1/apps/{slug}/platforms", s.v1(s.handleApiAddPlatform))
	mux.HandleFunc("DELETE /api/v1/apps/{slug}/{platform}/releases/{versionCode}", s.v1(s.handleApiDeleteRelease))
	mux.HandleFunc("PUT /api/v1/apps/{slug}/{platform}/retention", s.v1(s.handleV1Retention))
	mux.HandleFunc("POST /api/v1/apps/{slug}/{platform}/play", s.v1(s.handleApiSetPlay))
	mux.HandleFunc("GET /api/v1/apps/{slug}/{platform}/play/preflight", s.v1(s.handleApiPlayPreflight))
	mux.HandleFunc("GET /api/v1/apps/{slug}/{platform}/tracks", s.v1(s.handleApiListTracks))
	mux.HandleFunc("POST /api/v1/apps/{slug}/{platform}/testers", s.v1(s.handleApiInviteTesters))

	// Release hub settings, tokens, shared Play accounts.
	mux.HandleFunc("GET /api/v1/settings", s.v1(s.handleV1GetSettings))
	mux.HandleFunc("PATCH /api/v1/settings", s.v1(s.handleV1PatchSettings))
	mux.HandleFunc("GET /api/v1/tokens", s.v1(s.handleV1ListTokens))
	mux.HandleFunc("POST /api/v1/tokens", s.v1(s.handleApiCreateToken))
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", s.v1(s.handleV1DeleteToken))

	// Keyless publishing: OIDC grants per app.
	mux.HandleFunc("GET /api/v1/apps/{slug}/oidc/grants", s.v1(s.handleV1ListOidcGrants))
	mux.HandleFunc("POST /api/v1/apps/{slug}/oidc/grants", s.v1(s.handleV1CreateOidcGrant))
	mux.HandleFunc("DELETE /api/v1/oidc/grants/{id}", s.v1(s.handleV1DeleteOidcGrant))
	mux.HandleFunc("GET /api/v1/play-accounts", s.v1(s.handleV1ListPlayAccounts))
	mux.HandleFunc("POST /api/v1/play-accounts", s.v1(s.handleApiPlayAccounts))
	mux.HandleFunc("DELETE /api/v1/play-accounts/{id}", s.v1(s.handleV1DeletePlayAccount))

	// Anything else under /api/v1 is a JSON 404, never the SPA shell.
	mux.HandleFunc("/api/v1/", s.v1Public(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "no such endpoint: "+r.Method+" "+r.URL.Path)
	}))
}

// v1 guards a session-only route: same-origin check, session cookie, JSON
// body mapping and the response envelope.
func (s *Server) v1(next http.HandlerFunc) http.HandlerFunc {
	return s.v1Public(func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			writeEnvelopeErr(w, http.StatusUnauthorized, "SESSION_INVALID", "Your session has expired. Sign in again.")
			return
		}
		next(w, r)
	})
}

// v1Public is v1 without the session requirement (sign-in routes).
func (s *Server) v1Public(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			writeEnvelopeErr(w, http.StatusForbidden, "FORBIDDEN", "cross-site request refused")
			return
		}
		if err := jsonBodyToForm(r); err != nil {
			writeEnvelopeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		cw := &captureWriter{header: http.Header{}}
		next(cw, r)
		cw.flushEnvelope(w)
	}
}

// sameOrigin rejects state-changing requests that a browser marks as
// cross-site. The session cookie is SameSite=Lax already; this is the second
// layer (CSRF via top-level navigation, older browsers).
func sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	}
	return true // non-browser client: no ambient cookie to abuse
}

// jsonBodyToForm maps a flat JSON object body onto r.Form / r.PostForm so the
// shared handlers can keep reading FormValue. Multipart and urlencoded bodies
// are left for the handler to parse.
func jsonBodyToForm(r *http.Request) error {
	if r.Body == nil || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	vals := url.Values{}
	if len(bytes.TrimSpace(raw)) > 0 {
		var obj map[string]any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&obj); err != nil {
			return fmt.Errorf("body must be a JSON object")
		}
		for k, v := range obj {
			switch t := v.(type) {
			case nil:
				vals.Set(k, "")
			case string:
				vals.Set(k, t)
			case json.Number:
				vals.Set(k, t.String())
			case bool:
				vals.Set(k, strconv.FormatBool(t))
			default:
				return fmt.Errorf("field %q must be a string, number or boolean", k)
			}
		}
	}
	for k, vs := range r.URL.Query() {
		if !vals.Has(k) {
			vals[k] = vs
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(nil))
	r.Form, r.PostForm = vals, vals
	return nil
}

// captureWriter buffers a handler's response so v1 can re-wrap it.
type captureWriter struct {
	header http.Header
	status int
	buf    bytes.Buffer
}

func (c *captureWriter) Header() http.Header { return c.header }
func (c *captureWriter) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
	}
}
func (c *captureWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.buf.Write(b)
}

func (c *captureWriter) flushEnvelope(w http.ResponseWriter) {
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	for k, vs := range c.header {
		switch http.CanonicalHeaderKey(k) {
		case "Content-Type", "Content-Length", "X-Content-Type-Options":
			continue
		}
		w.Header()[k] = vs
	}
	body := bytes.TrimSpace(c.buf.Bytes())

	if status >= 400 {
		// Already an envelope (writeEnvelopeErr inside a handler).
		var env struct {
			Success *bool           `json:"success"`
			Error   json.RawMessage `json:"error"`
		}
		if json.Unmarshal(body, &env) == nil && env.Success != nil {
			writeRaw(w, status, body)
			return
		}
		msg := string(body)
		var legacy apiError
		if json.Unmarshal(body, &legacy) == nil && legacy.Error != "" {
			msg = legacy.Error
		}
		if msg == "" {
			msg = http.StatusText(status)
		}
		writeEnvelopeErr(w, status, errorCode(status), msg)
		return
	}

	if len(body) == 0 {
		writeRaw(w, status, []byte(`{"success":true}`))
		return
	}
	if !json.Valid(body) {
		body, _ = json.Marshal(string(body))
	}
	out := make([]byte, 0, len(body)+32)
	out = append(out, `{"success":true,"data":`...)
	out = append(out, body...)
	out = append(out, '}')
	writeRaw(w, status, out)
}

func writeRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}

func writeEnvelopeErr(w http.ResponseWriter, status int, code, msg string) {
	b, _ := json.Marshal(map[string]any{
		"success": false,
		"error":   map[string]string{"code": code, "message": msg},
	})
	writeRaw(w, status, b)
}

func errorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "VALIDATION_ERROR"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusBadGateway:
		return "UPSTREAM_ERROR"
	}
	if status >= 500 {
		return "INTERNAL_ERROR"
	}
	return fmt.Sprintf("HTTP_%d", status)
}

// ---- auth ----

// handleV1SetupStatus GET /api/v1/auth/setup → {needsSetup}
func (s *Server) handleV1SetupStatus(w http.ResponseWriter, r *http.Request) {
	set, err := s.hasPassword()
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	writeJSON(w, 200, map[string]bool{"needsSetup": !set})
}

// handleV1Setup POST /api/v1/auth/setup {password} — first run only: sets the
// admin password and signs in.
func (s *Server) handleV1Setup(w http.ResponseWriter, r *http.Request) {
	set, err := s.hasPassword()
	if err != nil {
		writeErr(w, 500, "db error")
		return
	}
	if set {
		writeErr(w, 403, "setup is already complete; sign in instead")
		return
	}
	pw := r.FormValue("password")
	if len(pw) < 8 {
		writeErr(w, 400, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err == nil {
		err = dbgen.New(s.DB).SetConfig(r.Context(), dbgen.SetConfigParams{
			Key: "admin_password_hash", Value: string(hash),
		})
	}
	if err != nil {
		writeErr(w, 500, "failed to save: "+err.Error())
		return
	}
	if err := s.setSession(w, r); err != nil {
		writeErr(w, 500, "session error")
		return
	}
	writeJSON(w, 201, map[string]any{"user": adminUser})
}

// handleV1Login POST /api/v1/auth/login {password}
func (s *Server) handleV1Login(w http.ResponseWriter, r *http.Request) {
	if !s.validPassword(r.FormValue("password")) {
		writeEnvelopeErr(w, 401, "INVALID_CREDENTIALS", "Wrong password.")
		return
	}
	if err := s.setSession(w, r); err != nil {
		writeErr(w, 500, "session error")
		return
	}
	writeJSON(w, 200, map[string]any{"user": adminUser})
}

// handleV1Logout POST /api/v1/auth/logout — always succeeds.
func (s *Server) handleV1Logout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleV1Me GET /api/v1/auth/me — the session user (v1 guard already 401s).
func (s *Server) handleV1Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, adminUser)
}
