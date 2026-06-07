package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"weft/internal/config"
	"weft/internal/directory/fake"
	"weft/internal/idalloc"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, c.base+path, &buf)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		req.Header.Set(csrfHeader, c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out := new(bytes.Buffer)
	_, _ = out.ReadFrom(resp.Body)
	return resp, out.Bytes()
}

func (c *client) login(uid, pw string) int {
	resp, b := c.do(http.MethodPost, "/api/login", loginReq{Username: uid, Password: pw})
	if resp.StatusCode == http.StatusOK {
		var me meDTO
		_ = json.Unmarshal(b, &me)
		c.csrf = me.CSRF
	}
	return resp.StatusCode
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	cfg.CookieSecure = false
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10999}, idalloc.Range{Min: 20000, Max: 20999})
	srv := New(cfg, f, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })
	return ts
}

func TestFullFlow(t *testing.T) {
	ts := testServer(t)
	admin := newClient(t, ts.URL)

	// Bootstrap, then admin login.
	if resp, _ := admin.do(http.MethodPost, "/api/setup/bootstrap", bootstrapReq{Password: "rootpw"}); resp.StatusCode != 200 {
		t.Fatalf("bootstrap: %d", resp.StatusCode)
	}
	if code := admin.login("admin", "rootpw"); code != 200 {
		t.Fatalf("admin login: %d", code)
	}

	// Create a POSIX user with defaults.
	resp, b := admin.do(http.MethodPost, "/api/users", createUserReq{
		UID: "alice", CN: "Alice", SN: "Ex", Password: "sw0rdfish-long", POSIX: &posixReq{},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: %d %s", resp.StatusCode, b)
	}
	var u userDTO
	_ = json.Unmarshal(b, &u)
	if u.POSIX == nil || u.POSIX.UIDNumber != 10000 || u.POSIX.GIDNumber != 20000 {
		t.Fatalf("posix defaults wrong: %+v", u.POSIX)
	}

	// CSRF is required on writes.
	saved := admin.csrf
	admin.csrf = "bogus"
	if resp, _ := admin.do(http.MethodPost, "/api/groups", createGroupReq{CN: "devs"}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected CSRF 403, got %d", resp.StatusCode)
	}
	admin.csrf = saved

	// Group + membership + effective groups.
	admin.do(http.MethodPost, "/api/groups", createGroupReq{CN: "devs"})
	admin.do(http.MethodPost, "/api/groups/devs/members", memberReq{UID: "alice"})
	resp, b = admin.do(http.MethodGet, "/api/users/alice/groups", nil)
	var gs []groupDTO
	_ = json.Unmarshal(b, &gs)
	if len(gs) != 2 {
		t.Fatalf("expected 2 effective groups, got %d (%s)", len(gs), b)
	}
}

func TestNonAdminIsSelfServiceOnly(t *testing.T) {
	ts := testServer(t)
	admin := newClient(t, ts.URL)
	admin.do(http.MethodPost, "/api/setup/bootstrap", bootstrapReq{Password: "rootpw"})
	admin.login("admin", "rootpw")
	admin.do(http.MethodPost, "/api/users", createUserReq{UID: "bob", CN: "Bob", SN: "B", Password: "longpassword12"})

	bob := newClient(t, ts.URL)
	if code := bob.login("bob", "longpassword12"); code != 200 {
		t.Fatalf("bob login: %d", code)
	}
	// Management endpoints are forbidden.
	if resp, _ := bob.do(http.MethodGet, "/api/users", nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bob /users: want 403, got %d", resp.StatusCode)
	}
	// Self-service works.
	if resp, b := bob.do(http.MethodGet, "/api/me", nil); resp.StatusCode != 200 {
		t.Fatalf("bob /me: %d %s", resp.StatusCode, b)
	}
	if resp, b := bob.do(http.MethodPost, "/api/me/password", passwordReq{OldPassword: "longpassword12", NewPassword: "evenlongerpass34"}); resp.StatusCode != 200 {
		t.Fatalf("bob change own pw: %d %s", resp.StatusCode, b)
	}
}

func TestLoginRateLimit(t *testing.T) {
	ts := testServer(t)
	c := newClient(t, ts.URL)
	var last int
	for i := 0; i < 7; i++ {
		last = c.login("admin", "wrong")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d", last)
	}
}

func TestAdminLoginDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	cfg.CookieSecure = false
	cfg.AllowAdmin = false
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10999}, idalloc.Range{Min: 20000, Max: 20999})
	srv := New(cfg, f, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })

	c := newClient(t, ts.URL)
	// Bootstrap still works (it uses the rootpw directly, not the login path).
	if resp, _ := c.do(http.MethodPost, "/api/setup/bootstrap", bootstrapReq{Password: "rootpw"}); resp.StatusCode != 200 {
		t.Fatalf("bootstrap: %d", resp.StatusCode)
	}
	// The admin uid is rejected at login.
	if code := c.login("admin", "rootpw"); code != http.StatusForbidden {
		t.Fatalf("admin login with allow_admin=false: want 403, got %d", code)
	}
}

func TestMetaExposesSessionTimeout(t *testing.T) {
	ts := testServer(t)
	admin := newClient(t, ts.URL)
	admin.do(http.MethodPost, "/api/setup/bootstrap", bootstrapReq{Password: "rootpw"})
	admin.login("admin", "rootpw")
	_, b := admin.do(http.MethodGet, "/api/meta", nil)
	var m metaDTO
	_ = json.Unmarshal(b, &m)
	if m.SessionTimeoutSeconds <= 0 {
		t.Fatalf("meta sessionTimeoutSeconds = %d, want > 0", m.SessionTimeoutSeconds)
	}
}

func TestRequiresAuth(t *testing.T) {
	ts := testServer(t)
	c := newClient(t, ts.URL)
	if resp, _ := c.do(http.MethodGet, "/api/users", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
	_ = context.Background()
}
