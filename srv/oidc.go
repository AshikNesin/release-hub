package srv

// Keyless publishing: verify short-lived OIDC identity tokens (GitHub
// Actions, or any OIDC issuer) against the issuer's published keys and mint
// an app-scoped release-hub token, so CI never stores a secret. Grants are registered
// per identity: issuer + subject → one app (srv/api_oidc.go, db migration 011).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"srv.exe.dev/db/dbgen"
)

// oidcAudience is the audience CI must request on its OIDC tokens.
const oidcAudience = "release-hub"

// exchangeTTL is how long a release-hub token minted by an OIDC exchange lives.
const exchangeTTL = time.Hour

// oidcLeeway is the clock-skew allowance for exp/nbf/iat.
const oidcLeeway = 2 * time.Minute

// oidcSigAlgs is the allow-list of JWT signature algorithms; GitHub signs
// with RS256. Restricting prevents algorithm confusion.
var oidcSigAlgs = []jose.SignatureAlgorithm{jose.RS256, jose.ES256}

// normalizeSubject completes a GitHub-style subject pasted without its ref
// part: GitHub's OIDC settings page shows `repo:owner@ID/name@ID`, but the
// token's subject also carries the ref (…:ref:refs/heads/main for a push to
// the default branch). A bare `repo:` prefix gains that suffix; anything
// already ref-like (":ref:", ":environment:", ":pull_request") is kept as-is.
func normalizeSubject(subject string) string {
	if !strings.HasPrefix(subject, "repo:") || strings.Contains(subject, ":ref:") ||
		strings.Contains(subject, ":environment:") || strings.Contains(subject, ":pull_request") {
		return subject
	}
	return subject + ":ref:refs/heads/main"
}

// discoverConfig fetches the issuer's OIDC discovery document and returns its
// jwks_uri. Only called for issuers that have a registered grant.
func discoverConfig(ctx context.Context, client *http.Client, issuer string) (string, error) {
	u := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: status %d", u, resp.StatusCode)
	}
	var cfg struct {
		Issuer  string `json:"issuer"`
		JWKSURL string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", u, err)
	}
	// The document must identify the issuer it was found under.
	if !sameURL(cfg.Issuer, issuer) {
		return "", fmt.Errorf("%s: discovery document is for issuer %q, want %q", u, cfg.Issuer, issuer)
	}
	if cfg.JWKSURL == "" {
		return "", fmt.Errorf("%s: discovery document has no jwks_uri", u)
	}
	return cfg.JWKSURL, nil
}

func sameURL(a, b string) bool {
	pa, err := url.Parse(strings.TrimSuffix(a, "/"))
	if err != nil {
		return false
	}
	pb, err := url.Parse(strings.TrimSuffix(b, "/"))
	return err == nil && pa.Scheme == pb.Scheme && pa.Host == pb.Host && pa.Path == pb.Path
}

// jwksCache caches fetched key sets by JWKS URL for a short TTL, so a burst
// of exchanges from one CI job doesn't refetch keys per request.
type jwksCache struct {
	mu    sync.Mutex
	entry map[string]*jwksEntry
}

type jwksEntry struct {
	set      *jose.JSONWebKeySet
	expires  time.Time
	pending  chan struct{} // closed when the fetch finishes; nil when idle
	fetchErr error
}

const jwksTTL = 15 * time.Minute

func (c *jwksCache) get(ctx context.Context, client *http.Client, jwksURL string) (*jose.JSONWebKeySet, error) {
	c.mu.Lock()
	if c.entry == nil {
		c.entry = map[string]*jwksEntry{}
	}
	e, ok := c.entry[jwksURL]
	if !ok {
		e = &jwksEntry{}
		c.entry[jwksURL] = e
	}
	if e.set != nil && time.Now().Before(e.expires) {
		set := e.set
		c.mu.Unlock()
		return set, nil
	}
	if e.pending == nil {
		done := make(chan struct{})
		e.pending = done
		go func() {
			set, err := fetchJWKS(context.WithoutCancel(ctx), client, jwksURL)
			c.mu.Lock()
			e.set, e.fetchErr = set, err
			if err == nil {
				e.expires = time.Now().Add(jwksTTL)
			} else {
				e.expires = time.Time{} // retry allowed on next call
			}
			e.pending = nil
			c.mu.Unlock()
			close(done)
		}()
	}
	done := e.pending
	c.mu.Unlock()

	select {
	case <-done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return e.set, e.fetchErr
}

func fetchJWKS(ctx context.Context, client *http.Client, jwksURL string) (*jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch jwks: status %d", resp.StatusCode)
	}
	var set jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}
	return &set, nil
}

// githubIssuer is GitHub Actions' OIDC issuer.
const githubIssuer = "https://token.actions.githubusercontent.com"

// oidcClaims is the claim set exchange cares about: the registered claims
// plus GitHub's repository claims, which repository grants match on.
type oidcClaims struct {
	jwt.Claims
	RepositoryID      flexID `json:"repository_id,omitempty"`
	RepositoryOwnerID flexID `json:"repository_owner_id,omitempty"`
	Ref               string `json:"ref,omitempty"`
}

// flexID is a numeric id GitHub sends as a JSON string ("123"); a bare
// number is accepted too. 0 means absent.
type flexID int64

func (f *flexID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("bad id %s", b)
	}
	*f = flexID(n)
	return nil
}

// verifyOIDCToken validates a raw identity token against the grant table and
// the issuer's keys, and returns the matching grant. The issuer of any
// incoming token must already carry a grant — unknown issuers are rejected
// before any network fetch, which also blocks SSRF via crafted iss claims.
//
// A token carrying a repository_id claim (GitHub) is matched against
// repository grants first: same issuer and repository id, then (after the
// signature check) the same owner id and ref. Otherwise, or when no
// repository grant exists, the issuer+subject grant applies as before.
func (s *Server) verifyOIDCToken(ctx context.Context, raw string) (dbgen.OidcGrantByIssuerSubjectRow, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 64<<10 {
		return dbgen.OidcGrantByIssuerSubjectRow{}, errors.New("missing token")
	}
	tok, err := jwt.ParseSigned(raw, oidcSigAlgs)
	if err != nil {
		return dbgen.OidcGrantByIssuerSubjectRow{}, fmt.Errorf("malformed token: %w", err)
	}
	var unsafe oidcClaims
	if err := tok.UnsafeClaimsWithoutVerification(&unsafe); err != nil {
		return dbgen.OidcGrantByIssuerSubjectRow{}, fmt.Errorf("malformed claims: %w", err)
	}
	iss := strings.TrimSpace(unsafe.Issuer)
	sub := strings.TrimSpace(unsafe.Subject)
	if iss == "" || sub == "" {
		return dbgen.OidcGrantByIssuerSubjectRow{}, errors.New("token has no issuer or subject")
	}

	// Gate on a registered grant before any fetch.
	q := dbgen.New(s.DB)
	var grant dbgen.OidcGrantByIssuerSubjectRow
	found := false
	if unsafe.RepositoryID > 0 {
		id := int64(unsafe.RepositoryID)
		if g, err := q.OidcGrantByIssuerRepository(ctx, dbgen.OidcGrantByIssuerRepositoryParams{
			Issuer: iss, RepositoryID: &id,
		}); err == nil {
			grant, found = dbgen.OidcGrantByIssuerSubjectRow(g), true
		}
	}
	if !found {
		g, err := q.OidcGrantByIssuerSubject(ctx, dbgen.OidcGrantByIssuerSubjectParams{Issuer: iss, Subject: sub})
		if err != nil {
			msg := fmt.Sprintf("no grant for issuer %s, subject %s", iss, sub)
			if unsafe.RepositoryID > 0 {
				msg += fmt.Sprintf(" (repository id %d)", int64(unsafe.RepositoryID))
			}
			return dbgen.OidcGrantByIssuerSubjectRow{}, errors.New(msg)
		}
		grant = g
	}

	jwksURL, err := discoverConfig(ctx, s.oidcHTTP, grant.Issuer)
	if err != nil {
		return grant, err
	}
	set, err := s.oidcJWKS.get(ctx, s.oidcHTTP, jwksURL)
	if err != nil {
		return grant, err
	}

	// Verify the signature: by key id when present, else try every key.
	keys := candidateKeys(set, keyID(tok))
	if len(keys) == 0 {
		return grant, errors.New("no matching signing key in the issuer's key set")
	}
	var claims oidcClaims
	verified := false
	for _, k := range keys {
		if err := tok.Claims(k, &claims); err == nil {
			verified = true
			break
		}
	}
	if !verified {
		return grant, errors.New("signature verification failed")
	}

	// Claims must match the grant and the release hub, with a little clock leeway.
	expected := jwt.Expected{Issuer: grant.Issuer, AnyAudience: jwt.Audience{oidcAudience}}
	if grant.RepositoryID == nil {
		expected.Subject = grant.Subject
	}
	if err := claims.ValidateWithLeeway(expected, oidcLeeway); err != nil {
		return grant, fmt.Errorf("claims rejected: %w", err)
	}
	if grant.RepositoryID != nil {
		if err := matchRepositoryGrant(grant, claims); err != nil {
			return grant, err
		}
	}
	return grant, nil
}

// matchRepositoryGrant checks a verified token's repository claims against a
// repository grant: same repository, same owner (a transfer to another
// account needs a new grant), and the granted ref when one is set.
func matchRepositoryGrant(grant dbgen.OidcGrantByIssuerSubjectRow, c oidcClaims) error {
	if int64(c.RepositoryID) != *grant.RepositoryID {
		return fmt.Errorf("claims rejected: repository id %d is not the granted %d", int64(c.RepositoryID), *grant.RepositoryID)
	}
	if grant.RepositoryOwnerID != nil && int64(c.RepositoryOwnerID) != *grant.RepositoryOwnerID {
		return fmt.Errorf("claims rejected: repository owner id %d is not the granted %d", int64(c.RepositoryOwnerID), *grant.RepositoryOwnerID)
	}
	if grant.Ref != "" && c.Ref != grant.Ref {
		return fmt.Errorf("claims rejected: ref %q may not publish; this grant allows %s", c.Ref, grant.Ref)
	}
	return nil
}

func keyID(tok *jwt.JSONWebToken) string {
	if len(tok.Headers) == 0 {
		return ""
	}
	return tok.Headers[0].KeyID
}

// candidateKeys returns verification keys for kid (or all keys when kid is
// empty/unknown), filtered to signature keys of the allowed algorithms.
func candidateKeys(set *jose.JSONWebKeySet, kid string) []jose.JSONWebKey {
	var keys []jose.JSONWebKey
	if kid != "" {
		keys = set.Key(kid)
	}
	if len(keys) == 0 {
		keys = set.Keys
	}
	out := keys[:0:0]
	for _, k := range keys {
		if !k.Valid() || k.Use == "enc" {
			continue
		}
		out = append(out, k)
	}
	return out
}
