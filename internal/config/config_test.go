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

func TestValidateUserAttrs(t *testing.T) {
	c := validBase()
	c.UserAttrs = []UserAttr{
		{Attr: "telephoneNumber", LabelDE: "Telefon", LabelEN: "Phone"},
		{Attr: "departmentNumber", Required: true},
	}
	c.UserExtraClasses = []string{"policePerson"}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid user_attr config rejected: %v", err)
	}

	for _, bad := range []UserAttr{
		{Attr: "uid"},              // reserved
		{Attr: "mail"},             // collides with mail_attr
		{Attr: "userPassword"},     // reserved (case-insensitive)
		{Attr: "invalid attr"},     // bad charset
		{Attr: ""},                 // empty
		{Attr: "1telephoneNumber"}, // must start with a letter
	} {
		c := validBase()
		c.UserAttrs = []UserAttr{bad}
		if err := c.Validate(); err == nil {
			t.Fatalf("user_attr %q should be rejected", bad.Attr)
		}
	}

	c = validBase()
	c.UserAttrs = []UserAttr{{Attr: "st"}, {Attr: "ST"}}
	if err := c.Validate(); err == nil {
		t.Fatal("duplicate user_attr (case-insensitive) should be rejected")
	}

	c = validBase()
	c.UserExtraClasses = []string{"bad class"}
	if err := c.Validate(); err == nil {
		t.Fatal("invalid user_extra_classes name should be rejected")
	}
}

func TestUserAttrLabel(t *testing.T) {
	a := UserAttr{Attr: "st", LabelDE: "Bundesland", LabelEN: "State"}
	if a.Label("de") != "Bundesland" || a.Label("en") != "State" {
		t.Fatal("Label lang selection broken")
	}
	if (UserAttr{Attr: "st", LabelDE: "Bundesland"}).Label("en") != "Bundesland" {
		t.Fatal("Label fallback to other language broken")
	}
	if (UserAttr{Attr: "st"}).Label("de") != "st" {
		t.Fatal("Label fallback to attr name broken")
	}
}

func TestLDAPIOverridesTLS(t *testing.T) {
	c := validBase()
	c.LDAPURL = "ldapi:///var/run/ldapi"
	// tls_mode stays at its default (ldaps) and allow_plain_bind is false;
	// for ldapi both must be ignored, so Validate must still pass.
	if !c.IsLDAPI() {
		t.Fatal("IsLDAPI should be true for ldapi:// url")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("ldapi config should validate despite tls_mode/allow_plain_bind: %v", err)
	}

	// A non-ldapi url with the same (mismatching) tls_mode must still fail.
	c2 := validBase()
	c2.LDAPURL = "ldap://localhost:389"
	c2.TLSMode = TLSLDAPS
	if err := c2.Validate(); err == nil {
		t.Fatal("tls_mode=ldaps with ldap:// scheme should fail")
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
