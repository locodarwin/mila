package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// recordSize is the one record a push message is sealed into (RFC 8188).
// Push services accept at least 4096 octets of body.
const recordSize = 4096

// headerSize is the aes128gcm header: salt, record size, key ID length
// and the 65-octet public key used as the key ID.
const headerSize = 16 + 4 + 1 + 65

var b64 = base64.RawURLEncoding

// decodeKey reads base64url with or without padding, as browsers send it.
func decodeKey(s string) ([]byte, error) {
	for len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return b64.DecodeString(s)
}

// encrypt seals plaintext for one browser as RFC 8291 says: an ECDH
// agreement between the sender's one-time key and the browser's key,
// mixed with the browser's auth secret, gives the content key and nonce
// for a single aes128gcm record. salt and ephemeral are fresh for every
// message; tests pass the RFC's own.
func encrypt(plaintext, uaPublic, authSecret, salt []byte, ephemeral *ecdh.PrivateKey) ([]byte, error) {
	if len(authSecret) != 16 {
		return nil, errors.New("push: auth secret is not 16 octets")
	}
	if len(salt) != 16 {
		return nil, errors.New("push: salt is not 16 octets")
	}
	if len(plaintext)+1+aes.BlockSize > recordSize {
		return nil, errors.New("push: message too long")
	}
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("push: browser key: %w", err)
	}
	secret, err := ephemeral.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPublic := ephemeral.PublicKey().Bytes()

	// IKM = HKDF(auth_secret, ecdh_secret, "WebPush: info" || 0x00 || ua_public || as_public, 32)
	keyInfo := append(append([]byte("WebPush: info\x00"), uaPublic...), asPublic...)
	ikm, err := hkdf.Key(sha256.New, secret, authSecret, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, headerSize+len(plaintext)+1+gcm.Overhead())
	out = append(out, salt...)
	out = binary.BigEndian.AppendUint32(out, recordSize)
	out = append(out, byte(len(asPublic)))
	out = append(out, asPublic...)
	// 0x02 marks the last (and only) record; no padding follows.
	record := append(append(make([]byte, 0, len(plaintext)+1), plaintext...), 2)
	return gcm.Seal(out, nonce, record, nil), nil
}

// seal is encrypt with a fresh salt and one-time key.
func seal(plaintext, uaPublic, authSecret []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return encrypt(plaintext, uaPublic, authSecret, salt, ephemeral)
}

// vapidTTL is how long a signed VAPID token is good for; push services
// refuse more than a day.
const vapidTTL = 12 * time.Hour

// vapid is the Authorization header that identifies this server to a push
// service (RFC 8292): a JWT signed with ES256 for the endpoint's origin,
// and the public key that checks it.
func vapid(key *ecdsa.PrivateKey, audience, subject string, now time.Time) (string, error) {
	header := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}{audience, now.Add(vapidTTL).Unix(), subject})
	if err != nil {
		return "", err
	}
	signing := header + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	// JWS wants r and s as two 32-octet big-endian numbers, not ASN.1.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		return "", err
	}
	return "vapid t=" + signing + "." + b64.EncodeToString(sig) + ", k=" + b64.EncodeToString(pub), nil
}
