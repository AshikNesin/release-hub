package srv

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func seedToken(t *testing.T, s *Server, tok, scope string) {
	t.Helper()
	if _, err := s.DB.Exec("INSERT INTO api_tokens (name, token_hash, scope_slug) VALUES ('mcp', ?, ?)", hashToken(tok), scope); err != nil {
		t.Fatalf("seed token: %v", err)
	}
}

// mcpPost sends one JSON-RPC message to /mcp and returns status and body.
func mcpPost(t *testing.T, ts *httptest.Server, tok string, msg any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(msg)
	req, _ := http.NewRequest("POST", ts.URL+"/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

type mcpCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent map[string]any `json:"structuredContent"`
	IsError           bool           `json:"isError"`
}

var mcpCallID int

// mcpCall runs tools/call and decodes the tool's JSON text into out.
func mcpCall(t *testing.T, ts *httptest.Server, tok, tool string, args map[string]any, wantErr bool, out any) string {
	t.Helper()
	mcpCallID++
	status, raw := mcpPost(t, ts, tok, map[string]any{
		"jsonrpc": "2.0", "id": mcpCallID, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	if status != 200 {
		t.Fatalf("%s: status %d: %s", tool, status, raw)
	}
	var resp struct {
		Result *mcpCallResult `json:"result"`
		Error  *jsonrpcError  `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Result == nil {
		t.Fatalf("%s: bad response: %s", tool, raw)
	}
	if resp.Result.IsError != wantErr {
		t.Fatalf("%s: isError=%v, want %v: %s", tool, resp.Result.IsError, wantErr, raw)
	}
	text := resp.Result.Content[0].Text
	if out != nil {
		if err := json.Unmarshal([]byte(text), out); err != nil {
			t.Fatalf("%s: decode %q: %v", tool, text, err)
		}
	}
	return text
}

func TestMCPAuth(t *testing.T) {
	s, ts := newTestServer(t)
	init := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}}
	if status, _ := mcpPost(t, ts, "", init); status != 401 {
		t.Fatalf("no token: status %d, want 401", status)
	}
	if status, _ := mcpPost(t, ts, "rh_wrong", init); status != 401 {
		t.Fatalf("bad token: status %d, want 401", status)
	}
	// OIDC-minted tokens are scoped to one app's publish endpoints.
	seedToken(t, s, "rh_scoped", "demo")
	if status, _ := mcpPost(t, ts, "rh_scoped", init); status != 403 {
		t.Fatalf("scoped token: status %d, want 403", status)
	}
	// OAuth discovery must not return the SPA shell.
	resp, err := http.Get(ts.URL + "/.well-known/oauth-protected-resource/mcp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("oauth discovery: status %d, want 404", resp.StatusCode)
	}
}

func TestMCPProtocol(t *testing.T) {
	s, ts := newTestServer(t)
	seedToken(t, s, "rh_mcp", "")

	status, raw := mcpPost(t, ts, "rh_mcp", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "test", "version": "1"}},
	})
	if status != 200 || !strings.Contains(string(raw), `"protocolVersion":"2025-06-18"`) || !strings.Contains(string(raw), `"tools"`) {
		t.Fatalf("initialize: %d %s", status, raw)
	}
	// Unknown versions get the newest one.
	_, raw = mcpPost(t, ts, "rh_mcp", map[string]any{"jsonrpc": "2.0", "id": 2, "method": "initialize",
		"params": map[string]any{"protocolVersion": "1999-01-01"}})
	if !strings.Contains(string(raw), `"protocolVersion":"`+mcpProtocolVersions[0]+`"`) {
		t.Fatalf("initialize fallback: %s", raw)
	}
	if status, raw := mcpPost(t, ts, "rh_mcp", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); status != 202 || len(raw) != 0 {
		t.Fatalf("notification: %d %q", status, raw)
	}
	_, raw = mcpPost(t, ts, "rh_mcp", map[string]any{"jsonrpc": "2.0", "id": 3, "method": "nope"})
	if !strings.Contains(string(raw), `-32601`) {
		t.Fatalf("unknown method: %s", raw)
	}

	_, raw = mcpPost(t, ts, "rh_mcp", map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/list"})
	var list struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range list.Result.Tools {
		names[tl.Name] = true
		if tl.InputSchema["type"] != "object" {
			t.Errorf("%s: inputSchema type %v", tl.Name, tl.InputSchema["type"])
		}
	}
	for _, want := range []string{"list_apps", "create_app", "add_platform", "get_download_links", "list_releases", "create_keyless_grant", "connect_github_repository"} {
		if !names[want] {
			t.Errorf("tools/list is missing %s", want)
		}
	}

	req, _ := http.NewRequest("GET", ts.URL+"/mcp", nil)
	req.Header.Set("Authorization", "Bearer rh_mcp")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Fatalf("GET /mcp: status %d, want 405", resp.StatusCode)
	}
}

func TestMCPAppLifecycle(t *testing.T) {
	t.Setenv("RELEASE_HUB_SECRET_KEY", "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleXRlc3QyMw==")
	s, ts := newTestServer(t)
	seedToken(t, s, "rh_mcp", "")
	tok := "rh_mcp"

	mcpCall(t, ts, tok, "create_app", map[string]any{"slug": "demo"}, false, nil)
	if msg := mcpCall(t, ts, tok, "create_app", map[string]any{"slug": "demo"}, true, nil); !strings.Contains(msg, "already exists") {
		t.Fatalf("duplicate create: %q", msg)
	}
	if msg := mcpCall(t, ts, tok, "create_app", map[string]any{}, true, nil); !strings.Contains(msg, "slug is required") {
		t.Fatalf("missing slug: %q", msg)
	}

	var plat map[string]string
	mcpCall(t, ts, tok, "add_platform", map[string]any{"slug": "demo", "platform": "android", "packageName": "io.demo"}, false, &plat)
	if plat["signingKey"] != "generated" || plat["signingSha256"] == "" {
		t.Fatalf("add android: %v", plat)
	}
	mcpCall(t, ts, tok, "add_platform", map[string]any{"slug": "demo", "platform": "ios", "packageName": "io.demo.ios"}, false, nil)

	var apps []struct {
		Slug      string
		Platforms []struct{ Platform string }
	}
	mcpCall(t, ts, tok, "list_apps", nil, false, &apps)
	if len(apps) != 1 || apps[0].Slug != "demo" || len(apps[0].Platforms) != 2 {
		t.Fatalf("list_apps: %+v", apps)
	}

	// A release, as CI would upload it.
	uploadAPK(t, ts, tok, "demo", "android", 7, "1.0.7")

	var rels []map[string]any
	mcpCall(t, ts, tok, "list_releases", map[string]any{"slug": "demo"}, false, &rels)
	if len(rels) != 1 || rels[0]["versionName"] != "1.0.7" || !strings.HasPrefix(rels[0]["url"].(string), "http://release-hub.test/artifacts/demo/android/") {
		t.Fatalf("list_releases: %v", rels)
	}

	var links struct {
		Platforms []struct {
			Platform     string
			Shares       []shareLink
			ManifestURL  string
			LatestDirect *struct {
				VersionCode int64
				URL         string
			}
		}
	}
	mcpCall(t, ts, tok, "get_download_links", map[string]any{"slug": "demo"}, false, &links)
	if len(links.Platforms) != 2 {
		t.Fatalf("links: %+v", links)
	}
	for _, p := range links.Platforms {
		switch p.Platform {
		case "android":
			if p.Shares[0].URL != "http://release-hub.test/apps/demo/download?channel=direct" ||
				p.LatestDirect == nil || p.LatestDirect.VersionCode != 7 ||
				p.ManifestURL != "http://release-hub.test/api/apps/demo/android/manifest" {
				t.Fatalf("android links: %+v", p)
			}
		case "ios":
			if p.Shares[0].URL != "http://release-hub.test/apps/demo/download/ios?channel=direct" || p.LatestDirect != nil {
				t.Fatalf("ios links: %+v", p)
			}
		}
	}

	var detail struct {
		Platforms []struct {
			Platform string
			Shares   []shareLink
		}
	}
	mcpCall(t, ts, tok, "get_app", map[string]any{"slug": "demo"}, false, &detail)
	if len(detail.Platforms) != 2 {
		t.Fatalf("get_app: %+v", detail)
	}

	var ret map[string]any
	mcpCall(t, ts, tok, "set_retention", map[string]any{"slug": "demo", "keep": 3}, false, &ret)
	if ret["effective"] != float64(3) {
		t.Fatalf("set_retention: %v", ret)
	}

	mcpCall(t, ts, tok, "create_keyless_grant", map[string]any{"slug": "demo",
		"issuer": "https://token.actions.githubusercontent.com", "subject": "repo:me/demo"}, false, nil)
	var grants []map[string]any
	mcpCall(t, ts, tok, "list_keyless_grants", nil, false, &grants)
	if len(grants) != 1 {
		t.Fatalf("grants: %v", grants)
	}
	mcpCall(t, ts, tok, "delete_keyless_grant", map[string]any{"id": grants[0]["id"]}, false, nil)

	var tokOut map[string]string
	mcpCall(t, ts, tok, "create_token", map[string]any{"name": "ci"}, false, &tokOut)
	if !strings.HasPrefix(tokOut["token"], "rh_") {
		t.Fatalf("create_token: %v", tokOut)
	}

	if msg := mcpCall(t, ts, tok, "set_play_publishing", map[string]any{"slug": "demo"}, true, nil); !strings.Contains(msg, "accountId") {
		t.Fatalf("set_play without account: %q", msg)
	}
	if msg := mcpCall(t, ts, tok, "add_play_account", map[string]any{"serviceAccountJson": "{}"}, true, nil); !strings.Contains(msg, "client_email") {
		t.Fatalf("add_play_account with bad JSON: %q", msg)
	}

	mcpCall(t, ts, tok, "update_settings", map[string]any{"play_testers": "beta@googlegroups.com"}, false, nil)
	var testers map[string][]string
	mcpCall(t, ts, tok, "get_testers", nil, false, &testers)
	if len(testers["groups"]) != 1 {
		t.Fatalf("get_testers: %v", testers)
	}

	mcpCall(t, ts, tok, "delete_release", map[string]any{"slug": "demo", "versionCode": 7}, false, nil)
	mcpCall(t, ts, tok, "delete_app", map[string]any{"slug": "demo"}, false, nil)
	if msg := mcpCall(t, ts, tok, "get_app", map[string]any{"slug": "demo"}, true, nil); !strings.Contains(msg, "unknown app") {
		t.Fatalf("get deleted app: %q", msg)
	}
}

// uploadAPK uploads a fake direct-channel APK through the bearer API, as CI does.
func uploadAPK(t *testing.T, ts *httptest.Server, tok, slug, platform string, versionCode int, versionName string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", slug+".apk")
	fw.Write([]byte("fake-apk-bytes"))
	mw.WriteField("channel", "direct")
	mw.WriteField("versionCode", strconv.Itoa(versionCode))
	mw.WriteField("versionName", versionName)
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/apps/"+slug+"/"+platform+"/releases", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload: %d %s", resp.StatusCode, raw)
	}
}
