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
