package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/syncer"
)

// samplePNG is an opaque w×w picture.
func samplePNG(t *testing.T, w int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, w))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7)
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// putAvatar sends a picture as the app does.
func (c *client) putAvatar(data []byte) (*http.Response, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "picture.png")
	_, _ = part.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("PUT", c.base+"/me/avatar", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

// fetch GETs a path below the server's root (the avatar paths the API
// gives are absolute) and returns the answer with its body.
func (c *client) fetch(path string, header http.Header) (*http.Response, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", strings.TrimSuffix(c.base, "/api/v1")+path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

func TestAvatarsOverHTTP(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		s := newServerOn(t, d)
		owner := s.browser(t)
		s.setup(t, owner)
		_, ws := owner.do("POST", "/workspaces", map[string]string{"name": "Team"})
		team := ws["id"].(string)
		dana := s.join(t, owner, team, "dana@example.com", "Dana", "member")
		gus := s.join(t, owner, team, "gus@example.com", "Gus", "guest")
		_, ws = owner.do("POST", "/workspaces", map[string]string{"name": "Other"})
		sam := s.join(t, owner, ws["id"].(string), "sam@example.com", "Sam", "member")
		_, me := dana.do("GET", "/me", nil)
		danaID := me["id"].(string)
		_, me = gus.do("GET", "/me", nil)
		gusID := me["id"].(string)
		if me["avatar"] != nil {
			t.Fatalf("a new account has a picture: %v", me["avatar"])
		}

		// Dana sets a picture.
		resp, user := dana.putAvatar(samplePNG(t, 300))
		path, _ := user["avatar"].(string)
		if resp.StatusCode != 200 || !strings.HasPrefix(path, "/api/v1/avatars/") || len(path) != len("/api/v1/avatars/")+32 {
			t.Fatalf("upload = %d %v", resp.StatusCode, user)
		}
		key := strings.TrimPrefix(path, "/api/v1/avatars/")

		// The owner, who shares Team with her, sees it.
		got, raw := owner.fetch(path, nil)
		if got.StatusCode != 200 || got.Header.Get("Content-Type") != "image/jpeg" ||
			got.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" ||
			got.Header.Get("ETag") != `"`+key+`"` ||
			got.Header.Get("Content-Security-Policy") != "default-src 'none'; sandbox" ||
			got.Header.Get("Cross-Origin-Resource-Policy") != "same-origin" {
			t.Fatalf("picture = %d %v", got.StatusCode, got.Header)
		}
		if img, err := jpeg.Decode(bytes.NewReader(raw)); err != nil || img.Bounds().Dx() != 256 {
			t.Fatalf("the picture served is not a 256 px JPEG: %v", err)
		}
		if got, _ := owner.fetch(path, http.Header{"If-None-Match": {`"` + key + `"`}}); got.StatusCode != http.StatusNotModified {
			t.Fatalf("a revalidation = %d", got.StatusCode)
		}
		// So does the guest; a stranger, a signed-out browser and a key
		// that is no key do not.
		if got, _ := gus.fetch(path, nil); got.StatusCode != 200 {
			t.Fatalf("the guest = %d", got.StatusCode)
		}
		if got, _ := sam.fetch(path, nil); got.StatusCode != 404 {
			t.Fatalf("a stranger = %d", got.StatusCode)
		}
		if got, _ := s.browser(t).fetch(path, nil); got.StatusCode != 401 {
			t.Fatalf("signed out = %d", got.StatusCode)
		}
		if got, _ := owner.fetch("/api/v1/avatars/not-a-key", nil); got.StatusCode != 404 {
			t.Fatalf("a malformed key = %d", got.StatusCode)
		}

		// Every list of people carries it.
		_, members := owner.doList("GET", "/workspaces/"+team+"/members")
		found := false
		for _, m := range members {
			if m["userId"] == danaID {
				found = m["avatar"] == path
			} else if _, ok := m["avatar"]; !ok {
				t.Errorf("a member without a picture has no avatar field: %v", m)
			}
		}
		if !found {
			t.Errorf("members = %v", members)
		}
		_, page := owner.do("GET", "/me/notifications", nil)
		found = false
		for _, n := range page["items"].([]any) {
			n := n.(map[string]any)
			if n["actorName"] == "Dana" {
				found = n["actorAvatar"] == path
			}
		}
		if !found {
			t.Errorf("notifications = %v", page)
		}
		_, users := owner.doList("GET", "/admin/users")
		found = false
		for _, u := range users {
			if u["id"] == danaID {
				found = u["avatar"] == path
			}
		}
		if !found {
			t.Errorf("admin users = %v", users)
		}
		if resp, _ := gus.putAvatar(samplePNG(t, 64)); resp.StatusCode != 200 {
			t.Fatalf("the guest's upload = %d", resp.StatusCode)
		}
		project := uuid.NewString()
		owner.do("POST", "/workspaces/"+team+"/sync/push", map[string]any{"schema": syncer.Schema, "ops": []any{
			map[string]any{"opId": uuid.NewString(), "entity": "project", "id": project, "hlc": hlcNow("owner"),
				"fields": map[string]any{"name": "Garden", "sort_key": "a0"}},
		}})
		_, guests := owner.doList("GET", "/workspaces/"+team+"/projects/"+project+"/guests")
		if len(guests) != 1 || !strings.HasPrefix(guests[0]["avatar"].(string), "/api/v1/avatars/") {
			t.Fatalf("project guests = %v", guests)
		}

		// Only an admin takes someone else's picture down.
		if resp, _ := dana.do("DELETE", "/admin/users/"+gusID+"/avatar", nil); resp.StatusCode != 403 {
			t.Fatalf("a member taking a picture down = %d", resp.StatusCode)
		}
		if resp, body := owner.do("DELETE", "/admin/users/"+gusID+"/avatar", nil); resp.StatusCode != 200 || body["avatar"] != nil || body["id"] != gusID {
			t.Fatalf("an admin taking a picture down = %d %v", resp.StatusCode, body)
		}

		// Removing it ends the old URL at once.
		if resp, body := dana.do("DELETE", "/me/avatar", nil); resp.StatusCode != 200 || body["avatar"] != nil {
			t.Fatalf("remove = %d %v", resp.StatusCode, body)
		}
		if got, _ := owner.fetch(path, nil); got.StatusCode != 404 {
			t.Fatalf("a removed picture = %d", got.StatusCode)
		}

		// What is refused.
		dana.origin = "https://evil.example"
		if resp, body := dana.putAvatar(samplePNG(t, 64)); resp.StatusCode != 403 || body["error"] != "bad_origin" {
			t.Fatalf("a cross-origin upload = %d %v", resp.StatusCode, body)
		}
		dana.origin = s.URL
		big := append(samplePNG(t, 64), make([]byte, 4<<20)...)
		if resp, body := dana.putAvatar(big); resp.StatusCode != http.StatusRequestEntityTooLarge || body["error"] != "too_large" {
			t.Fatalf("over 4 MB = %d %v", resp.StatusCode, body)
		}
		var jb bytes.Buffer
		_ = jpeg.Encode(&jb, image.NewNRGBA(image.Rect(0, 0, 64, 64)), nil)
		if resp, body := dana.putAvatar(jb.Bytes()); resp.StatusCode != 400 || !strings.Contains(body["message"].(string), "PNG") {
			t.Fatalf("a JPEG = %d %v", resp.StatusCode, body)
		}
		limited := false
		for range 12 {
			resp, _ := dana.putAvatar(samplePNG(t, 16))
			if resp.StatusCode == http.StatusTooManyRequests {
				limited = resp.Header.Get("Retry-After") != ""
				break
			}
		}
		if !limited {
			t.Fatal("no limit after a burst of uploads")
		}
		// Removing counts too, since each change tells everyone.
		if resp, body := dana.do("DELETE", "/me/avatar", nil); resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("a removal past the limit = %d %v", resp.StatusCode, body)
		}
		// Renaming has a limit of its own.
		renamed := false
		for i := range 12 {
			resp, _ := gus.do("PATCH", "/me", map[string]string{"name": "Gus " + strconv.Itoa(i)})
			if resp.StatusCode == http.StatusTooManyRequests {
				renamed = resp.Header.Get("Retry-After") != ""
				break
			}
		}
		if !renamed {
			t.Fatal("no limit after a burst of renames")
		}

		// Signing out and deleting the account clear the browser's cache.
		resp, _ = sam.do("POST", "/me/delete", map[string]string{"email": "sam@example.com"})
		if resp.StatusCode != 204 || resp.Header.Get("Clear-Site-Data") != `"cache"` {
			t.Fatalf("account deletion = %d %v", resp.StatusCode, resp.Header)
		}
		resp, _ = gus.do("POST", "/auth/sign-out", nil)
		if resp.StatusCode != 204 || resp.Header.Get("Clear-Site-Data") != `"cache"` {
			t.Fatalf("sign-out = %d %v", resp.StatusCode, resp.Header)
		}
		// A sign-out with no session left still answers, and still clears.
		resp, _ = gus.do("POST", "/auth/sign-out", nil)
		if resp.StatusCode != 204 || resp.Header.Get("Clear-Site-Data") != `"cache"` {
			t.Fatalf("a second sign-out = %d %v", resp.StatusCode, resp.Header)
		}
	})
}
