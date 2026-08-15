package srv

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"srv.exe.dev/db/dbgen"
)

// signingConfig is the plaintext structure stored encrypted in the DB.
type signingConfig struct {
	StorePassword string `json:"storePassword"`
	KeyAlias      string `json:"keyAlias"`
	KeyPassword   string `json:"keyPassword"`
}

// handleApiSetSigning POST /api/apps/{slug}/signing
// multipart: file=<keystore> storePassword= keyAlias= [keyPassword=]
// Stores the keystore encrypted; sha256 recorded in plaintext so gradle
// builds can verify what they fetched. keyPassword defaults to storePassword.
func (s *Server) handleApiSetSigning(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil || hdr.Size == 0 {
		writeErr(w, 400, "multipart file field with keystore required")
		return
	}
	defer f.Close()
	ks, err := io.ReadAll(io.LimitReader(f, 8<<20)) // keystores are small
	if err != nil {
		writeErr(w, 400, "read keystore: "+err.Error())
		return
	}
	cfg := signingConfig{
		StorePassword: r.FormValue("storePassword"),
		KeyAlias:      r.FormValue("keyAlias"),
		KeyPassword:   r.FormValue("keyPassword"),
	}
	if cfg.StorePassword == "" || cfg.KeyAlias == "" {
		writeErr(w, 400, "storePassword and keyAlias required")
		return
	}
	if cfg.KeyPassword == "" {
		cfg.KeyPassword = cfg.StorePassword
	}
	cfgJSON, _ := json.Marshal(cfg)
	encKS, err := encryptCreds(ks)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	encCfg, err := encryptCreds(cfgJSON)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	sum := sha256.Sum256(ks)
	if err := dbgen.New(s.DB).SetSigningConfig(r.Context(), dbgen.SetSigningConfigParams{
		SignKeystore: encKS, SignConfig: encCfg,
		SignSha256: hex.EncodeToString(sum[:]), ID: app.ID,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"stored": true, "keystoreSha256": hex.EncodeToString(sum[:])})
}

// handleApiGetSigning GET /api/apps/{slug}/signing
// Returns the keystore bytes (X-Hub-Keystore-Sha256 header) and config
// headers for CI: any authenticated build env can produce signed releases.
// It is a bearer-auth endpoint — never expose it publicly.
func (s *Server) handleApiGetSigning(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if app.SignKeystore == "" {
		writeErr(w, 404, "no signing keystore stored for this app")
		return
	}
	ks, err := decryptCreds(app.SignKeystore)
	if err != nil {
		writeErr(w, 500, "decrypt keystore: "+err.Error())
		return
	}
	cfgRaw, err := decryptCreds(app.SignConfig)
	if err != nil {
		writeErr(w, 500, "decrypt config: "+err.Error())
		return
	}
	var cfg signingConfig
	_ = json.Unmarshal(cfgRaw, &cfg)
	sum := sha256.Sum256(ks)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Hub-Keystore-Sha256", hex.EncodeToString(sum[:]))
	w.Header().Set("X-Hub-Store-Password", cfg.StorePassword)
	w.Header().Set("X-Hub-Key-Alias", cfg.KeyAlias)
	w.Header().Set("X-Hub-Key-Password", cfg.KeyPassword)
	w.WriteHeader(200)
	_, _ = w.Write(ks)
}

// handleApiDeleteSigning POST /api/apps/{slug}/signing/delete — wipes stored key material.
func (s *Server) handleApiDeleteSigning(w http.ResponseWriter, r *http.Request) {
	app, ok := s.appFromSlug(w, r, r.PathValue("slug"))
	if !ok {
		return
	}
	if err := dbgen.New(s.DB).SetSigningConfig(r.Context(), dbgen.SetSigningConfigParams{
		SignKeystore: "", SignConfig: "", SignSha256: "", ID: app.ID,
	}); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"stored": false})
}

// generateKeystore creates a fresh RSA-2048 key with a long-lived self-signed
// certificate and returns it as a PKCS#12 keystore, ready for Android Gradle
// signing (storeFile + storePassword/keyAlias/keyPassword). Also returns the
// plaintext config and a PEM copy of the public certificate (handy for Play
// App Signing enrollment or pinning).
//
// The PKCS#12 encoder is LegacyRC2 (the traditional keytool/OpenSSL parameters)
// rather than Modern2023 — not for crypto reasons (the keystore is additionally
// encrypted at rest by the hub) but because older Android build tooling and
// some JDK builds still expect the legacy PBE algorithms. Everything modern
// still reads it fine.
func generateKeystore(commonName string) (p12 []byte, cfg signingConfig, certPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, cfg, nil, fmt.Errorf("generate rsa key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, cfg, nil, fmt.Errorf("generate serial: %w", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"release-hub"},
		},
		NotBefore:             time.Now().Add(-time.Hour), // tolerate clock skew
		NotAfter:              time.Now().AddDate(30, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, cfg, nil, fmt.Errorf("create certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, cfg, nil, fmt.Errorf("parse certificate: %w", err)
	}

	// High-entropy passwords (openssl rand -hex style): the PKCS#12 KDF
	// iterations don't add meaningful protection, so the password must.
	rawPW := make([]byte, 16)
	if _, err := rand.Read(rawPW); err != nil {
		return nil, cfg, nil, fmt.Errorf("generate password: %w", err)
	}
	storePW := hex.EncodeToString(rawPW)
	cfg = signingConfig{
		StorePassword: storePW,
		KeyAlias:      "release",
		KeyPassword:   storePW,
	}
	p12, err = pkcs12.LegacyRC2.Encode(key, cert, nil, cfg.StorePassword)
	if err != nil {
		return nil, cfg, nil, fmt.Errorf("encode pkcs12: %w", err)
	}
	// Java derives the keystore alias from the PKCS#12 friendlyName attribute;
	// without it keytool shows the entry as "1" and gradle's keyAlias=release
	// fails. The library has no API to set it on key bags, so inject it into the
	// (unencrypted) key bag and recompute the integrity MAC ourselves.
	p12, err = withKeyBagFriendlyName(p12, cfg.KeyAlias, cfg.StorePassword)
	if err != nil {
		return nil, cfg, nil, fmt.Errorf("set keystore alias: %w", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return p12, cfg, certPEM, nil
}

// FetchSigningForBuild is the helper CI scripts use: writes the keystore to
// path and returns the signing properties for gradle.
func FetchSigningForBuild(hubBaseURL, slug, token, outPath string) (storePassword, keyAlias, keyPassword, sha string, err error) {
	req, err := http.NewRequest("GET", hubBaseURL+"/api/apps/"+slug+"/signing", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err = fmt.Errorf("fetch signing: %s", resp.Status)
		return
	}
	ks, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if err = os.WriteFile(outPath, ks, 0o600); err != nil {
		return
	}
	storePassword = resp.Header.Get("X-Hub-Store-Password")
	keyAlias = resp.Header.Get("X-Hub-Key-Alias")
	keyPassword = resp.Header.Get("X-Hub-Key-Password")
	sha = resp.Header.Get("X-Hub-Keystore-Sha256")
	return
}

// ---- PKCS#12 friendlyName injection -------------------------------
//
// go-pkcs12's Encoder.Encode has no way to set the friendlyName attribute on
// the key/cert bags. Java (and therefore Android Gradle signing via keytool
// semantics) uses friendlyName as the keystore alias, so without it the entry
// shows up as "1" and a configured keyAlias of "release" fails with
// "key not found". withKeyBagFriendlyName re-marshals the DER: it walks the
// AuthSafe, finds the unencrypted SafeContents containing the shrouded key
// bag, adds the friendlyName attribute to that bag, and recomputes the
// PFX MAC (HMAC-SHA1 via the PKCS#12 KDF, matching LegacyRC2).

type pfxPduX struct {
	Version  int
	AuthSafe struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
	}
	MacData struct {
		Mac struct {
			Algorithm pkix.AlgorithmIdentifier
			Digest    []byte
		}
		MacSalt    []byte
		Iterations int `asn1:"optional,default:1"`
	} `asn1:"optional"`
}

// wrapTLV prefixes tlv with a context tag header: tag <<inner>>.
func wrapTLV(tag byte, tlv []byte) ([]byte, error) {
	var lenBytes []byte
	n := len(tlv)
	switch {
	case n < 0x80:
		lenBytes = []byte{byte(n)}
	case n < 0x100:
		lenBytes = []byte{0x81, byte(n)}
	case n < 0x10000:
		lenBytes = []byte{0x82, byte(n >> 8), byte(n)}
	case n < 0x1000000:
		lenBytes = []byte{0x83, byte(n >> 16), byte(n >> 8), byte(n)}
	default:
		return nil, fmt.Errorf("tlv too large: %d", n)
	}
	return append([]byte{tag}, append(lenBytes, tlv...)...), nil
}

// derLength returns DER length bytes for n.
func derLength(n int) []byte {
	switch {
	case n < 0x80:
		return []byte{byte(n)}
	case n < 0x100:
		return []byte{0x81, byte(n)}
	case n < 0x10000:
		return []byte{0x82, byte(n >> 8), byte(n)}
	default:
		return []byte{0x83, byte(n >> 16), byte(n >> 8), byte(n)}
	}
}

func rawValueOf(tlv []byte) (asn1.RawValue, error) {
	var rv asn1.RawValue
	if _, err := asn1.Unmarshal(tlv, &rv); err != nil {
		return rv, err
	}
	return rv, nil
}

// buildDataContentInfo constructs ContentInfo{pkcs7-data, [0] EXPLICIT
// OCTET STRING(payload)} with correct universal tags.
func buildDataContentInfo(payload []byte) (asn1.RawValue, error) {
	octet, err := asn1.Marshal(payload) // 04 len ...
	if err != nil {
		return asn1.RawValue{}, err
	}
	// ContentInfo ::= SEQUENCE { contentType OID, content [0] EXPLICIT ANY }
	// with content = OCTET STRING. Hand-assemble both TLVs: asn1.Marshal
	// cannot emit a context-tagged wrapper around raw bytes reliably.
	var out bytes.Buffer
	out.Write([]byte{0x30}) // SEQUENCE, length patched below
	body, err := asn1.Marshal(asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1})
	if err != nil {
		return asn1.RawValue{}, err
	}
	out.Write(body)
	out.Write([]byte{0xA0})
	out.Write(derLength(len(octet)))
	out.Write(octet)
	seq := out.Bytes()
	return rawValueOf(append([]byte{0x30}, append(derLength(len(seq)-1), seq[1:]...)...))
}

func withKeyBagFriendlyName(p12 []byte, alias, password string) ([]byte, error) {
	var pfx pfxPduX
	rest, err := asn1.Unmarshal(p12, &pfx)
	if err != nil || len(rest) != 0 {
		return nil, fmt.Errorf("parse pfx: %w", err)
	}
	// AuthSafe.Content is [0] EXPLICIT wrapping an OCTET STRING whose payload
	// is the DER of SEQUENCE OF ContentInfo.
	var octet asn1.RawValue
	if _, err := asn1.Unmarshal(pfx.AuthSafe.Content.Bytes, &octet); err != nil || octet.Tag != asn1.TagOctetString {
		return nil, fmt.Errorf("parse authSafe octet string: %w", err)
	}
	var contents []asn1.RawValue
	if rest, err := asn1.Unmarshal(octet.Bytes, &contents); err != nil || len(rest) != 0 {
		return nil, fmt.Errorf("parse authSafe contents: %w", err)
	}

	oidData := asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidPkcs8ShroudedKeyBag := asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	oidFriendlyName := asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 20}

	var outContents []asn1.RawValue
	modified := false
	for _, ci := range contents {
		if !modified {
			var hdr struct {
				ContentType asn1.ObjectIdentifier
				Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
			}
			if _, err := asn1.Unmarshal(ci.FullBytes, &hdr); err == nil && hdr.ContentType.Equal(oidData) {
				// unencrypted SafeContents: again peel [0] EXPLICIT → OCTET STRING.
				var oct asn1.RawValue
				var bags []asn1.RawValue
				if _, e1 := asn1.Unmarshal(hdr.Content.Bytes, &oct); e1 == nil && oct.Tag == asn1.TagOctetString {
					if rest2, e2 := asn1.Unmarshal(oct.Bytes, &bags); e2 == nil && len(rest2) == 0 {
						var outBags []asn1.RawValue
						for _, bag := range bags {
							var sb struct {
								Id         asn1.ObjectIdentifier
								Value      asn1.RawValue `asn1:"tag:0,explicit"`
								Attributes []pkcs12AttrX `asn1:"set,optional"`
							}
							if _, err := asn1.Unmarshal(bag.FullBytes, &sb); err != nil {
								return nil, fmt.Errorf("parse safeBag: %w", err)
							}
							if sb.Id.Equal(oidPkcs8ShroudedKeyBag) && !hasAttrX(sb.Attributes, oidFriendlyName) {
								friendly, err := friendlyNameAttr(alias)
								if err != nil {
									return nil, err
								}
								sb.Attributes = append(sb.Attributes, friendly)
								nb, err := asn1.Marshal(sb)
								if err != nil {
									return nil, fmt.Errorf("marshal safeBag: %w", err)
								}
								var rv asn1.RawValue
								if _, err := asn1.Unmarshal(nb, &rv); err != nil {
									return nil, err
								}
								outBags = append(outBags, rv)
								modified = true
							} else {
								outBags = append(outBags, bag)
							}
						}
						if modified {
							nb, err := asn1.Marshal(outBags)
							if err != nil {
								return nil, err
							}
							// Build the ContentInfo by hand so the inner
							// element is a genuine OCTET STRING (universal
							// tag 4), not a context tag: encoding via a
							// struct's tag:0 field would turn Tag=4 into a
							// context-specific 4, which nothing can parse.
							ciBytes, err := buildDataContentInfo(nb)
							if err != nil {
								return nil, err
							}
							outContents = append(outContents, ciBytes)
							continue
						}
					}
				}
			}
		}
		outContents = append(outContents, ci)
	}
	if !modified {
		return p12, nil // key bag not found / already has friendlyName: leave as-is
	}

	newInner, err := asn1.Marshal(outContents)
	if err != nil {
		return nil, err
	}
	// The AuthSafe Content is [0] EXPLICIT wrapping an OCTET STRING whose
	// payload is the DER of SEQUENCE OF ContentInfo — not another ContentInfo.
	oct, err := asn1.Marshal(newInner)
	if err != nil {
		return nil, err
	}
	var octRV asn1.RawValue
	if _, err := asn1.Unmarshal(oct, &octRV); err != nil {
		return nil, err
	}
	// Hand-build the [0] EXPLICIT wrapper: A0 <len> <octet TLV>. go's
	// asn1.Marshal re-encodes RawValue fields (Class/Tag are advisory), which
	// produced a SEQUENCE instead of the context tag — unreadable by every
	// PKCS#12 consumer.
	octTLV := oct // "04 len payload"
	a0TLV, err := wrapTLV(0xA0, octTLV)
	if err != nil {
		return nil, err
	}
	var wrappedRV asn1.RawValue
	if _, err := asn1.Unmarshal(a0TLV, &wrappedRV); err != nil {
		return nil, err
	}

	// Recompute the MAC over the new AuthSafe (RFC 7292 appendix B, ID=3).
	if !pfx.MacData.Mac.Algorithm.Algorithm.Equal(asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}) { // SHA-1 only (LegacyRC2)
		return nil, fmt.Errorf("unsupported MAC algorithm for re-sign: %s", pfx.MacData.Mac.Algorithm.Algorithm)
	}
	pw, err := bmpStringZeroTerm(password)
	if err != nil {
		return nil, err
	}
	macKey := pkcs12KDF(pw, pfx.MacData.MacSalt, pfx.MacData.Iterations, 3, 20)
	mac := hmac.New(sha1.New, macKey)
	mac.Write(newInner)

	// Rebuild the PFX with the new AuthSafe content and fresh digest.
	type pfxOut struct {
		Version  int
		AuthSafe struct {
			ContentType asn1.ObjectIdentifier
			Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
		}
		MacData struct {
			Mac struct {
				Algorithm pkix.AlgorithmIdentifier
				Digest    []byte
			}
			MacSalt    []byte
			Iterations int `asn1:"optional,default:1"`
		} `asn1:"optional"`
	}
	out := pfxOut{Version: pfx.Version}
	out.AuthSafe.ContentType = pfx.AuthSafe.ContentType
	out.AuthSafe.Content = wrappedRV
	out.MacData = pfx.MacData
	out.MacData.Mac.Digest = mac.Sum(nil)
	return asn1.Marshal(out)
}

type pkcs12AttrX struct {
	Id    asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"set"`
}

func hasAttrX(attrs []pkcs12AttrX, id asn1.ObjectIdentifier) bool {
	for _, a := range attrs {
		if a.Id.Equal(id) {
			return true
		}
	}
	return false
}

func friendlyNameAttr(alias string) (pkcs12AttrX, error) {
	bmp, err := bmpStringZeroTerm(alias)
	if err != nil {
		return pkcs12AttrX{}, err
	}
	// PKCS#9 friendlyName attribute value: SET OF { BMPString }. The struct's
	// `asn1:"set"` field tag supplies the SET, so Value.Bytes must hold just
	// the encoded BMPString TLV (tag 30 decimal = 0x1e).
	bmpTLV, err := asn1.Marshal(asn1.RawValue{Class: 0, Tag: 30, IsCompound: false, Bytes: bmp[:len(bmp)-2]})
	if err != nil {
		return pkcs12AttrX{}, err
	}
	return pkcs12AttrX{
		Id:    asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 20},
		Value: asn1.RawValue{Class: 0, Tag: 17, IsCompound: true, Bytes: bmpTLV},
	}, nil
}

// bmpStringZeroTerm encodes s as UCS-2 with a NUL terminator (RFC 7292 B.1).
func bmpStringZeroTerm(s string) ([]byte, error) {
	out := make([]byte, 0, 2*len(s)+2)
	for _, r := range s {
		if r > 0xFFFF {
			return nil, fmt.Errorf("character %q cannot be encoded in UCS-2", r)
		}
		out = append(out, byte(r>>8), byte(r))
	}
	return append(out, 0, 0), nil
}

// pkcs12KDF implements RFC 7292 appendix B.2 for SHA-1 (u=20, v=64).
func pkcs12KDF(password, salt []byte, iterations int, id byte, size int) []byte {
	const u, v = 20, 64
	D := bytes.Repeat([]byte{id}, v)
	S := fillRepeats(salt, v)
	P := fillRepeats(password, v)
	I := append(append([]byte{}, S...), P...)
	c := (size + u - 1) / u
	A := make([]byte, c*u)
	for i := 0; i < c; i++ {
		Ai := sha1Sum(append(append([]byte{}, D...), I...))
		for j := 1; j < iterations; j++ {
			Ai = sha1Sum(Ai)
		}
		copy(A[i*u:], Ai)
		if i < c-1 {
			B := fillRepeats(Ai, v)[:v]
			for k := 0; k < len(I)/v; k++ {
				var n big.Int
				n.SetBytes(I[k*v : (k+1)*v])
				n.Add(&n, big.NewInt(int64(B[0])))
				if n.Sign() < 0 {
					// shouldn't happen; keep simple
				}
				b := make([]byte, v)
				n.FillBytes(b)
				copy(I[k*v:(k+1)*v], b)
			}
		}
	}
	return A[:size]
}

func fillRepeats(pattern []byte, v int) []byte {
	if len(pattern) == 0 {
		return nil
	}
	outLen := v * ((len(pattern) + v - 1) / v)
	return bytes.Repeat(pattern, (outLen+len(pattern)-1)/len(pattern))[:outLen]
}

func sha1Sum(in []byte) []byte {
	s := sha1.Sum(in)
	return s[:]
}
