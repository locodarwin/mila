package push

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/locodarwin/mila/server/internal/notify"
	"github.com/locodarwin/mila/server/internal/push/pushtest"
)

func notifyMessage(title, body string) notify.PushMessage {
	return notify.PushMessage{Title: title, Body: body, URL: "/?task=t&ws=w", Tag: "t"}
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := b64.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The worked example in RFC 8291 Appendix A, with its salt and sender key
// injected, must come out byte for byte.
func TestEncryptRFC8291Example(t *testing.T) {
	plaintext := mustB64(t, "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24")
	if string(plaintext) != "When I grow up, I want to be a watermelon" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	as, err := ecdh.P256().NewPrivateKey(mustB64(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	if got := b64.EncodeToString(as.PublicKey().Bytes()); got != "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8" {
		t.Fatalf("as_public = %s", got)
	}
	uaPublic := mustB64(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4")
	auth := mustB64(t, "BTBZMqHH6r4Tts7J_aSIgg")
	salt := mustB64(t, "DGv6ra1nlYgDCS1FRnbzlw")

	got, err := encrypt(plaintext, uaPublic, auth, salt, as)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(got) != want {
		t.Fatalf("body =\n%s\nwant\n%s", b64.EncodeToString(got), want)
	}

	// The browser's side of the example opens it again.
	ua, err := ecdh.P256().NewPrivateKey(mustB64(t, "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := pushtest.Decrypt(got, ua, auth)
	if err != nil || !bytes.Equal(plain, plaintext) {
		t.Fatalf("decrypt = %q, %v", plain, err)
	}
}

func TestSealIsFreshEachTime(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	a, err := seal([]byte("hello"), ua.PublicKey().Bytes(), auth)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := seal([]byte("hello"), ua.PublicKey().Bytes(), auth)
	if bytes.Equal(a[:16], b[:16]) || bytes.Equal(a[21:86], b[21:86]) {
		t.Fatal("salt or sender key repeated")
	}
	for _, body := range [][]byte{a, b} {
		if plain, err := pushtest.Decrypt(body, ua, auth); err != nil || string(plain) != "hello" {
			t.Fatalf("decrypt = %q, %v", plain, err)
		}
	}
	if _, err := seal(make([]byte, recordSize), ua.PublicKey().Bytes(), auth); err == nil {
		t.Fatal("a message over one record was sealed")
	}
	if _, err := seal([]byte("x"), []byte{4, 1, 2}, auth); err == nil {
		t.Fatal("a bad browser key was accepted")
	}
}

func TestVAPIDToken(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Unix(1_800_000_000, 0)
	h, err := vapid(key, "https://push.example.com", "mailto:mila@example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	rest, ok := strings.CutPrefix(h, "vapid t=")
	if !ok {
		t.Fatalf("header %q", h)
	}
	token, k, ok := strings.Cut(rest, ", k=")
	if !ok {
		t.Fatalf("header %q", h)
	}
	pub, _ := key.PublicKey.Bytes()
	if k != b64.EncodeToString(pub) {
		t.Fatalf("k = %s", k)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q", token)
	}
	var head map[string]string
	_ = json.Unmarshal(mustB64(t, parts[0]), &head)
	if head["alg"] != "ES256" || head["typ"] != "JWT" {
		t.Fatalf("header %v", head)
	}
	var claims map[string]any
	_ = json.Unmarshal(mustB64(t, parts[1]), &claims)
	if claims["aud"] != "https://push.example.com" || claims["sub"] != "mailto:mila@example.com" ||
		claims["exp"] != float64(now.Add(12*time.Hour).Unix()) {
		t.Fatalf("claims %v", claims)
	}
	sig := mustB64(t, parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature does not verify")
	}
}

func TestPayloadFits(t *testing.T) {
	long := strings.Repeat("Ünïcode \"quoted\" <b> ", 400)
	for _, m := range []struct{ title, body string }{
		{"Short", long},
		{long, ""},
		{long, long},
	} {
		raw := payload(notifyMessage(m.title, m.body))
		if len(raw) > maxPayload {
			t.Fatalf("payload of %d bytes", len(raw))
		}
		var got message
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got.URL != "/?task=t&ws=w" || got.Tag != "t" {
			t.Fatalf("payload lost its link: %+v", got)
		}
		if m.body != "" && !strings.HasSuffix(got.Body, "…") {
			t.Fatalf("body not marked as cut: %q", got.Body)
		}
		if len(m.title) > maxTitle && (utf8.RuneCountInString(got.Title) != maxTitle || !strings.HasSuffix(got.Title, "…")) {
			t.Fatalf("title not cut to %d: %q", maxTitle, got.Title)
		}
	}
	raw := payload(notifyMessage("Water the plants", "Due today"))
	if string(raw) != `{"title":"Water the plants","body":"Due today","url":"/?task=t\u0026ws=w","tag":"t"}` {
		t.Fatalf("payload = %s", raw)
	}
}

func TestSubject(t *testing.T) {
	cases := map[[2]string]string{
		{"Mila <mila@example.com>", "https://example.com"}: "mailto:mila@example.com",
		{"ops@example.com", "https://example.com"}:         "mailto:ops@example.com",
		{"", "https://mila.example.com/"}:                  "https://mila.example.com",
	}
	for in, want := range cases {
		if got := Subject(in[0], in[1]); got != want {
			t.Errorf("Subject(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}
