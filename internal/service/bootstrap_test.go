package service

import (
	"context"
	"testing"

	"weft/internal/config"
	"weft/internal/directory/fake"
	"weft/internal/idalloc"
)

func TestBootstrap(t *testing.T) {
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10005}, idalloc.Range{Min: 20000, Max: 20005})
	ctx := context.Background()

	if ok, _ := f.Provisioned(ctx); ok {
		t.Fatal("should start unprovisioned")
	}
	admin, _ := f.BindAdmin(ctx, "rootpw")
	s := New(cfg)
	if err := s.Bootstrap(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if ok, _ := f.Provisioned(ctx); !ok {
		t.Fatal("should be provisioned after bootstrap")
	}
	g, err := admin.GetGroup(ctx, "users")
	if err != nil {
		t.Fatalf("default group missing: %v", err)
	}
	if g.GIDNumber != 20000 {
		t.Fatalf("users gid = %d, want 20000", g.GIDNumber)
	}
	// Idempotent re-run.
	if err := s.Bootstrap(ctx, admin); err != nil {
		t.Fatalf("re-run should be safe: %v", err)
	}
}

func TestBootstrapRequiresAdmin(t *testing.T) {
	cfg := config.Default()
	cfg.BaseDN = "dc=example,dc=org"
	f := fake.New("rootpw", idalloc.Range{Min: 10000, Max: 10005}, idalloc.Range{Min: 20000, Max: 20005})
	f.AddUser(directoryUser("alice"), "pw")
	ctx := context.Background()
	user, _ := f.BindUser(ctx, "alice", "pw")
	if err := New(cfg).Bootstrap(ctx, user); err == nil {
		t.Fatal("non-admin bootstrap should fail")
	}
}
