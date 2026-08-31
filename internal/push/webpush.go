// Package push implements W3C Web Push (RFC 8030/8291/8188) payload
// encryption and VAPID-authenticated delivery using only the Go standard
// library. No browser push SDK or third-party crypto dependency is required.
package push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MaxPayload is the Web Push recommended message ceiling (bytes).
const MaxPayload = 3993

// recordSize is the aes128gcm record size advertised in the header.
const recordSize = 4096

// Subscription is a browser PushSubscription serialized by the PWA.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// ErrGone signals that the push service permanently rejected the endpoint
// (404/410) and the subscription should be deleted.
var ErrGone = errors.New("push subscription is gone")

// hkdfExtract implements RFC 5869 Extract with HMAC-SHA-256.
func hkdfExtract(salt, ikm []byte) []byte {
	m := hmac.New(sha256.New, salt)
	m.Write(ikm)
	return m.Sum(nil)
}

// hkdfExpand implements RFC 5869 Expand with HMAC-SHA-256.
func hkdfExpand(prk, info []byte, length int) []byte {
	var out, t []byte
	for i := 1; len(out) < length; i++ {
		m := hmac.New(sha256.New, prk)
		m.Write(t)
		m.Write(info)
		m.Write([]byte{byte(i)})
		t = m.Sum(nil)
		out = append(out, t...)
	}
	return out[:length]
}

func decodeB64URL(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	// Tolerate standard base64 from sloppy clients.
	if dec, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return dec, nil
	}
	return base64.URLEncoding.DecodeString(s)
}

// derive performs the RFC 8291 §3.4 + RFC 8188 §2.2 key schedule and returns
// the content encryption key and 12-byte nonce.
func derive(uaPublic, asPublic, authSecret, ecdhSecret, salt []byte) (cek, nonce []byte) {
	keyInfo := bytes.Join([][]byte{
		[]byte("WebPush: info\x00"), uaPublic, asPublic,
	}, nil)
	prkKey := hkdfExtract(authSecret, ecdhSecret)
	ikm := hkdfExpand(prkKey, keyInfo, 32)

	prk := hkdfExtract(salt, ikm)
	cek = hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce = hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)
	return cek, nonce
}

// encryptWith deterministically encrypts payload given an ephemeral private
// scalar and salt (used by tests); production callers use Encrypt.
func encryptWith(sub Subscription, ephemeralScalar, salt, payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}
	if len(payload) > MaxPayload {
		return nil, fmt.Errorf("payload %d bytes exceeds Web Push limit %d", len(payload), MaxPayload)
	}
	uaPublic, err := decodeB64URL(sub.P256DH)
	if err != nil || len(uaPublic) != 65 {
		return nil, errors.New("invalid p256dh key")
	}
	authSecret, err := decodeB64URL(sub.Auth)
	if err != nil || len(authSecret) != 16 {
		return nil, errors.New("invalid auth secret (want 16 bytes)")
	}
	uaPub, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("ua public: %w", err)
	}
	epriv, err := ecdh.P256().NewPrivateKey(ephemeralScalar)
	if err != nil {
		return nil, fmt.Errorf("ephemeral key: %w", err)
	}
	asPublic := epriv.PublicKey().Bytes() // 65-byte uncompressed
	ecdhSecret, err := epriv.ECDH(uaPub)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}

	cek, nonce := derive(uaPublic, asPublic, authSecret, ecdhSecret, salt)
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// Single record: plaintext followed by the last-record delimiter 0x02.
	record := append(append([]byte{}, payload...), 0x02)
	sealed := gcm.Seal(nil, nonce, record, nil)

	var buf bytes.Buffer
	buf.Write(salt)
	_ = binary.Write(&buf, binary.BigEndian, uint32(recordSize))
	buf.WriteByte(byte(len(asPublic)))
	buf.Write(asPublic)
	buf.Write(sealed)
	return buf.Bytes(), nil
}

// Encrypt produces an aes128gcm body for a subscription with a fresh random
// salt and ephemeral key.
func Encrypt(sub Subscription, payload []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	epriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return encryptWith(sub, epriv.Bytes(), salt, payload)
}

// Decrypt reverses Encrypt using the receiver (UA) private scalar and auth
// secret; it exists for tests and for operators debugging payloads locally.
func Decrypt(uaPrivateScalar, authSecret, body []byte) ([]byte, error) {
	if len(body) < 16+4+1+65 {
		return nil, errors.New("body too short")
	}
	salt := body[:16]
	rs := binary.BigEndian.Uint32(body[16:20])
	if rs < 20 {
		return nil, errors.New("bad record size")
	}
	idLen := int(body[20])
	if len(body) < 21+idLen {
		return nil, errors.New("truncated key id")
	}
	asPublic := body[21 : 21+idLen]
	sealed := body[21+idLen:]

	upriv, err := ecdh.P256().NewPrivateKey(uaPrivateScalar)
	if err != nil {
		return nil, err
	}
	asPub, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		return nil, err
	}
	shared, err := upriv.ECDH(asPub)
	if err != nil {
		return nil, err
	}
	uaPublic := upriv.PublicKey().Bytes()
	cek, nonce := derive(uaPublic, asPublic, authSecret, shared, salt)
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	record, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, err
	}
	if len(record) == 0 || record[len(record)-1] != 0x02 {
		return nil, errors.New("bad final-record delimiter")
	}
	return record[:len(record)-1], nil
}

// originOf returns scheme://host of a push endpoint.
func originOf(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid endpoint URL")
	}
	return u.Scheme + "://" + u.Host, nil
}

// Send encrypts payload and delivers it to the push service. It returns the
// HTTP status; 404/410 are reported as ErrGone so callers can prune devices.
func Send(client *http.Client, sub Subscription, vapid *VAPIDKeys, subject string, payload []byte, ttl int) (int, error) {
	body, err := Encrypt(sub, payload)
	if err != nil {
		return 0, err
	}
	origin, err := originOf(sub.Endpoint)
	if err != nil {
		return 0, err
	}
	jwt, err := vapid.SignJWT(origin, subject, time.Now())
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest(http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", strconv.Itoa(ttl))
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", jwt, vapid.PublicB64()))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer io.Copy(io.Discard, resp.Body)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return resp.StatusCode, ErrGone
	}
	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, fmt.Errorf("push service %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}
	return resp.StatusCode, nil
}
