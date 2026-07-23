package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"weft/internal/config"
	"weft/internal/directory/fake"
	"weft/internal/idalloc"
)

// importTestServer is testServer with a fast bcrypt cost (imports hash one
// password per row) and a configurable uid range.
func importTestServer(t *testing.T, uidRange idalloc.Range) *httptest.Server {
	t.Helper()
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	cfg.CookieSecure = false
	cfg.BcryptCost = 4
	f := fake.New("rootpw", uidRange, idalloc.Range{Min: 20000, Max: 20999})
	srv := New(cfg, f, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Close() })
	return ts
}

func importAdmin(t *testing.T, ts *httptest.Server) *client {
	t.Helper()
	admin := newClient(t, ts.URL)
	if resp, _ := admin.do(http.MethodPost, "/api/setup/bootstrap", bootstrapReq{Password: "rootpw"}); resp.StatusCode != 200 {
		t.Fatalf("bootstrap: %d", resp.StatusCode)
	}
	if code := admin.login("admin", "rootpw"); code != 200 {
		t.Fatalf("admin login: %d", code)
	}
	return admin
}

func row(i int, uid string) importRowReq {
	return importRowReq{Row: i, createUserReq: createUserReq{
		UID: uid, CN: "U " + uid, SN: uid, Password: "longenough-" + uid,
	}}
}

func postImport(t *testing.T, c *client, rows []importRowReq) (int, importRespDTO) {
	t.Helper()
	resp, b := c.do(http.MethodPost, "/api/users/import", importReq{Rows: rows})
	var out importRespDTO
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func wantStatuses(t *testing.T, res importRespDTO, want ...string) {
	t.Helper()
	if len(res.Results) != len(want) {
		t.Fatalf("got %d results, want %d (%+v)", len(res.Results), len(want), res.Results)
	}
	for i, w := range want {
		if res.Results[i].Status != w {
			t.Fatalf("row %d: status %q (%s), want %q", i, res.Results[i].Status, res.Results[i].Error, w)
		}
	}
}

func TestImportHappyPath(t *testing.T) {
	ts := importTestServer(t, idalloc.Range{Min: 10000, Max: 10999})
	admin := importAdmin(t, ts)

	rows := []importRowReq{row(0, "anna"), row(1, "berta"), row(2, "clara")}
	rows[1].POSIX = &posixReq{}
	rows[2].Mail = &mailReq{Mail: "clara@example.org", Aliases: []string{"c@example.org"}}

	code, res := postImport(t, admin, rows)
	if code != 200 {
		t.Fatalf("import: %d", code)
	}
	wantStatuses(t, res, "created", "created", "created")
	if res.Results[1].User == nil || res.Results[1].User.POSIX == nil || res.Results[1].User.POSIX.UIDNumber != 10000 {
		t.Fatalf("posix row result: %+v", res.Results[1].User)
	}
	if res.Results[0].Row != 0 || res.Results[2].Row != 2 {
		t.Fatal("row indexes not echoed")
	}

	// The users exist and can log in with their imported passwords.
	c := newClient(t, ts.URL)
	if codeLogin := c.login("clara", "longenough-clara"); codeLogin != 200 {
		t.Fatalf("imported user login: %d", codeLogin)
	}
}

func TestImportConflictAndRetry(t *testing.T) {
	ts := importTestServer(t, idalloc.Range{Min: 10000, Max: 10999})
	admin := importAdmin(t, ts)

	admin.do(http.MethodPost, "/api/users", createUserReq{UID: "jdoe", CN: "J", SN: "D", Password: "longpassword12"})

	rows := []importRowReq{row(0, "jdoe"), row(1, "anna")}
	code, res := postImport(t, admin, rows)
	if code != 200 {
		t.Fatalf("import: %d", code)
	}
	wantStatuses(t, res, "exists", "created")

	// Retrying the identical chunk is safe: everything reports "exists".
	_, res = postImport(t, admin, rows)
	wantStatuses(t, res, "exists", "exists")
}

func TestImportInvalidRowContinues(t *testing.T) {
	ts := importTestServer(t, idalloc.Range{Min: 10000, Max: 10999})
	admin := importAdmin(t, ts)

	bad := row(0, "Über!") // invalid uid charset
	rows := []importRowReq{bad, row(1, "anna"), row(2, "anna")} // row 2: duplicate in file
	code, res := postImport(t, admin, rows)
	if code != 200 {
		t.Fatalf("import: %d", code)
	}
	wantStatuses(t, res, "invalid", "created", "invalid")
	if res.Results[0].Error == "" || res.Results[2].Error == "" {
		t.Fatal("invalid rows must carry an error message")
	}

	// Empty sn and unknown primary group are also per-row invalid.
	noSN := row(0, "dora")
	noSN.SN = ""
	badGroup := row(1, "emil")
	badGroup.POSIX = &posixReq{PrimaryGroup: "nope"}
	_, res = postImport(t, admin, []importRowReq{noSN, badGroup})
	wantStatuses(t, res, "invalid", "invalid")
}

func TestImportRowCaps(t *testing.T) {
	ts := importTestServer(t, idalloc.Range{Min: 10000, Max: 10999})
	admin := importAdmin(t, ts)

	if code, _ := postImport(t, admin, nil); code != http.StatusBadRequest {
		t.Fatalf("0 rows: want 400, got %d", code)
	}
	many := make([]importRowReq, maxImportRows+1)
	for i := range many {
		many[i] = row(i, fmt.Sprintf("user%03d", i))
	}
	if code, _ := postImport(t, admin, many); code != http.StatusBadRequest {
		t.Fatalf("%d rows: want 400", len(many))
	}
}

func TestImportChunksAllocateMonotonically(t *testing.T) {
	ts := importTestServer(t, idalloc.Range{Min: 10000, Max: 10999})
	admin := importAdmin(t, ts)

	mkChunk := func(base int) []importRowReq {
		rows := make([]importRowReq, 3)
		for i := range rows {
			rows[i] = row(base+i, fmt.Sprintf("user%03d", base+i))
			rows[i].POSIX = &posixReq{}
		}
		return rows
	}
	_, res1 := postImport(t, admin, mkChunk(0))
	_, res2 := postImport(t, admin, mkChunk(3))
	wantStatuses(t, res1, "created", "created", "created")
	wantStatuses(t, res2, "created", "created", "created")
	last := 0
	for _, r := range append(res1.Results, res2.Results...) {
		n := r.User.POSIX.UIDNumber
		if n <= last {
			t.Fatalf("uidNumbers not monotonic: %d after %d", n, last)
		}
		last = n
	}
}

func TestImportRangeExhaustedAbortsChunk(t *testing.T) {
	ts := importTestServer(t, idalloc.Range{Min: 10000, Max: 10001}) // 2 free uids
	admin := importAdmin(t, ts)

	rows := make([]importRowReq, 4)
	for i := range rows {
		rows[i] = row(i, fmt.Sprintf("user%d", i))
		rows[i].POSIX = &posixReq{}
	}
	code, res := postImport(t, admin, rows)
	if code != 200 {
		t.Fatalf("import: %d", code)
	}
	wantStatuses(t, res, "created", "created", "error", "skipped")
}

func TestImportRequiresAdmin(t *testing.T) {
	ts := importTestServer(t, idalloc.Range{Min: 10000, Max: 10999})
	admin := importAdmin(t, ts)
	admin.do(http.MethodPost, "/api/users", createUserReq{UID: "bob", CN: "B", SN: "B", Password: "longpassword12"})

	bob := newClient(t, ts.URL)
	if code := bob.login("bob", "longpassword12"); code != 200 {
		t.Fatalf("bob login: %d", code)
	}
	if code, _ := postImport(t, bob, []importRowReq{row(0, "eve")}); code != http.StatusForbidden {
		t.Fatalf("non-admin import: want 403, got %d", code)
	}
}
