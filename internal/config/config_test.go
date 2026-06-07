package config

import "testing"

func validBase() Config {
	c := Default()
	c.LDAPURL = "ldaps://ldap.example.org:636"
	c.BaseDN = "dc=example,dc=org"
	return c
}

func TestValidate(t *testing.T) {
	if err := validBase().Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	c := validBase()
	c.LDAPURL = ""
	if err := c.Validate(); err == nil {
		t.Fatal("missing ldap_url should fail")
	}

	c = validBase()
	c.TLSMode = TLSPlain
	if err := c.Validate(); err == nil {
		t.Fatal("plain without allow_plain_bind should fail")
	}
	c.AllowPlainBind = true
	c.LDAPURL = "ldap://localhost:389"
	if err := c.Validate(); err != nil {
		t.Fatalf("plain+allow should pass: %v", err)
	}

	c = validBase()
	c.AdminUID = ""
	if err := c.Validate(); err == nil {
		t.Fatal("missing admin_uid should fail")
	}
}

func TestDNHelpers(t *testing.T) {
	c := validBase()
	if got := c.UserDN("alice"); got != "uid=alice,ou=people,dc=example,dc=org" {
		t.Fatalf("UserDN = %q", got)
	}
	if got := c.GroupDN("devs"); got != "cn=devs,ou=groups,dc=example,dc=org" {
		t.Fatalf("GroupDN = %q", got)
	}
	if got := c.HomeDir("alice"); got != "/home/alice" {
		t.Fatalf("HomeDir = %q", got)
	}
	if !c.IsAdminUID("admin") || !c.IsAdminUID("ADMIN") || c.IsAdminUID("alice") {
		t.Fatal("IsAdminUID mismatch")
	}
	if got := c.AdminBindDN(); got != "uid=admin,ou=people,dc=example,dc=org" {
		t.Fatalf("AdminBindDN (derived) = %q", got)
	}
	c.AdminDN = "cn=admin,dc=example,dc=org"
	if got := c.AdminBindDN(); got != "cn=admin,dc=example,dc=org" {
		t.Fatalf("AdminBindDN (explicit) = %q", got)
	}
}
