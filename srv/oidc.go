package srv

// Keyless publishing: verify short-lived OIDC identity tokens (GitHub
// Actions, or any OIDC issuer) against the issuer's published keys and mint
// an app-scoped hub token, so CI never stores a secret. Grants are registered
// per identity: issuer + subject → one app (srv/api_oidc.go, db migration 011).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"srv.exe.dev/db/dbgen"
)

// oidcAudience is the audience CI must request on its OIDC tokens.
const oidcAudience = "release-hub"

// exchangeTTL is how long a hub token minted by an OIDC exchange lives.
const exchangeTTL = time.Hour

// oidcLeeway is the clock-skew allowance for exp/nbf/iat.
const oidcLeeway = 2 * time.Minute

// oidcSigAlgs is the allow-list of JWT signature algorithms; GitHub signs
// with RS256. Restricting prevents algorithm confusion.
var oidcSigAlgs = []jose.SignatureAlgorithm{jose.RS256, jose.ES256}

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

// oidcClaims is the claim set exchange cares about (jwt.Claims embedded).
type oidcClaims = jwt.Claims

// verifyOIDCToken validates a raw identity token against the grant table and
// the issuer's keys, and returns the matching grant. The issuer of any
// incoming token must already carry a grant — unknown issuers are rejected
// before any network fetch, which also blocks SSRF via crafted iss claims.
func (s *Server) verifyOIDCToken(ctx context.Context, raw string) (dbgen.OidcGrantByIssuerSubjectRow, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 64<<10 {
		return dbgen.OidcGrantByIssuerSubjectRow{}, errors.New("missing token")
	}
	tok, err := jwt.ParseSigned(raw, oidcSigAlgs)
	if err != nil {
		return dbgen.OidcGrantByIssuerSubjectRow{}, fmt.Errorf("malformed token: %w", err)
	}
	var unsafe jwt.Claims
	if err := tok.UnsafeClaimsWithoutVerification(&unsafe); err != nil {
		return dbgen.OidcGrantByIssuerSubjectRow{}, fmt.Errorf("malformed claims: %w", err)
	}
	iss := strings.TrimSpace(unsafe.Issuer)
	sub := strings.TrimSpace(unsafe.Subject)
	if iss == "" || sub == "" {
		return dbgen.OidcGrantByIssuerSubjectRow{}, errors.New("token has no issuer or subject")
	}

	// Gate on a registered grant before any fetch.
	grant, err := dbgen.New(s.DB).OidcGrantByIssuerSubject(ctx, dbgen.OidcGrantByIssuerSubjectParams{
		Issuer: iss, Subject: sub,
	})
	if err != nil {
		return dbgen.OidcGrantByIssuerSubjectRow{}, fmt.Errorf("no grant for issuer %s, subject %s", iss, sub)
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

	// Claims must match the grant and the hub, with a little clock leeway.
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:     grant.Issuer,
		Subject:    grant.Subject,
		AnyAudience: jwt.Audience{oidcAudience},
	}, oidcLeeway); err != nil {
		return grant, fmt.Errorf("claims rejected: %w", err)
	}
	return grant, nil
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
