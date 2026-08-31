package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// VAPIDKeys is the application-server key pair used both for VAPID
// authentication (ES256 JWT) and as the Web Push applicationServerKey that
// browsers bind subscriptions to.
type VAPIDKeys struct {
	priv *ecdsa.PrivateKey
}

// vapidFile is the on-disk serialization (base64url scalar + point).
type vapidFile struct {
	Private string `json:"private"`
	Public  string `json:"public"`
}

// GenerateVAPID creates a fresh P-256 key pair.
func GenerateVAPID() (*VAPIDKeys, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return &VAPIDKeys{priv: priv}, nil
}

// Marshal serializes the pair to JSON (private scalar + uncompressed point,
// both base64url). The file must be stored with 0600 permissions.
func (v *VAPIDKeys) Marshal() ([]byte, error) {
	d := v.priv.D.FillBytes(make([]byte, 32))
	return json.Marshal(vapidFile{
		Private: b64(d),
		Public:  b64(v.PublicBytes()),
	})
}

// UnmarshalVAPID restores a key pair from Marshal output.
func UnmarshalVAPID(b []byte) (*VAPIDKeys, error) {
	var f vapidFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	dBytes, err := b64dec(f.Private)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	pubBytes, err := b64dec(f.Public)
	if err != nil {
		return nil, fmt.Errorf("public key: %w", err)
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), pubBytes)
	if x == nil {
		return nil, errors.New("invalid public point")
	}
	d := new(big.Int).SetBytes(dBytes)
	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y},
		D:         d,
	}
	// Sanity: derived public point must match.
	px, py := elliptic.P256().ScalarBaseMult(d.FillBytes(make([]byte, 32)))
	if px.Cmp(x) != 0 || py.Cmp(y) != 0 {
		return nil, errors.New("private/public key mismatch")
	}
	return &VAPIDKeys{priv: priv}, nil
}

// PublicBytes returns the 65-byte uncompressed public point.
func (v *VAPIDKeys) PublicBytes() []byte {
	return elliptic.Marshal(elliptic.P256(), v.priv.PublicKey.X, v.priv.PublicKey.Y)
}

// PublicB64 returns the applicationServerKey as base64url.
func (v *VAPIDKeys) PublicB64() string { return b64(v.PublicBytes()) }

// SignJWT builds an ES256 JWT for the push service audience (scheme://host of
// the subscription endpoint), expiring 12 hours after issuedAt.
func (v *VAPIDKeys) SignJWT(audience, subject string, issuedAt time.Time) (string, error) {
	if !strings.HasPrefix(audience, "https://") && !strings.HasPrefix(audience, "http://") {
		return "", errors.New("audience must be an origin URL")
	}
	header := `{"typ":"JWT","alg":"ES256"}`
	payload, err := json.Marshal(map[string]any{
		"aud": audience,
		"exp": issuedAt.Add(12 * time.Hour).Unix(),
		"sub": subject,
	})
	if err != nil {
		return "", err
	}
	signingInput := b64url([]byte(header)) + "." + b64url(payload)
	sum := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, v.priv, sum[:])
	if err != nil {
		return "", err
	}
	// ES256 uses the raw R||S concatenation (64 bytes), not ASN.1.
	sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return signingInput + "." + b64url(sig), nil
}

func b64(b []byte) string    { return base64.RawURLEncoding.EncodeToString(b) }
func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func b64dec(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
}
