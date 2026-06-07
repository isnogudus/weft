package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

// Load builds a Config from defaults, then the TOML file at path (if non-empty),
// then WEFT_* environment overrides. Flags are applied by the caller afterwards.
// It does not call Validate; the caller does, after applying flags.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			return cfg, fmt.Errorf("config: reading %s: %w", path, err)
		}
	}
	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// applyEnv overlays WEFT_* environment variables. Only the most operationally
// relevant keys are exposed as env (the rest live in the TOML file).
func applyEnv(c *Config) error {
	envStr("WEFT_LDAP_URL", &c.LDAPURL)
	envStr("WEFT_BASE_DN", &c.BaseDN)
	envStr("WEFT_ADMIN_UID", &c.AdminUID)
	envStr("WEFT_ADMIN_DN", &c.AdminDN)
	envStr("WEFT_CHROOT", &c.Chroot)
	envStr("WEFT_USER", &c.User)
	envStr("WEFT_GROUP", &c.Group)
	if err := envBool("WEFT_SANDBOX", &c.Sandbox); err != nil {
		return err
	}
	envStr("WEFT_LISTEN_ADDR", &c.ListenAddr)
	envStr("WEFT_CA_CERT_FILE", &c.CACertFile)
	envStr("WEFT_TLS_CERT_FILE", &c.TLSCertFile)
	envStr("WEFT_TLS_KEY_FILE", &c.TLSKeyFile)
	if v := os.Getenv("WEFT_TLS_MODE"); v != "" {
		c.TLSMode = TLSMode(v)
	}
	if err := envBool("WEFT_INSECURE_SKIP_VERIFY", &c.InsecureSkipVerify); err != nil {
		return err
	}
	if err := envBool("WEFT_ALLOW_PLAIN_BIND", &c.AllowPlainBind); err != nil {
		return err
	}
	if err := envBool("WEFT_COOKIE_SECURE", &c.CookieSecure); err != nil {
		return err
	}
	if err := envInt("WEFT_BCRYPT_COST", &c.BcryptCost); err != nil {
		return err
	}
	if v := os.Getenv("WEFT_SESSION_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("config: WEFT_SESSION_TIMEOUT: %w", err)
		}
		c.SessionTimeout = Duration(d)
	}
	return nil
}

func envStr(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func envBool(key string, dst *bool) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("config: %s: %w", key, err)
	}
	*dst = b
	return nil
}

func envInt(key string, dst *int) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("config: %s: %w", key, err)
	}
	*dst = n
	return nil
}
