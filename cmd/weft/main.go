// Command weft is a single-binary web UI to administer users and groups in an
// external LDAP server (target: OpenBSD ldapd). It serves the embedded SPA and
// a JSON API; authentication is passthrough bind against the directory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"weft/internal/config"
	"weft/internal/directory"
	"weft/internal/directory/fake"
	"weft/internal/directory/ldapd"
	"weft/internal/server"
	"weft/web"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "weft:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "", "path to TOML config file")
		listenAddr  = flag.String("listen", "", "override listen address")
		insecure    = flag.Bool("insecure", false, "do not verify the LDAP server's TLS certificate")
		dev         = flag.Bool("dev", false, "run against an in-memory fake directory (no LDAP)")
		devRootpw   = flag.String("dev-rootpw", "rootpw", "admin password in -dev mode")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("weft", version)
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *listenAddr != "" {
		cfg.ListenAddr = *listenAddr
	}
	if *insecure {
		cfg.InsecureSkipVerify = true
	}

	var dir directory.Directory
	if *dev {
		cfg = devDefaults(cfg)
		dir = fake.New(*devRootpw, cfg.UIDRange(), cfg.GIDRange())
		log.Printf("DEV MODE: in-memory fake directory, admin uid=%q password=%q", cfg.AdminUID, *devRootpw)
	} else {
		if err := cfg.Validate(); err != nil {
			return err
		}
		dir = ldapd.New(cfg)
		if cfg.IsLDAPI() {
			log.Printf("LDAP server: %s (local unix socket, secured by file permissions; tls_mode/allow_plain_bind ignored), base_dn=%q",
				cfg.LDAPURL, cfg.BaseDN)
		} else {
			log.Printf("LDAP server: %s (tls_mode=%s, base_dn=%q)", cfg.LDAPURL, cfg.TLSMode, cfg.BaseDN)
		}
	}
	// Print the resolved admin bind DN -- it MUST match ldapd's rootdn.
	log.Printf("admin login: type uid %q; it binds as %q (must equal ldapd rootdn)",
		cfg.AdminUID, cfg.AdminBindDN())
	// TLS warnings only apply to network transports, not the local ldapi socket.
	if !cfg.IsLDAPI() {
		if cfg.InsecureSkipVerify {
			log.Print("WARNING: insecure_skip_verify is enabled -- TLS certificates are not validated")
		}
		if cfg.TLSMode == config.TLSPlain {
			log.Print("WARNING: tls_mode=plain -- credentials are sent without TLS (dev only)")
		}
	}

	assets, err := web.Assets()
	if err != nil {
		return fmt.Errorf("loading embedded frontend: %w", err)
	}

	srv := server.New(cfg, dir, assets)
	defer srv.Close()

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("weft %s listening on %s", version, cfg.ListenAddr)
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			errCh <- httpSrv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			errCh <- httpSrv.ListenAndServe()
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-stop:
		log.Print("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(ctx)
	}
	return nil
}

// devDefaults fills the minimum config needed for -dev mode.
func devDefaults(cfg config.Config) config.Config {
	if cfg.BaseDN == "" {
		cfg.BaseDN = "dc=example,dc=org"
	}
	cfg.CookieSecure = false // local http
	return cfg
}
