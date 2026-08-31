package push

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// RFC 5869 Appendix A.1 Test Case 1 (SHA-256) validates the HKDF core.
func TestHKDFRFC5869Vector(t *testing.T) {
	ikm := make([]byte, 22)
	for i := range ikm {
		ikm[i] = 0x0b
	}
	salt := mustHex(t, "000102030405060708090a0b0c")
	info := mustHex(t, "f0f1f2f3f4f5f6f7f8f9")
	wantPRK := "077709362c2e32df0ddc3f0dc47bba6390b6c73bb50f9c3122ec844ad7c2b3e5"
	wantOKM := "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"

	prk := hkdfExtract(salt, ikm)
	if hex.EncodeToString(prk) != wantPRK {
		t.Fatalf("PRK mismatch:\n got %x\nwant %s", prk, wantPRK)
	}
	okm := hkdfExpand(prk, info, 42)
	if hex.EncodeToString(okm) != wantOKM {
		t.Fatalf("OKM mismatch:\n got %x\nwant %s", okm, wantOKM)
	}
}

func genUA(t *testing.T) (*ecdh.PrivateKey, Subscription, []byte) {
	t.Helper()
	ua, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding
	sub := Subscription{
		Endpoint: "https://push.example.net/w/123",
		P256DH:   b64.EncodeToString(ua.PublicKey().Bytes()),
		Auth:     b64.EncodeToString(auth),
	}
	return ua, sub, auth
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	ua, sub, auth := genUA(t)
	plaintext := []byte("Backup finished in 42s ✅ nudge")
	body, err := Encrypt(sub, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	// aes128gcm header layout: salt(16) rs(4) idlen(1) id(65)
	if body[20] != 65 {
		t.Fatalf("keyid length = %d, want 65", body[20])
	}
	out, err := Decrypt(ua.Bytes(), auth, body)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plaintext) {
		t.Fatalf("roundtrip mismatch: %q", string(out))
	}
}

func TestEncryptRejectsOversize(t *testing.T) {
	_, sub, _ := genUA(t)
	big := make([]byte, MaxPayload+1)
	if _, err := Encrypt(sub, big); err == nil {
		t.Fatal("expected payload size error")
	}
}

// TestDeterministicVector emits a fixed-input vector for independent
// cross-language decryption (tests/webpush_vector.json via Python).
func TestDeterministicVector(t *testing.T) {
	uaScalar := mustHex(t, "112233445566778899001122334455667788990011223344556677889900aabb")
	asScalar := mustHex(t, "ccddeeff00112233445566778899aabbccddeeff00112233445566778899ccdd")
	salt := mustHex(t, "0102030405060708090a0b0c0d0e0f10")
	auth := mustHex(t, "a1a2a3a4a5a6a7a8a9a0b1b2b3b4b5b6")

	uaPriv, err := ecdh.P256().NewPrivateKey(uaScalar)
	if err != nil {
		t.Fatal(err)
	}
	sub := Subscription{
		Endpoint: "https://push.example.net/w/vec",
		P256DH:   base64.RawURLEncoding.EncodeToString(uaPriv.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(auth),
	}
	plaintext := []byte("When I grow up, I want to be a nudge.")
	body, err := encryptWith(sub, asScalar, salt, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	// self-decrypt sanity
	out, err := Decrypt(uaScalar, auth, body)
	if err != nil || string(out) != string(plaintext) {
		t.Fatalf("deterministic self-decrypt failed: %v %q", err, out)
	}
	vec := map[string]string{
		"ua_private": hex.EncodeToString(uaScalar),
		"as_private": hex.EncodeToString(asScalar),
		"auth":       hex.EncodeToString(auth),
		"salt":       hex.EncodeToString(salt),
		"plaintext":  string(plaintext),
		"ciphertext": hex.EncodeToString(body),
	}
	if os.Getenv("UPDATE_VECTOR") != "" {
		b, _ := json.MarshalIndent(vec, "", "  ")
		if err := os.WriteFile("../../tests/webpush_vector.json", b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVAPIDMarshalAndJWT(t *testing.T) {
	v, err := GenerateVAPID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	v2, err := UnmarshalVAPID(b)
	if err != nil {
		t.Fatal(err)
	}
	if v2.PublicB64() != v.PublicB64() {
		t.Fatal("public key changed after marshal roundtrip")
	}
	jwt, err := v.SignJWT("https://push.example.net", "mailto:ops@example.com", time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatal("jwt must have 3 parts")
	}
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if !strings.Contains(string(header), `"alg":"ES256"`) {
		t.Fatalf("bad header %s", header)
	}
	signingInput := parts[0] + "." + parts[1]
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if len(sig) != 64 {
		t.Fatalf("ES256 signature = %d bytes, want 64", len(sig))
	}
	// Verify with the public key (R||S split).
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	sum := sha256.Sum256([]byte(signingInput))
	pub := &v.priv.PublicKey
	if !ecdsa.Verify(pub, sum[:], r, s) {
		t.Fatal("JWT signature failed verification")
	}
}

func TestSendRejectsBadAudience(t *testing.T) {
	v, _ := GenerateVAPID()
	_, sub, _ := genUA(t)
	sub.Endpoint = "not-a-url"
	if _, err := Send(http.DefaultClient, sub, v, "mailto:x", []byte("hi"), 60); err == nil {
		t.Fatal("expected audience validation error")
	}
}
