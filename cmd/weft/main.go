// Command weft is a single-binary web UI to administer users and groups in an
// external LDAP server (target: OpenBSD ldapd). It serves the embedded SPA and
// a JSON API; authentication is passthrough bind against the directory.
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"weft/internal/applog"
	"weft/internal/config"
	"weft/internal/directory"
	"weft/internal/directory/fake"
	"weft/internal/directory/ldapd"
	"weft/internal/privsep"
	"weft/internal/sandbox"
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

	assets, err := web.Assets()
	if err != nil {
		return fmt.Errorf("loading embedded frontend: %w", err)
	}

	switch {
	case privsep.IsWorker():
		return runWorker(cfg, assets)
	case !*dev && cfg.Privsep && privsep.Supported && os.Geteuid() == 0:
		// privsep is on by default but only engages as root: only then can the
		// worker chroot and drop privileges. Non-root / -dev run single-process.
		return runMonitor(cfg)
	default:
		return runSingle(cfg, *dev, *devRootpw, assets)
	}
}

// runSingle is the classic single-process mode (also used for -dev and when
// privsep is disabled). It applies the in-process sandbox (chroot/drop/pledge).
func runSingle(cfg config.Config, dev bool, devRootpw string, assets fs.FS) error {
	_, closeLog := setupLogging(cfg, "single")
	defer closeLog()

	var dir directory.Directory
	if dev {
		cfg = devDefaults(cfg)
		dir = fake.New(devRootpw, cfg.UIDRange(), cfg.GIDRange())
		log.Printf("DEV MODE: in-memory fake directory, admin uid=%q password=%q", cfg.AdminUID, devRootpw)
	} else {
		if err := cfg.Validate(); err != nil {
			return err
		}
		d, err := ldapd.New(cfg, nil) // default network dialer; reads CA/system roots now
		if err != nil {
			return err
		}
		dir = d
		logStartup(cfg)
		if cfg.Privsep && privsep.Supported && os.Geteuid() != 0 {
			log.Print("privsep is enabled but weft is not running as root; running single-process (start as root to enable privilege separation)")
		}
	}

	srv := server.New(cfg, dir, assets)
	defer srv.Close()

	ln, err := listen(cfg)
	if err != nil {
		return err
	}

	if !dev {
		warmRuntime()
		// In-process confinement. A chroot can't reach an ldapi socket, so skip
		// it in that case (privilege drop + pledge/unveil still apply). Use
		// privsep to keep the chroot with ldapi or a hostname.
		sbChroot := cfg.Chroot
		if cfg.Sandbox && os.Geteuid() == 0 && sbChroot != "" {
			switch {
			case cfg.IsLDAPI():
				log.Printf("sandbox: ldapi socket %q is outside the chroot; skipping chroot (privilege drop + pledge/unveil still apply). Enable privsep to keep the chroot.",
					cfg.LDAPISocketPath())
				sbChroot = ""
			case cfg.LDAPHostIsName():
				log.Printf("WARNING: chroot %q is active and ldap_url uses a hostname -- DNS config is not inside the chroot; use an IP address, enable privsep, or set chroot=\"\"",
					sbChroot)
			}
		}
		if err := sandbox.Apply(sandbox.Config{
			Enabled:    cfg.Sandbox,
			Chroot:     sbChroot,
			User:       cfg.User,
			Group:      cfg.Group,
			LDAPI:      cfg.IsLDAPI(),
			SocketPath: cfg.LDAPISocketPath(),
			CACertFile: cfg.CACertFile,
			NeedsDNS:   cfg.LDAPHostIsName(),
		}); err != nil {
			return fmt.Errorf("sandbox: %w", err)
		}
	}

	return serveAndWait(newHTTPServer(srv), ln, cfg.ListenAddr, nil)
}

// runMonitor is the privileged side of privilege separation: it binds the
// listener, re-execs the worker, and serves LDAP dial requests.
func runMonitor(cfg config.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	logLine, closeLog := setupLogging(cfg, "monitor")
	defer closeLog()
	logStartup(cfg)

	network, address, err := cfg.DialTarget()
	if err != nil {
		return err
	}
	dial := func() (net.Conn, error) { return net.DialTimeout(network, address, 10*time.Second) }

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	tcpLn, ok := ln.(*net.TCPListener)
	if !ok {
		return fmt.Errorf("privsep: expected a TCP listener for %s", cfg.ListenAddr)
	}
	log.Printf("privsep: monitor (pid %d) opening LDAP connections on the worker's behalf; worker chroots to %q, drops to %q",
		os.Getpid(), cfg.Chroot, cfg.User)

	return privsep.RunMonitor(tcpLn, dial, func() error {
		return sandbox.ConfineMonitor(sandbox.Config{
			Enabled:    cfg.Sandbox,
			LDAPI:      cfg.IsLDAPI(),
			SocketPath: cfg.LDAPISocketPath(),
			NeedsDNS:   cfg.LDAPHostIsName(),
			Syslog:     cfg.Log == "syslog",
		})
	}, logLine)
}

// runWorker is the unprivileged side: it serves HTTP and the JSON API, asking
// the monitor for LDAP connections. It chroots and drops privileges; in privsep
// mode the worker never touches the filesystem for LDAP, so a /var/empty chroot
// is safe even with a hostname or ldapi endpoint.
func runWorker(cfg config.Config, assets fs.FS) error {
	setupLogging(cfg, "worker")
	w, err := privsep.StartWorker()
	if err != nil {
		return err
	}
	dir, err := ldapd.New(cfg, w.DialLDAP) // warms the TLS trust store in this process
	if err != nil {
		return err
	}

	srv := server.New(cfg, dir, assets)
	defer srv.Close()

	ln, err := wrapTLS(cfg, w.Listener)
	if err != nil {
		return err
	}

	warmRuntime()
	if err := sandbox.ConfineWorker(sandbox.Config{
		Enabled: cfg.Sandbox,
		Chroot:  cfg.Chroot,
		User:    cfg.User,
		Group:   cfg.Group,
	}); err != nil {
		return fmt.Errorf("sandbox worker: %w", err)
	}
	log.Printf("privsep: worker (pid %d) serving on %s", os.Getpid(), cfg.ListenAddr)

	return serveAndWait(newHTTPServer(srv), ln, cfg.ListenAddr, w.Done())
}

// --- helpers ---

// listen opens the TCP listener and wraps it in TLS if a keypair is configured.
func listen(cfg config.Config) (net.Listener, error) {
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	return wrapTLS(cfg, ln)
}

// wrapTLS wraps ln in a TLS listener when tls_cert_file/tls_key_file are set,
// loading the keypair now (before any sandbox locks the filesystem).
func wrapTLS(cfg config.Config, ln net.Listener) (net.Listener, error) {
	if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return ln, nil
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load tls keypair: %w", err)
	}
	return tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}), nil
}

// warmRuntime triggers every lazy, filesystem-backed initialisation before the
// sandbox locks the filesystem, so the worker can run under a tight pledge that
// promises no rpath:
//   - the net/http MIME table reads /etc/mime.types on the first .js/.css lookup
//   - the time package reads /etc/localtime on the first local-time format (logs)
//   - the CSPRNG (harmless on OpenBSD, which uses getentropy, but cheap insurance)
//
// If a syscall still trips pledge after this, the fix is to warm whatever opened
// a file here -- not to widen the promise.
func warmRuntime() {
	_ = mime.TypeByExtension(".html")
	_ = time.Now().Local().Format(time.RFC3339)
	var b [1]byte
	_, _ = rand.Read(b[:])
}

// setupLogging configures the log package for this process's role. In syslog
// mode the monitor and the single-process own the syslog connection; the worker
// keeps logging to stderr (it is chrooted and cannot reach /dev/log), and the
// monitor forwards those lines. Returns a line sink for the monitor to forward
// the worker's output (nil unless this is the monitor in syslog mode).
func setupLogging(cfg config.Config, role string) (logLine func(string), closeFn func()) {
	noop := func() {}
	if cfg.Log != "syslog" {
		return nil, noop
	}
	if role == "worker" {
		log.SetFlags(0) // syslog adds its own timestamp; the monitor forwards us
		return nil, noop
	}
	sink, err := applog.NewSyslog(cfg.SyslogTag)
	if err != nil {
		log.Printf("syslog unavailable (%v); logging to stderr", err)
		return nil, noop
	}
	log.SetOutput(sink)
	log.SetFlags(0)
	return sink.WriteLine, func() { sink.Close() }
}

func newHTTPServer(srv *server.Server) *http.Server {
	return &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// serveAndWait serves until an OS signal, a shutdown request on stopC (nil in
// single-process mode; the privsep monitor's shutdown channel otherwise), or a
// server error.
func serveAndWait(httpSrv *http.Server, ln net.Listener, addr string, stopC <-chan struct{}) error {
	errCh := make(chan error, 1)
	go func() {
		log.Printf("weft %s listening on %s", version, addr)
		errCh <- httpSrv.Serve(ln)
	}()

	shutdown := func() error {
		log.Print("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(ctx)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-stop:
		return shutdown()
	case <-stopC:
		return shutdown()
	}
}

// logStartup prints the LDAP target, the resolved admin bind DN, and TLS
// warnings (suppressed for the local ldapi socket).
func logStartup(cfg config.Config) {
	if cfg.IsLDAPI() {
		log.Printf("LDAP server: %s (local unix socket, secured by file permissions; tls_mode/allow_plain_bind ignored), base_dn=%q",
			cfg.LDAPURL, cfg.BaseDN)
	} else {
		log.Printf("LDAP server: %s (tls_mode=%s, base_dn=%q)", cfg.LDAPURL, cfg.TLSMode, cfg.BaseDN)
	}
	log.Printf("admin login: type uid %q; it binds as %q (must equal ldapd rootdn)",
		cfg.AdminUID, cfg.AdminBindDN())
	if !cfg.IsLDAPI() {
		if cfg.InsecureSkipVerify {
			log.Print("WARNING: insecure_skip_verify is enabled -- TLS certificates are not validated")
		}
		if cfg.TLSMode == config.TLSPlain {
			log.Print("WARNING: tls_mode=plain -- credentials are sent without TLS (dev only)")
		}
	}
}

// devDefaults fills the minimum config needed for -dev mode.
func devDefaults(cfg config.Config) config.Config {
	if cfg.BaseDN == "" {
		cfg.BaseDN = "dc=example,dc=org"
	}
	cfg.CookieSecure = false // local http
	return cfg
}
