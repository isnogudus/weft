package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// adminClient logs in as the admin and runs the setup wizard's bootstrap, in
// that order: since the wizard lives inside the admin session, there is no way
// to provision the directory before logging in.
func adminClient(t *testing.T, ts *httptest.Server) *client {
	t.Helper()
	admin := newClient(t, ts.URL)
	if code := admin.login("admin", "rootpw"); code != 200 {
		t.Fatalf("admin login: %d", code)
	}
	if resp, b := admin.do(http.MethodPost, "/api/setup/bootstrap", nil); resp.StatusCode != 200 {
		t.Fatalf("bootstrap: %d %s", resp.StatusCode, b)
	}
	return admin
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
	admin := adminClient(t, ts)

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
	admin := adminClient(t, ts)
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

// The admin's own password is written to admin_dn, not to UserDN(admin_uid):
// with admin_dn pointing at a real entry outside ou=people, the latter does not
// exist and the change used to fail with "nicht gefunden".
func TestAdminChangesOwnPassword(t *testing.T) {
	ts := testServer(t)
	admin := adminClient(t, ts)
	if resp, b := admin.do(http.MethodPost, "/api/me/password", passwordReq{OldPassword: "rootpw", NewPassword: "newadminpass56"}); resp.StatusCode != 200 {
		t.Fatalf("admin change own pw: %d %s", resp.StatusCode, b)
	}
	again := newClient(t, ts.URL)
	if code := again.login("admin", "rootpw"); code != http.StatusUnauthorized {
		t.Fatalf("old admin password: want 401, got %d", code)
	}
	if code := again.login("admin", "newadminpass56"); code != 200 {
		t.Fatalf("new admin password: %d", code)
	}
}

// When admin_dn is the server's rootdn, its password is rootpw/olcRootPW and
// weft cannot change it -- whether or not the rootdn also exists as an entry.
// The change must be refused, and the old password must keep working.
func TestAdminRootDNPasswordIsRefused(t *testing.T) {
	for name, mode := range map[string]fake.AdminMode{
		"synthetic":  fake.AdminRootSynthetic,
		"with entry": fake.AdminRootWithEntry,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.BaseDN = "dc=example,dc=org"
			cfg.CookieSecure = false
			f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10999}, idalloc.Range{Min: 20000, Max: 20999})
			f.SetAdminMode(mode)
			srv := New(cfg, f, nil)
			ts := httptest.NewServer(srv.Handler())
			t.Cleanup(func() { ts.Close(); srv.Close() })

			admin := adminClient(t, ts)
			resp, b := admin.do(http.MethodPost, "/api/me/password", passwordReq{OldPassword: "rootpw", NewPassword: "newadminpass56"})
			if resp.StatusCode != http.StatusConflict {
				t.Fatalf("admin change own pw: want 409, got %d %s", resp.StatusCode, b)
			}
			// The session still works with the unchanged password.
			if resp, b := admin.do(http.MethodGet, "/api/users", nil); resp.StatusCode != 200 {
				t.Fatalf("admin session after refusal: %d %s", resp.StatusCode, b)
			}
			if code := newClient(t, ts.URL).login("admin", "rootpw"); code != 200 {
				t.Fatalf("old admin password: %d", code)
			}
		})
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
	// The admin uid is rejected at login.
	if code := c.login("admin", "rootpw"); code != http.StatusForbidden {
		t.Fatalf("admin login with allow_admin=false: want 403, got %d", code)
	}
	// And with no admin session, the wizard is out of reach: this config
	// requires a directory that is already provisioned (logged at startup).
	if resp, _ := c.do(http.MethodPost, "/api/setup/bootstrap", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bootstrap without session: want 401, got %d", resp.StatusCode)
	}
}

// TestSetupRunsInsideAdminSession pins the property that motivated the design:
// nothing about the directory's contents is exposed before login, and the
// bootstrap is not an unauthenticated rootpw oracle.
func TestSetupRunsInsideAdminSession(t *testing.T) {
	ts := testServer(t)
	c := newClient(t, ts.URL)

	_, b := c.do(http.MethodGet, "/api/setup/status", nil)
	var st setupStatusDTO
	_ = json.Unmarshal(b, &st)
	if !st.Reachable || st.AdminUID != "admin" {
		t.Fatalf("setup status: %+v", st)
	}
	if bytes.Contains(b, []byte("provisioned")) {
		t.Fatalf("pre-login status must not report directory contents: %s", b)
	}
	if resp, _ := c.do(http.MethodPost, "/api/setup/bootstrap", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bootstrap without session: want 401, got %d", resp.StatusCode)
	}

	// The admin login reports the missing base structure; bootstrap clears it.
	resp, b := c.do(http.MethodPost, "/api/login", loginReq{Username: "admin", Password: "rootpw"})
	if resp.StatusCode != 200 {
		t.Fatalf("admin login: %d %s", resp.StatusCode, b)
	}
	var me meDTO
	_ = json.Unmarshal(b, &me)
	c.csrf = me.CSRF
	if !me.NeedsSetup {
		t.Fatal("login on an unprovisioned directory should report needsSetup")
	}
	if resp, b := c.do(http.MethodPost, "/api/setup/bootstrap", nil); resp.StatusCode != 200 {
		t.Fatalf("bootstrap: %d %s", resp.StatusCode, b)
	}
	_, b = c.do(http.MethodGet, "/api/me", nil)
	_ = json.Unmarshal(b, &me)
	if me.NeedsSetup {
		t.Fatal("needsSetup should be cleared after bootstrap")
	}

	// A non-admin never sees the wizard: their entry proves provisioning.
	c.do(http.MethodPost, "/api/users", createUserReq{UID: "bob", CN: "Bob", SN: "B", Password: "longpassword12"})
	bob := newClient(t, ts.URL)
	if code := bob.login("bob", "longpassword12"); code != 200 {
		t.Fatalf("bob login: %d", code)
	}
	_, b = bob.do(http.MethodGet, "/api/me", nil)
	_ = json.Unmarshal(b, &me)
	if me.NeedsSetup {
		t.Fatal("non-admin session should never need setup")
	}
}

func TestMetaExposesSessionTimeout(t *testing.T) {
	ts := testServer(t)
	admin := adminClient(t, ts)
	_, b := admin.do(http.MethodGet, "/api/meta", nil)
	var m metaDTO
	_ = json.Unmarshal(b, &m)
	if m.SessionTimeoutSeconds <= 0 {
		t.Fatalf("meta sessionTimeoutSeconds = %d, want > 0", m.SessionTimeoutSeconds)
	}
	if m.TestUserGenerator {
		t.Fatal("meta testUserGenerator should default to false")
	}
	if m.UserIDAttr != "uid" {
		t.Fatalf("meta userIdAttr = %q, want default uid", m.UserIDAttr)
	}
}

func TestListUsersPagination(t *testing.T) {
	ts := testServer(t)
	admin := adminClient(t, ts)

	// Names chosen so lexicographic uid order is "alice0".."alice9", "alice10"..
	// is avoided -- pad so the expected order is unambiguous.
	for i := 0; i < 23; i++ {
		uid := fmt.Sprintf("alice%02d", i)
		resp, b := admin.do(http.MethodPost, "/api/users", createUserReq{
			UID: uid, CN: uid, SN: "A", Password: "longpassword12",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create %s: %d %s", uid, resp.StatusCode, b)
		}
	}

	// Default page size (25) with only 23 users: one page holds everything.
	_, b := admin.do(http.MethodGet, "/api/users", nil)
	var all userListDTO
	if err := json.Unmarshal(b, &all); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, b)
	}
	if all.Total != 23 || len(all.Users) != 23 || all.Page != 1 || all.PageSize != 25 {
		t.Fatalf("unpaged listing = %+v", all)
	}
	if all.Users[0].UID != "alice00" || all.Users[22].UID != "alice22" {
		t.Fatalf("sort order wrong: first=%q last=%q", all.Users[0].UID, all.Users[22].UID)
	}

	// pageSize=10: 3 pages, last one partial.
	_, b = admin.do(http.MethodGet, "/api/users?pageSize=10", nil)
	var p1 userListDTO
	_ = json.Unmarshal(b, &p1)
	if p1.Total != 23 || len(p1.Users) != 10 || p1.Users[0].UID != "alice00" || p1.Users[9].UID != "alice09" {
		t.Fatalf("page 1: %+v", p1)
	}

	_, b = admin.do(http.MethodGet, "/api/users?pageSize=10&page=2", nil)
	var p2 userListDTO
	_ = json.Unmarshal(b, &p2)
	if len(p2.Users) != 10 || p2.Users[0].UID != "alice10" || p2.Users[9].UID != "alice19" {
		t.Fatalf("page 2: %+v", p2)
	}

	_, b = admin.do(http.MethodGet, "/api/users?pageSize=10&page=3", nil)
	var p3 userListDTO
	_ = json.Unmarshal(b, &p3)
	if len(p3.Users) != 3 || p3.Users[0].UID != "alice20" || p3.Users[2].UID != "alice22" {
		t.Fatalf("page 3 (partial): %+v", p3)
	}

	// Past the last page: empty, not an error.
	_, b = admin.do(http.MethodGet, "/api/users?pageSize=10&page=4", nil)
	var p4 userListDTO
	_ = json.Unmarshal(b, &p4)
	if len(p4.Users) != 0 || p4.Total != 23 {
		t.Fatalf("page past the end: %+v", p4)
	}

	// pageSize is clamped, not rejected.
	_, b = admin.do(http.MethodGet, "/api/users?pageSize=100000", nil)
	var clamped userListDTO
	_ = json.Unmarshal(b, &clamped)
	if clamped.PageSize != maxUserPageSize {
		t.Fatalf("pageSize not clamped: %+v", clamped)
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

// TestHealth covers the endpoint a supervisor polls: the status code follows
// the directory, the body describes nothing else, and no session is needed.
func TestHealth(t *testing.T) {
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	cfg.CookieSecure = false
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10999}, idalloc.Range{Min: 20000, Max: 20999})
	srv := New(cfg, f, nil)
	srv.healthTTL = 0 // probe every call, so the test sees state changes at once
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })

	c := newClient(t, ts.URL)
	resp, b := c.do(http.MethodGet, "/api/healthz", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("healthz: %d %s", resp.StatusCode, b)
	}
	var h healthDTO
	_ = json.Unmarshal(b, &h)
	if h.Status != "ok" || h.LDAP != "up" {
		t.Fatalf("healthz body: %+v", h)
	}
	// Unauthenticated, so it must not describe the deployment.
	for _, leak := range []string{cfg.BaseDN, cfg.AdminBindDN(), cfg.LDAPURL} {
		if leak != "" && bytes.Contains(b, []byte(leak)) {
			t.Fatalf("healthz body leaks %q: %s", leak, b)
		}
	}

	f.SetUnreachable(true)
	resp, b = c.do(http.MethodGet, "/api/healthz", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("healthz while LDAP is down: want 503, got %d %s", resp.StatusCode, b)
	}
	_ = json.Unmarshal(b, &h)
	if h.Status != "down" || h.LDAP != "down" {
		t.Fatalf("healthz body while down: %+v", h)
	}
	// The SPA's own probe agrees, and recovery is picked up.
	_, b = c.do(http.MethodGet, "/api/setup/status", nil)
	var st setupStatusDTO
	_ = json.Unmarshal(b, &st)
	if st.Reachable {
		t.Fatal("setup/status should report unreachable while LDAP is down")
	}
	f.SetUnreachable(false)
	if resp, _ := c.do(http.MethodGet, "/api/healthz", nil); resp.StatusCode != 200 {
		t.Fatalf("healthz after recovery: %d", resp.StatusCode)
	}
}

// TestHealthProbeIsCached pins that polling does not mean one LDAP connection
// per request: within the TTL the cached verdict is reused.
func TestHealthProbeIsCached(t *testing.T) {
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	cfg.CookieSecure = false
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10999}, idalloc.Range{Min: 20000, Max: 20999})
	srv := New(cfg, f, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })

	c := newClient(t, ts.URL)
	c.do(http.MethodGet, "/api/healthz", nil) // primes the cache
	f.SetUnreachable(true)
	if resp, _ := c.do(http.MethodGet, "/api/healthz", nil); resp.StatusCode != 200 {
		t.Fatal("a probe within the TTL should reuse the cached verdict")
	}
	srv.healthTTL = 0
	if resp, _ := c.do(http.MethodGet, "/api/healthz", nil); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("once the cache expires the probe must run again")
	}
}
