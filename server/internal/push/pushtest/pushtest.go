// Package pushtest is a fake push service for tests. It plays a browser
// and its push service at once: it hands out subscriptions with keys it
// holds, and decrypts and checks every message sent to them the way a
// browser would (RFC 8291), after checking the VAPID signature (RFC 8292).
package pushtest

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var b64 = base64.RawURLEncoding

// Message is a push as a browser would receive it.
type Message struct {
	Device  string // the subscription it reached
	Title   string `json:"title"`
	Body    string `json:"body"`
	URL     string `json:"url"`
	Tag     string `json:"tag"`
	Urgency string
	TTL     string
	Raw     []byte // the decrypted payload
}

// Device is one subscribed browser's half: the keys it keeps secret.
type device struct {
	key    *ecdh.PrivateKey
	secret []byte
}

// Server is the fake push service.
type Server struct {
	*httptest.Server
	t *testing.T

	mu       sync.Mutex
	devices  map[string]device
	status   map[string]int // answer for a device; 0 means 201
	received []Message
	// Key, when set, is the VAPID public key every request must carry.
	Key string
}

// New starts a fake push service, stopped when the test ends.
func New(t *testing.T) *Server {
	s := &Server{t: t, devices: map[string]device{}, status: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

// Subscription is what a browser's PushSubscription.toJSON() gives.
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Subscribe makes a new browser subscription named name.
func (s *Server) Subscribe(name string) Subscription {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		s.t.Fatal(err)
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	s.mu.Lock()
	s.devices[name] = device{key: key, secret: secret}
	s.mu.Unlock()
	return Subscription{
		Endpoint: s.URL + "/send/" + name,
		P256dh:   b64.EncodeToString(key.PublicKey().Bytes()),
		Auth:     b64.EncodeToString(secret),
	}
}

// Answer makes the service answer status for a device; 0 restores 201.
func (s *Server) Answer(name string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[name] = status
}

// Received returns the messages delivered so far and forgets them.
func (s *Server) Received() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.received
	s.received = nil
	return out
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutPrefix(r.URL.Path, "/send/")
	s.mu.Lock()
	dev, known := s.devices[name]
	status := s.status[name]
	s.mu.Unlock()
	if r.Method != http.MethodPost || !ok || !known {
		http.Error(w, "no such subscription", http.StatusNotFound)
		return
	}
	if status != 0 && status != http.StatusCreated {
		http.Error(w, "refused", status)
		return
	}
	if err := s.checkVAPID(r); err != nil {
		s.t.Errorf("pushtest: %v", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if ce := r.Header.Get("Content-Encoding"); ce != "aes128gcm" {
		s.t.Errorf("pushtest: Content-Encoding = %q", ce)
	}
	body, _ := io.ReadAll(r.Body)
	plain, err := Decrypt(body, dev.key, dev.secret)
	if err != nil {
		s.t.Errorf("pushtest: decrypt: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	m := Message{Device: name, Urgency: r.Header.Get("Urgency"), TTL: r.Header.Get("TTL"), Raw: plain}
	if err := json.Unmarshal(plain, &m); err != nil {
		s.t.Errorf("pushtest: payload: %v", err)
	}
	s.mu.Lock()
	s.received = append(s.received, m)
	s.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

// checkVAPID verifies the Authorization header: an ES256 JWT for this
// origin, not expired and good for at most a day, signed by key k.
func (s *Server) checkVAPID(r *http.Request) error {
	h, ok := strings.CutPrefix(r.Header.Get("Authorization"), "vapid ")
	if !ok {
		return errors.New("no vapid authorization")
	}
	var token, k string
	for part := range strings.SplitSeq(h, ",") {
		part = strings.TrimSpace(part)
		if v, ok := strings.CutPrefix(part, "t="); ok {
			token = v
		} else if v, ok := strings.CutPrefix(part, "k="); ok {
			k = v
		}
	}
	if s.Key != "" && k != s.Key {
		return fmt.Errorf("vapid key %q, want %q", k, s.Key)
	}
	pubRaw, err := b64.DecodeString(k)
	if err != nil {
		return err
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pubRaw)
	if err != nil {
		return err
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("malformed jwt")
	}
	var header struct{ Typ, Alg string }
	if raw, err := b64.DecodeString(parts[0]); err != nil || json.Unmarshal(raw, &header) != nil || header.Alg != "ES256" {
		return fmt.Errorf("jwt header %s", parts[0])
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return errors.New("jwt signature is not 64 octets")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return errors.New("jwt signature does not verify")
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	raw, err := b64.DecodeString(parts[1])
	if err != nil || json.Unmarshal(raw, &claims) != nil {
		return errors.New("jwt claims")
	}
	if claims.Aud != s.URL {
		return fmt.Errorf("jwt aud %q, want %q", claims.Aud, s.URL)
	}
	exp := time.Unix(claims.Exp, 0)
	if time.Until(exp) <= 0 || time.Until(exp) > 24*time.Hour {
		return fmt.Errorf("jwt exp %v", exp)
	}
	if !strings.HasPrefix(claims.Sub, "mailto:") && !strings.HasPrefix(claims.Sub, "https:") && !strings.HasPrefix(claims.Sub, "http:") {
		return fmt.Errorf("jwt sub %q", claims.Sub)
	}
	return nil
}

// Decrypt opens an aes128gcm message (RFC 8188) sealed for a browser
// holding key and secret (RFC 8291), as the browser does.
func Decrypt(body []byte, key *ecdh.PrivateKey, secret []byte) ([]byte, error) {
	if len(body) < 21 {
		return nil, errors.New("short body")
	}
	salt, rs, idlen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	if len(body) < 21+idlen {
		return nil, errors.New("short header")
	}
	asPublic, record := body[21:21+idlen], body[21+idlen:]
	if len(record) > int(rs) {
		return nil, fmt.Errorf("record of %d octets over the record size %d", len(record), rs)
	}
	as, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		return nil, err
	}
	shared, err := key.ECDH(as)
	if err != nil {
		return nil, err
	}
	uaPublic := key.PublicKey().Bytes()
	info := append(append([]byte("WebPush: info\x00"), uaPublic...), asPublic...)
	ikm, err := hkdf.Key(sha256.New, shared, secret, string(info), 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
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
	plain, err := gcm.Open(nil, nonce, record, nil)
	if err != nil {
		return nil, err
	}
	// Strip padding back to the delimiter; 0x02 marks the last record.
	i := len(plain) - 1
	for i >= 0 && plain[i] == 0 {
		i--
	}
	if i < 0 || plain[i] != 2 {
		return nil, errors.New("no last-record delimiter")
	}
	return plain[:i], nil
}
