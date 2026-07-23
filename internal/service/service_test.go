package service

import (
	"context"
	"strings"
	"testing"

	"weft/internal/config"
	"weft/internal/directory"
	"weft/internal/directory/fake"
	"weft/internal/idalloc"
)

func setup(t *testing.T) (*Service, directory.Conn) {
	t.Helper()
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10005}, idalloc.Range{Min: 20000, Max: 20005})
	f.AddGroup(directory.Group{CN: "users", GIDNumber: 20000})
	admin, err := f.BindAdmin(context.Background(), "rootpw")
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg), admin
}

func TestCreateUserPlain(t *testing.T) {
	s, c := setup(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, c, NewUser{UID: "alice", CN: "Alice", SN: "A", Password: "hunter2hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if u.POSIX != nil {
		t.Fatal("expected no POSIX profile")
	}
	got, err := c.GetUser(ctx, "alice")
	if err != nil || got.UID != "alice" {
		t.Fatalf("user not stored: %v", err)
	}
}

func TestCreateUserPOSIXDefaults(t *testing.T) {
	s, c := setup(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, c, NewUser{
		UID: "bob", CN: "Bob", SN: "B", Password: "longenoughpw!",
		POSIX: &POSIXInput{}, // all defaults
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.POSIX.UIDNumber != 10000 {
		t.Fatalf("uidNumber = %d, want 10000", u.POSIX.UIDNumber)
	}
	if u.POSIX.GIDNumber != 20000 { // primary group "users"
		t.Fatalf("gidNumber = %d, want 20000", u.POSIX.GIDNumber)
	}
	if u.POSIX.HomeDirectory != "/home/bob" {
		t.Fatalf("home = %q", u.POSIX.HomeDirectory)
	}
	if u.POSIX.LoginShell != "/bin/ksh" {
		t.Fatalf("shell = %q", u.POSIX.LoginShell)
	}
}

func TestCreateUserCNIdentity(t *testing.T) {
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	cfg.UserIDAttr = "cn"
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10005}, idalloc.Range{Min: 20000, Max: 20005})
	f.AddGroup(directory.Group{CN: "users", GIDNumber: 20000})
	admin, err := f.BindAdmin(context.Background(), "rootpw")
	if err != nil {
		t.Fatal(err)
	}
	s := New(cfg)
	ctx := context.Background()

	// cn is forced to equal the identifier regardless of what's submitted --
	// LDAP requires the RDN's value to be one of the entry's own cn values.
	u, err := s.CreateUser(ctx, admin, NewUser{UID: "jdoe", CN: "Should be ignored", SN: "Doe", Password: "hunter2hunter2"})
	if err != nil {
		t.Fatal(err)
	}
	if u.CN != "jdoe" {
		t.Fatalf("CN = %q, want it collapsed to the identifier %q", u.CN, u.UID)
	}
	got, err := admin.GetUser(ctx, "jdoe")
	if err != nil || got.CN != "jdoe" {
		t.Fatalf("stored user: %+v, err %v", got, err)
	}

	// Even an empty submitted cn (matching the SPA hiding that field in this
	// mode) must not fail validText("cn", ...) -- it's populated from uid
	// before validation runs.
	if _, err := s.CreateUser(ctx, admin, NewUser{UID: "asmith", SN: "Smith", Password: "hunter2hunter2"}); err != nil {
		t.Fatalf("create with empty cn should succeed in cn-identity mode: %v", err)
	}

	// UpdateUser collapses the same way.
	u, err = s.UpdateUser(ctx, admin, NewUser{UID: "jdoe", CN: "Still ignored", SN: "Doe2"})
	if err != nil {
		t.Fatal(err)
	}
	if u.CN != "jdoe" {
		t.Fatalf("UpdateUser: CN = %q, want collapsed to jdoe", u.CN)
	}
}

func setupWithAttrs(t *testing.T) (*Service, directory.Conn) {
	t.Helper()
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	cfg.UserAttrs = []config.UserAttr{
		{Attr: "telephoneNumber", LabelDE: "Telefon"},
		{Attr: "st", LabelDE: "Bundesland", Required: true},
	}
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10005}, idalloc.Range{Min: 20000, Max: 20005})
	f.AddGroup(directory.Group{CN: "users", GIDNumber: 20000})
	admin, err := f.BindAdmin(context.Background(), "rootpw")
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg), admin
}

func TestCreateUserExtraAttrs(t *testing.T) {
	s, c := setupWithAttrs(t)
	ctx := context.Background()

	// Required extra attribute missing -> reject.
	_, err := s.CreateUser(ctx, c, NewUser{UID: "dora", CN: "Dora", SN: "D", Password: "longenoughpw!"})
	if err == nil || !strings.Contains(err.Error(), "st") {
		t.Fatalf("expected required-attr error, got %v", err)
	}

	// Unknown key -> reject.
	_, err = s.CreateUser(ctx, c, NewUser{
		UID: "dora", CN: "Dora", SN: "D", Password: "longenoughpw!",
		Extra: map[string]string{"st": "NI", "surprise": "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "surprise") {
		t.Fatalf("expected unknown-attr error, got %v", err)
	}

	// Valid create round-trips; empty optional value is dropped.
	u, err := s.CreateUser(ctx, c, NewUser{
		UID: "dora", CN: "Dora", SN: "D", Password: "longenoughpw!",
		Extra: map[string]string{"st": " NI ", "telephoneNumber": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if u.Extra["st"] != "NI" {
		t.Fatalf("extra st = %q, want NI (trimmed)", u.Extra["st"])
	}
	if _, ok := u.Extra["telephoneNumber"]; ok {
		t.Fatal("empty optional extra should be dropped")
	}
	got, err := c.GetUser(ctx, "dora")
	if err != nil || got.Extra["st"] != "NI" {
		t.Fatalf("extra not stored: %v, %v", got, err)
	}

	// Update replaces the set.
	if _, err = s.UpdateUser(ctx, c, NewUser{
		UID: "dora", CN: "Dora", SN: "D",
		Extra: map[string]string{"st": "HB", "telephoneNumber": "+49 421 1"},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = c.GetUser(ctx, "dora")
	if got.Extra["st"] != "HB" || got.Extra["telephoneNumber"] != "+49 421 1" {
		t.Fatalf("extra after update = %v", got.Extra)
	}
}

func TestCreateUserRejectsBadUID(t *testing.T) {
	s, c := setup(t)
	_, err := s.CreateUser(context.Background(), c, NewUser{
		UID: "evil,dc=x", CN: "E", SN: "E", Password: "longenoughpw!",
	})
	if err == nil || !strings.Contains(err.Error(), "uid") {
		t.Fatalf("expected uid validation error, got %v", err)
	}
}

func TestCreateUserRejectsLongPassword(t *testing.T) {
	s, c := setup(t)
	_, err := s.CreateUser(context.Background(), c, NewUser{
		UID: "carol", CN: "C", SN: "C", Password: strings.Repeat("x", 73),
	})
	if err == nil {
		t.Fatal("expected password length rejection")
	}
}

func TestCreateGroupAllocatesGID(t *testing.T) {
	s, c := setup(t)
	ctx := context.Background()
	g, err := s.CreateGroup(ctx, c, "devs", 0)
	if err != nil {
		t.Fatal(err)
	}
	if g.GIDNumber != 20001 { // 20000 taken by "users"
		t.Fatalf("gid = %d, want 20001", g.GIDNumber)
	}
}

func TestToggleMailProfile(t *testing.T) {
	s, c := setup(t)
	ctx := context.Background()
	if _, err := s.CreateUser(ctx, c, NewUser{UID: "d", CN: "D", SN: "D", Password: "longenoughpw!"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.UpdateUser(ctx, c, NewUser{
		UID: "d", CN: "D", SN: "D",
		Mail: &directory.MailProfile{Mail: "d@example.org", Aliases: []string{"", "dd@example.org"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := c.GetUser(ctx, "d")
	if got.Mail == nil || got.Mail.Mail != "d@example.org" || len(got.Mail.Aliases) != 1 {
		t.Fatalf("mail profile not set/normalized: %+v", got.Mail)
	}
}
