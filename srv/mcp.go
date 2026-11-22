package srv

// MCP: a Model Context Protocol server at /mcp, so an AI client (Claude
// Code, Claude Desktop, Cursor, …) can drive the release hub the way the web
// app does: register apps, add platforms, read releases and download links,
// wire Play, manage tokens and keyless grants.
//
// Transport is MCP "Streamable HTTP" in its stateless form: every JSON-RPC
// request is one POST answered with one application/json response. There is
// no session id and no server-to-client stream (GET answers 405, which the
// spec allows), because every tool is a plain request/response call.
//
// Auth is the bearer API's: Authorization: Bearer rh_... through requireAPI,
// no OAuth. OIDC-minted tokens are scoped to one app's publish endpoints and
// are refused here, so only a handmade token can use MCP.
//
// Tools reuse the existing handlers instead of re-implementing them: each
// call builds an in-process request (path values + form fields), runs the
// same handler the bearer API or the UI's /api/v1 uses, and turns its JSON
// reply into the tool result. Behaviour, validation and error messages stay
// identical across the UI, the API and MCP.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"

	"srv.exe.dev/db/dbgen"
)

// mcpProtocolVersions are the MCP revisions this server speaks, newest
// first. The stateless tools-only subset used here is identical across them.
var mcpProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const mcpMaxBody = 1 << 20

const mcpInstructions = `Release Hub distributes Android/iOS builds. An app is a product slug; each
platform (android, ios) has its own package name, signing key, releases and
optional Google Play publishing. Typical flow: create_app → add_platform →
CI uploads builds → get_download_links to share. Builds are uploaded by CI
(POST /api/apps/{slug}/{platform}/releases), not through MCP. Platform
defaults to android wherever it is optional.`

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// handleMCP POST /mcp (bearer auth via requireAPI).
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "this MCP endpoint is stateless: POST JSON-RPC requests only", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, mcpMaxBody+1))
	if err != nil || len(raw) > mcpMaxBody {
		writeRPC(w, jsonrpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &jsonrpcError{-32700, "request body unreadable or over 1MB"}})
		return
	}
	var req jsonrpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeRPC(w, jsonrpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &jsonrpcError{-32700, "parse error: body must be one JSON-RPC message"}})
		return
	}
	if len(req.ID) == 0 || req.Method == "" {
		// A notification (notifications/initialized, …) or a client's
		// response: nothing to answer.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	resp := jsonrpcResponse{JSONRPC: "2.0", ID: req.ID}
	result, rpcErr := s.mcpDispatch(r.Context(), req)
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	writeRPC(w, resp)
}

func writeRPC(w http.ResponseWriter, resp jsonrpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) mcpDispatch(ctx context.Context, req jsonrpcRequest) (any, *jsonrpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := mcpProtocolVersions[0]
		for _, v := range mcpProtocolVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "release-hub", "title": "Release Hub", "version": buildVersion()},
			"instructions":    mcpInstructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		defs := make([]map[string]any, 0, len(mcpTools))
		for _, t := range mcpTools {
			defs = append(defs, t.definition())
		}
		return map[string]any{"tools": defs}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &jsonrpcError{-32602, "invalid params: " + err.Error()}
		}
		tool, ok := mcpToolByName(p.Name)
		if !ok {
			return nil, &jsonrpcError{-32602, "unknown tool: " + p.Name}
		}
		args := mcpArgs(p.Arguments)
		for _, name := range tool.required {
			if args.str(name) == "" {
				return toolError(fmt.Sprintf("%s is required", name)), nil
			}
		}
		out, err := tool.run(ctx, s, args)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return toolResult(out), nil
	}
	return nil, &jsonrpcError{-32601, "method not found: " + req.Method}
}

func buildVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, st := range bi.Settings {
			if st.Key == "vcs.revision" && len(st.Value) >= 7 {
				return st.Value[:7]
			}
		}
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

// toolResult renders a tool's output as pretty JSON text, plus
// structuredContent when it is a JSON object (the spec requires an object).
func toolResult(out any) map[string]any {
	var b []byte
	switch v := out.(type) {
	case json.RawMessage:
		var buf bytes.Buffer
		if json.Indent(&buf, v, "", "  ") == nil {
			b = buf.Bytes()
		} else {
			b = v
		}
	default:
		b, _ = json.MarshalIndent(v, "", "  ")
	}
	res := map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}}
	var obj map[string]any
	if json.Unmarshal(b, &obj) == nil && obj != nil {
		res["structuredContent"] = obj
	}
	return res
}

func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}

// ---- arguments ----

type mcpArgs map[string]any

// str returns an argument as the string a form field would carry.
func (a mcpArgs) str(name string) string {
	switch v := a[name].(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func (a mcpArgs) has(name string) bool { _, ok := a[name]; return ok }

// form copies the named arguments that are present into form values.
func (a mcpArgs) form(names ...string) url.Values {
	v := url.Values{}
	for _, n := range names {
		if a.has(n) && a[n] != nil {
			v.Set(n, a.str(n))
		}
	}
	return v
}

// path picks the named arguments that are set as path values.
func (a mcpArgs) path(names ...string) map[string]string {
	p := map[string]string{}
	for _, n := range names {
		if v := a.str(n); v != "" {
			p[n] = v
		}
	}
	return p
}

// ---- running the existing handlers in-process ----

// callHandler runs h on an in-process request carrying path values and form
// fields (or a multipart body), and returns its JSON reply. A 4xx/5xx reply
// becomes an error with the handler's own message.
func (s *Server) callHandler(ctx context.Context, method string, h http.HandlerFunc, path map[string]string, form url.Values, multipartBody *multipartPayload) (json.RawMessage, error) {
	var body io.Reader = http.NoBody
	if multipartBody != nil {
		body = bytes.NewReader(multipartBody.body)
	}
	r, err := http.NewRequestWithContext(ctx, method, "/mcp", body)
	if err != nil {
		return nil, err
	}
	for k, v := range path {
		r.SetPathValue(k, v)
	}
	if multipartBody != nil {
		r.Header.Set("Content-Type", multipartBody.contentType)
	} else {
		if form == nil {
			form = url.Values{}
		}
		r.Form, r.PostForm = form, form
	}
	cw := &captureWriter{header: http.Header{}}
	h(cw, r)
	status := cw.status
	if status == 0 {
		status = http.StatusOK
	}
	out := bytes.TrimSpace(cw.buf.Bytes())
	if status >= 400 {
		msg := string(out)
		var legacy apiError
		if json.Unmarshal(out, &legacy) == nil && legacy.Error != "" {
			msg = legacy.Error
		}
		if msg == "" {
			msg = http.StatusText(status)
		}
		return nil, errors.New(msg)
	}
	if len(out) == 0 || !json.Valid(out) {
		b, _ := json.Marshal(map[string]any{"ok": true})
		return b, nil
	}
	return json.RawMessage(out), nil
}

type multipartPayload struct {
	body        []byte
	contentType string
}

// fileUpload builds a multipart body with one file field (what
// upsertPlayAccountFromRequest reads).
func fileUpload(field, filename string, content []byte) (*multipartPayload, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(content); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return &multipartPayload{body: buf.Bytes(), contentType: mw.FormDataContentType()}, nil
}

// ---- download links ----

// shareLink is one stable, shareable download URL of a platform.
type shareLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// shareLinks returns the stable /apps/{slug}/download links for a platform:
// direct always, the Play channels when Play publishing is on. The app page
// shows the same list.
func (s *Server) shareLinks(slug string, p dbgen.AppPlatform) []shareLink {
	dl := s.baseURL + "/apps/" + slug + "/download"
	if p.Platform != "android" {
		dl += "/" + p.Platform
	}
	dl += "?channel="
	links := []shareLink{{"direct", dl + "direct"}}
	if p.PlayEnabled == 1 {
		links = append(links, shareLink{"internal", dl + "internal"}, shareLink{"beta", dl + "beta"}, shareLink{"public", dl + "public"})
	}
	return links
}

// downloadLinks is get_download_links: per platform, the stable share links,
// the update-manifest URL and the newest direct build's artifact URL.
func (s *Server) downloadLinks(ctx context.Context, slug, platform string) (any, error) {
	q := dbgen.New(s.DB)
	app, err := q.AppBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("unknown app: %s", slug)
	}
	plats, err := q.ListPlatformsByApp(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	type latest struct {
		VersionCode int64  `json:"versionCode"`
		VersionName string `json:"versionName"`
		URL         string `json:"url"`
		Sha256      string `json:"sha256"`
		SizeBytes   int64  `json:"sizeBytes"`
	}
	type platOut struct {
		Platform     string      `json:"platform"`
		PackageName  string      `json:"packageName"`
		Shares       []shareLink `json:"shares"`
		ManifestURL  string      `json:"manifestUrl"`
		LatestDirect *latest     `json:"latestDirect"`
	}
	out := []platOut{}
	for _, p := range plats {
		if platform != "" && p.Platform != platform {
			continue
		}
		po := platOut{
			Platform: p.Platform, PackageName: p.PackageName,
			Shares:      s.shareLinks(slug, p),
			ManifestURL: s.baseURL + "/api/apps/" + slug + "/" + p.Platform + "/manifest",
		}
		rels, err := q.LatestReleaseForChannel(ctx, dbgen.LatestReleaseForChannelParams{AppPlatformID: p.ID, Channel: "direct"})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if len(rels) > 0 {
			rel := rels[0]
			u, err := s.storage.PublicURL(ctx, slug+"/"+p.Platform+"/"+rel.FileName)
			if err != nil {
				return nil, fmt.Errorf("sign url: %w", err)
			}
			po.LatestDirect = &latest{rel.VersionCode, rel.VersionName, u, rel.Sha256, rel.SizeBytes}
		}
		out = append(out, po)
	}
	if platform != "" && len(out) == 0 {
		return nil, fmt.Errorf("unknown app/platform: %s/%s", slug, platform)
	}
	return map[string]any{"slug": slug, "platforms": out}, nil
}

// ---- tools ----

type mcpTool struct {
	name        string
	title       string
	description string
	props       map[string]any // JSON-schema properties
	required    []string
	readOnly    bool
	destructive bool
	idempotent  bool
	run         func(ctx context.Context, s *Server, a mcpArgs) (any, error)
}

func (t mcpTool) definition() map[string]any {
	props := t.props
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(t.required) > 0 {
		schema["required"] = t.required
	}
	ann := map[string]any{"title": t.title, "readOnlyHint": t.readOnly}
	if !t.readOnly {
		ann["destructiveHint"] = t.destructive
		ann["idempotentHint"] = t.idempotent
	}
	ann["openWorldHint"] = false
	return map[string]any{
		"name": t.name, "title": t.title, "description": t.description,
		"inputSchema": schema, "annotations": ann,
	}
}

func mcpToolByName(name string) (mcpTool, bool) {
	for _, t := range mcpTools {
		if t.name == name {
			return t, true
		}
	}
	return mcpTool{}, false
}

// Schema snippets.
func pStr(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func pInt(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
func pBool(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func pEnum(desc string, vals ...string) map[string]any {
	return map[string]any{"type": "string", "enum": vals, "description": desc}
}

var (
	argSlug     = pStr("App slug, e.g. tiny-firewall.")
	argPlatform = pEnum("Platform variant. Defaults to android.", "android", "ios")
)

// simple wraps a handler call whose arguments map 1:1 onto path values and
// form fields.
func simple(method string, h func(*Server) http.HandlerFunc, path []string, form ...string) func(context.Context, *Server, mcpArgs) (any, error) {
	return func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
		return s.callHandler(ctx, method, h(s), a.path(path...), a.form(form...), nil)
	}
}

var mcpTools = []mcpTool{
	// ---- apps and platforms ----
	{
		name: "list_apps", title: "List apps", readOnly: true,
		description: "Every app with its platforms, package names, Play/signing status, release count and newest release.",
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleV1ListApps }, nil),
	},
	{
		name: "get_app", title: "Get app details", readOnly: true,
		description: "Everything the app page shows, per platform: releases (Play status), share links, manifest and upload URLs, testers, retention, signing-key fingerprint and Play wiring. Release urls here are paths on this server; list_releases and get_download_links return absolute URLs.",
		props:       map[string]any{"slug": argSlug},
		required:    []string{"slug"},
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleV1AppDetail }, []string{"slug"}),
	},
	{
		name: "create_app", title: "Create app",
		description: "Register a new app (product slug only). Add its android/ios variants with add_platform afterwards.",
		props:       map[string]any{"slug": pStr("New slug: lowercase letters, digits and dashes.")},
		required:    []string{"slug"},
		run:         simple("POST", func(s *Server) http.HandlerFunc { return s.handleApiCreateApp }, nil, "slug"),
	},
	{
		name: "delete_app", title: "Delete app", destructive: true, idempotent: true,
		description: "Permanently delete an app: all platforms, releases and artifacts, stored signing keys, keyless grants and their tokens. What Google Play already published stays on Play. Cannot be undone.",
		props:       map[string]any{"slug": argSlug},
		required:    []string{"slug"},
		run:         simple("DELETE", func(s *Server) http.HandlerFunc { return s.handleApiDeleteApp }, []string{"slug"}),
	},
	{
		name: "add_platform", title: "Add platform",
		description: "Add an android or ios variant to an app. Android platforms get a freshly generated signing key (its SHA-256 is returned). get_app's suggestedPackage proposes a package name from the bundle prefix setting.",
		props: map[string]any{
			"slug":        argSlug,
			"platform":    pEnum("Platform to add.", "android", "ios"),
			"packageName": pStr("Android application id or iOS bundle id, e.g. com.example.app."),
		},
		required: []string{"slug", "platform", "packageName"},
		run:      simple("POST", func(s *Server) http.HandlerFunc { return s.handleApiAddPlatform }, []string{"slug"}, "platform", "packageName"),
	},
	{
		name: "set_retention", title: "Set release retention", idempotent: true,
		description: "How many releases a platform keeps. keep: N keeps the newest N, 0 keeps everything, omit/null inherits the release-hub-wide setting.",
		props: map[string]any{
			"slug": argSlug, "platform": argPlatform,
			"keep": map[string]any{"type": []string{"integer", "null"}, "description": "Releases to keep (0 = all, null = inherit)."},
		},
		required: []string{"slug"},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			return s.callHandler(ctx, "PUT", s.handleV1Retention, a.path("slug", "platform"), url.Values{"keep": {a.str("keep")}}, nil)
		},
	},

	// ---- releases and distribution ----
	{
		name: "list_releases", title: "List releases", readOnly: true,
		description: "Releases of one platform, newest first: version code/name, channel, notes, size, sha256, absolute download URL and Play status.",
		props:       map[string]any{"slug": argSlug, "platform": argPlatform},
		required:    []string{"slug"},
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleApiReleases }, []string{"slug", "platform"}),
	},
	{
		name: "get_download_links", title: "Get download links", readOnly: true,
		description: "Shareable links per platform: the stable /download links (direct, plus internal/beta/public when Play is on) that always point at the newest build, the in-app update manifest URL, and the newest direct build's artifact URL.",
		props:       map[string]any{"slug": argSlug, "platform": pEnum("Only this platform (default: all).", "android", "ios")},
		required:    []string{"slug"},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			return s.downloadLinks(ctx, a.str("slug"), a.str("platform"))
		},
	},
	{
		name: "delete_release", title: "Delete release", destructive: true, idempotent: true,
		description: "Delete one release and its artifact from the release hub. Builds already on Google Play stay there.",
		props:       map[string]any{"slug": argSlug, "platform": argPlatform, "versionCode": pInt("Version code of the release.")},
		required:    []string{"slug", "versionCode"},
		run:         simple("DELETE", func(s *Server) http.HandlerFunc { return s.handleApiDeleteRelease }, []string{"slug", "platform", "versionCode"}),
	},

	// ---- Google Play ----
	{
		name: "set_play_publishing", title: "Set Play publishing", idempotent: true,
		description: "Turn Google Play publishing on for a platform with a shared service account (accountId from list_play_accounts), or off with enable=false.",
		props: map[string]any{
			"slug": argSlug, "platform": argPlatform,
			"accountId": pInt("Shared Play account id to publish through."),
			"enable":    pBool("false turns Play publishing off. Default true."),
		},
		required: []string{"slug"},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			form := url.Values{}
			if a.str("enable") == "false" {
				form.Set("enable", "false")
			} else if a.str("accountId") != "" {
				form.Set("account", a.str("accountId"))
			} else {
				return nil, errors.New("accountId is required to enable Play publishing (or pass enable=false)")
			}
			return s.callHandler(ctx, "POST", s.handleApiSetPlay, a.path("slug", "platform"), form, nil)
		},
	},
	{
		name: "check_play_setup", title: "Check Play setup", readOnly: true,
		description: "Verify a platform's Play wiring (account, credentials, API access, app exists on Play) without uploading. Returns {ok, detail}.",
		props:       map[string]any{"slug": argSlug, "platform": argPlatform},
		required:    []string{"slug"},
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleApiPlayPreflight }, []string{"slug", "platform"}),
	},
	{
		name: "list_play_tracks", title: "List Play tracks", readOnly: true,
		description: "The platform's Google Play tracks with their releases and tester groups (needs Play publishing on).",
		props:       map[string]any{"slug": argSlug, "platform": argPlatform},
		required:    []string{"slug"},
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleApiListTracks }, []string{"slug", "platform"}),
	},
	{
		name: "get_testers", title: "Get testers", readOnly: true,
		description: "The release-hub-wide beta tester Google Groups (pushable to Play with invite_testers) and individual tester emails (paste into Play Console). Edit them with update_settings.",
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			groups, emails := s.playTesters(ctx), s.testerEmails(ctx)
			if groups == nil {
				groups = []string{}
			}
			if emails == nil {
				emails = []string{}
			}
			return map[string]any{"groups": groups, "emails": emails}, nil
		},
	},
	{
		name: "invite_testers", title: "Invite testers", idempotent: true,
		description: "Push the configured beta tester Google Groups to the platform's Play closed testing (alpha) track.",
		props:       map[string]any{"slug": argSlug, "platform": argPlatform},
		required:    []string{"slug"},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			return s.callHandler(ctx, "POST", s.handleApiInviteTesters, a.path("slug", "platform"), url.Values{"channel": {"alpha"}}, nil)
		},
	},
	{
		name: "list_play_accounts", title: "List Play accounts", readOnly: true,
		description: "Shared Google Play service accounts: id, email and which app platforms publish through each. Credentials are never returned.",
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleV1ListPlayAccounts }, nil),
	},
	{
		name: "add_play_account", title: "Add Play account",
		description: "Store a Google Play service-account JSON key as a shared account (replaces an existing account with the same client_email). Prefer adding keys in the web app: this sends the private key through the AI client.",
		props:       map[string]any{"serviceAccountJson": pStr("The full service-account JSON key file contents.")},
		required:    []string{"serviceAccountJson"},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			body, err := fileUpload("file", "service-account.json", []byte(a.str("serviceAccountJson")))
			if err != nil {
				return nil, err
			}
			return s.callHandler(ctx, "POST", s.handleApiPlayAccounts, nil, nil, body)
		},
	},
	{
		name: "delete_play_account", title: "Delete Play account", destructive: true, idempotent: true,
		description: "Remove a shared Play service account. Platforms using it stop publishing to Play.",
		props:       map[string]any{"id": pInt("Play account id.")},
		required:    []string{"id"},
		run:         simple("POST", func(s *Server) http.HandlerFunc { return s.handleApiDeletePlayAccount }, nil, "id"),
	},

	// ---- keyless publishing (OIDC grants, GitHub App) ----
	{
		name: "list_keyless_grants", title: "List keyless grants", readOnly: true,
		description: "Keyless publishing grants (CI identities allowed to publish without a stored token). With slug: that app's grants; without: every grant.",
		props:       map[string]any{"slug": pStr("Only this app's grants (optional).")},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			if a.str("slug") != "" {
				return s.callHandler(ctx, "GET", s.handleV1ListOidcGrants, a.path("slug"), nil, nil)
			}
			return s.callHandler(ctx, "GET", s.handleV1ListAllOidcGrants, nil, nil, nil)
		},
	},
	{
		name: "create_keyless_grant", title: "Create keyless grant",
		description: "Let a CI identity publish an app keylessly by OIDC issuer + subject. For GitHub Actions the issuer is https://token.actions.githubusercontent.com and the subject e.g. repo:owner/name (completed to the default branch). With the GitHub App connected, connect_github_repository is simpler.",
		props: map[string]any{
			"slug":    argSlug,
			"issuer":  pStr("OIDC issuer URL (https)."),
			"subject": pStr("OIDC subject claim, e.g. repo:owner/name:ref:refs/heads/main."),
			"name":    pStr("Label for the grant (optional)."),
		},
		required: []string{"slug", "issuer", "subject"},
		run:      simple("POST", func(s *Server) http.HandlerFunc { return s.handleV1CreateOidcGrant }, []string{"slug"}, "issuer", "subject", "name"),
	},
	{
		name: "delete_keyless_grant", title: "Delete keyless grant", destructive: true, idempotent: true,
		description: "Remove a keyless grant and revoke its outstanding tokens. removeRepoVariables also deletes the RELEASE_HUB_URL/RELEASE_HUB_APP repo variables a GitHub connect set.",
		props: map[string]any{
			"id":                  pInt("Grant id from list_keyless_grants."),
			"removeRepoVariables": pBool("Also remove the GitHub repo variables (repository grants only)."),
		},
		required: []string{"id"},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			form := url.Values{}
			if a.str("removeRepoVariables") == "true" {
				form.Set("variables", "1")
			}
			return s.callHandler(ctx, "DELETE", s.handleV1DeleteOidcGrant, a.path("id"), form, nil)
		},
	},
	{
		name: "github_status", title: "GitHub App status", readOnly: true,
		description: "Whether this release hub's GitHub App is created and installed (needed for connect_github_repository). Creating/installing it is a browser flow in Settings → GitHub.",
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleV1GithubSettings }, nil),
	},
	{
		name: "list_github_repositories", title: "List GitHub repositories", readOnly: true,
		description: "Repositories the GitHub App can reach, for connect_github_repository.",
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleV1GithubRepositories }, nil),
	},
	{
		name: "connect_github_repository", title: "Connect GitHub repository",
		description: "One-click keyless publishing: create a grant for the repo's default branch and set its RELEASE_HUB_URL / RELEASE_HUB_APP variables. If those variables already hold other values nothing changes and the conflicts are returned; retry with overwrite=true to replace them.",
		props: map[string]any{
			"slug":       argSlug,
			"repository": pStr("owner/name"),
			"overwrite":  pBool("Replace conflicting repo variables."),
		},
		required: []string{"slug", "repository"},
		run:      simple("POST", func(s *Server) http.HandlerFunc { return s.handleV1GithubConnect }, []string{"slug"}, "repository", "overwrite"),
	},

	// ---- settings, tokens, vault ----
	{
		name: "get_settings", title: "Get settings", readOnly: true,
		description: "Release-hub-wide settings: bundle prefix, beta tester groups and emails, default retention and the certificate subject used for generated signing keys.",
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleV1GetSettings }, nil),
	},
	{
		name: "update_settings", title: "Update settings", idempotent: true,
		description: "Change release-hub-wide settings. Only the fields you pass are written.",
		props: map[string]any{
			"bundle_prefix":      pStr("Package name prefix suggested for new platforms, e.g. com.example."),
			"play_testers":       pStr("Beta tester Google Group addresses, comma-separated."),
			"play_tester_emails": pStr("Individual tester emails, comma-separated."),
			"prune_keep":         pStr("Default releases kept per platform (0 or empty = keep all)."),
			"org":                pStr("Signing certificate Organization (O)."),
			"ou":                 pStr("Signing certificate Organizational unit (OU)."),
			"locality":           pStr("Signing certificate Locality (L)."),
			"state":              pStr("Signing certificate State (ST)."),
			"country":            pStr("Signing certificate two-letter Country (C)."),
		},
		run: func(ctx context.Context, s *Server, a mcpArgs) (any, error) {
			form := a.form("bundle_prefix", "play_testers", "play_tester_emails", "prune_keep", "org", "ou", "locality", "state", "country")
			return s.callHandler(ctx, "PATCH", s.handleV1PatchSettings, nil, form, nil)
		},
	},
	{
		name: "list_tokens", title: "List API tokens", readOnly: true,
		description: "API tokens by id and name with creation and last-use times. Secrets are never shown.",
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleV1ListTokens }, nil),
	},
	{
		name: "create_token", title: "Create API token",
		description: "Create a new API token. The raw token is returned once; store it right away.",
		props:       map[string]any{"name": pStr("What the token is for, e.g. ci-myapp.")},
		required:    []string{"name"},
		run:         simple("POST", func(s *Server) http.HandlerFunc { return s.handleApiCreateToken }, nil, "name"),
	},
	{
		name: "delete_token", title: "Revoke API token", destructive: true, idempotent: true,
		description: "Revoke an API token immediately (id from list_tokens). Revoking the token this MCP connection uses disconnects it.",
		props:       map[string]any{"id": pInt("Token id.")},
		required:    []string{"id"},
		run:         simple("DELETE", func(s *Server) http.HandlerFunc { return s.handleV1DeleteToken }, []string{"id"}),
	},
	{
		name: "vault_status", title: "1Password vault status", readOnly: true,
		description: "Whether 1Password Connect is configured and which stored keys are still waiting to move into it.",
		run:         simple("GET", func(s *Server) http.HandlerFunc { return s.handleVaultStatus }, nil),
	},
	{
		name: "vault_migrate", title: "Move keys to 1Password",
		description: "Move stored signing keys and Play credentials into the 1Password vault (only when it is configured).",
		run:         simple("POST", func(s *Server) http.HandlerFunc { return s.handleVaultMigrate }, nil),
	},
}
