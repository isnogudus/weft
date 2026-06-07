package fake

import (
	"context"
	"errors"
	"testing"

	"weft/internal/directory"
	"weft/internal/idalloc"
)

func newFake() *Fake {
	return New("rootpw", idalloc.Range{Min: 10000, Max: 10002}, idalloc.Range{Min: 10000, Max: 59999})
}

func TestBind(t *testing.T) {
	f := newFake()
	f.AddUser(directory.User{UID: "alice", CN: "Alice", SN: "A"}, "secret")
	ctx := context.Background()

	if _, err := f.BindUser(ctx, "alice", "wrong"); !errors.Is(err, directory.ErrInvalidCredentials) {
		t.Fatalf("bad password: got %v", err)
	}
	if _, err := f.BindUser(ctx, "alice", "secret"); err != nil {
		t.Fatalf("good password: %v", err)
	}
	if _, err := f.BindAdmin(ctx, "rootpw"); err != nil {
		t.Fatalf("admin bind: %v", err)
	}
	if _, err := f.BindAdmin(ctx, "nope"); !errors.Is(err, directory.ErrInvalidCredentials) {
		t.Fatalf("admin bad: got %v", err)
	}
}

func TestUserMayWriteOnlyOwnEntry(t *testing.T) {
	f := newFake()
	f.AddUser(directory.User{UID: "alice", CN: "Alice", SN: "A"}, "pw")
	f.AddUser(directory.User{UID: "bob", CN: "Bob", SN: "B"}, "pw")
	ctx := context.Background()
	alice, _ := f.BindUser(ctx, "alice", "pw")

	// own entry: allowed
	if err := alice.UpdateUser(ctx, directory.User{UID: "alice", CN: "Alice Cooper", SN: "A"}); err != nil {
		t.Fatalf("update own: %v", err)
	}
	if err := alice.SetPassword(ctx, "alice", "{CRYPT}$2b$10$x"); err != nil {
		t.Fatalf("set own password: %v", err)
	}
	// someone else's entry: denied
	if err := alice.UpdateUser(ctx, directory.User{UID: "bob", CN: "x", SN: "y"}); !errors.Is(err, directory.ErrPermission) {
		t.Fatalf("update other: got %v, want ErrPermission", err)
	}
	if err := alice.SetPassword(ctx, "bob", "{CRYPT}x"); !errors.Is(err, directory.ErrPermission) {
		t.Fatalf("set other password: got %v", err)
	}
	// admin-only operations: denied for a user
	if err := alice.CreateUser(ctx, directory.User{UID: "carol"}, "{CRYPT}x"); !errors.Is(err, directory.ErrPermission) {
		t.Fatalf("create user as user: got %v", err)
	}
	if err := alice.DeleteUser(ctx, "bob"); !errors.Is(err, directory.ErrPermission) {
		t.Fatalf("delete user as user: got %v", err)
	}
}

func TestEffectiveGroups(t *testing.T) {
	f := newFake()
	f.AddGroup(directory.Group{CN: "users", GIDNumber: 10000})
	f.AddGroup(directory.Group{CN: "devs", GIDNumber: 10001, MemberUID: []string{"alice"}})
	f.AddGroup(directory.Group{CN: "ops", GIDNumber: 10002})
	f.AddUser(directory.User{
		UID: "alice", CN: "Alice", SN: "A",
		POSIX: &directory.POSIXProfile{UIDNumber: 10000, GIDNumber: 10000, HomeDirectory: "/home/alice"},
	}, "pw")
	ctx := context.Background()
	admin, _ := f.BindAdmin(ctx, "rootpw")

	groups, err := admin.EffectiveGroups(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, g := range groups {
		got[g.CN] = true
	}
	// primary "users" (via gidNumber) + supplementary "devs" (via memberUid)
	if !got["users"] || !got["devs"] {
		t.Fatalf("expected users+devs, got %v", got)
	}
	if got["ops"] {
		t.Fatalf("ops should not be effective")
	}
}

func TestRenameUIDFixesMembership(t *testing.T) {
	f := newFake()
	f.AddGroup(directory.Group{CN: "devs", GIDNumber: 10001, MemberUID: []string{"alice", "bob"}})
	f.AddUser(directory.User{UID: "alice", CN: "Alice", SN: "A"}, "pw")
	ctx := context.Background()
	admin, _ := f.BindAdmin(ctx, "rootpw")

	if err := admin.RenameUID(ctx, "alice", "alicia"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.GetUser(ctx, "alice"); !errors.Is(err, directory.ErrNotFound) {
		t.Fatalf("old uid should be gone: %v", err)
	}
	if _, err := admin.GetUser(ctx, "alicia"); err != nil {
		t.Fatalf("new uid should exist: %v", err)
	}
	g, _ := admin.GetGroup(ctx, "devs")
	if g.HasMember("alice") || !g.HasMember("alicia") {
		t.Fatalf("memberUid not migrated: %v", g.MemberUID)
	}
}

func TestAllocateUIDNumber(t *testing.T) {
	f := newFake()
	f.AddUser(directory.User{UID: "a", POSIX: &directory.POSIXProfile{UIDNumber: 10000, GIDNumber: 10000}}, "pw")
	ctx := context.Background()
	admin, _ := f.BindAdmin(ctx, "rootpw")

	n, err := admin.AllocateUIDNumber(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10001 {
		t.Fatalf("next uid = %d, want 10001", n)
	}
	// exhaust the tiny range (10000..10002)
	f.AddUser(directory.User{UID: "b", POSIX: &directory.POSIXProfile{UIDNumber: 10001}}, "pw")
	f.AddUser(directory.User{UID: "c", POSIX: &directory.POSIXProfile{UIDNumber: 10002}}, "pw")
	if _, err := admin.AllocateUIDNumber(ctx); !errors.Is(err, directory.ErrRangeExhausted) {
		t.Fatalf("expected exhausted, got %v", err)
	}
}
