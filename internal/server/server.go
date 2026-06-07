// Package server exposes weft's JSON API and serves the embedded SPA. Sessions
// are server-side (opaque cookie); bind credentials live only in memory and
// every request re-binds to the directory as the logged-in identity.
package server

import (
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"weft/internal/config"
	"weft/internal/directory"
	"weft/internal/service"
)

// Server wires the configuration, directory, service and HTTP routes together.
type Server struct {
	cfg      config.Config
	dir      directory.Directory
	svc      *service.Service
	sessions *sessionStore
	login    *rateLimiter
	static   fs.FS
	handler  http.Handler
}

// New builds a Server. staticFS is the embedded frontend (its root containing
// index.html); pass nil to serve only the API.
func New(cfg config.Config, dir directory.Directory, staticFS fs.FS) *Server {
	s := &Server{
		cfg:      cfg,
		dir:      dir,
		svc:      service.New(cfg),
		sessions: newSessionStore(cfg.SessionTimeout.D()),
		login:    newRateLimiter(5, time.Minute),
		static:   staticFS,
	}
	s.handler = s.routes()
	return s
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Close releases background resources (the session reaper).
func (s *Server) Close() { s.sessions.close() }

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(requestLog)
	r.Use(securityHeaders)

	r.Route("/api", func(api chi.Router) {
		// Distinct JSON 404/405 so weft's "unknown endpoint" is unmistakable
		// (a static file server or proxy would return HTML instead).
		api.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "unbekannter API-Endpunkt: "+r.URL.Path)
		})
		api.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, "Methode nicht erlaubt: "+r.Method+" "+r.URL.Path)
		})

		// Public endpoints.
		api.Post("/login", s.handleLogin)
		api.Post("/logout", s.handleLogout)
		api.Get("/setup/status", s.handleSetupStatus)
		api.Post("/setup/bootstrap", s.handleBootstrap)

		// Authenticated endpoints.
		api.Group(func(a chi.Router) {
			a.Use(s.requireSession)
			a.Use(s.requireCSRF)
			a.Get("/me", s.handleMe)
			a.Get("/me/profile", s.handleMeProfile)
			a.Get("/me/groups", s.handleMeGroups)
			a.Get("/meta", s.handleMeta)
			a.Post("/me/password", s.handleChangeOwnPassword)

			// Admin-only management.
			a.Group(func(adm chi.Router) {
				adm.Use(s.requireAdmin)
				adm.Get("/users", s.handleListUsers)
				adm.Post("/users", s.handleCreateUser)
				adm.Get("/users/{uid}", s.handleGetUser)
				adm.Put("/users/{uid}", s.handleUpdateUser)
				adm.Delete("/users/{uid}", s.handleDeleteUser)
				adm.Post("/users/{uid}/password", s.handleResetPassword)
				adm.Post("/users/{uid}/rename", s.handleRenameUser)
				adm.Get("/users/{uid}/groups", s.handleUserGroups)

				adm.Get("/groups", s.handleListGroups)
				adm.Post("/groups", s.handleCreateGroup)
				adm.Get("/groups/{cn}", s.handleGetGroup)
				adm.Delete("/groups/{cn}", s.handleDeleteGroup)
				adm.Post("/groups/{cn}/members", s.handleAddMember)
				adm.Delete("/groups/{cn}/members/{uid}", s.handleRemoveMember)
			})
		})
	})

	// SPA fallback for everything else.
	if s.static != nil {
		r.Handle("/*", spaHandler(s.static))
	}
	return r
}

// securityHeaders sets conservative headers on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// --- middleware ---

// requireSession loads and validates the session cookie.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "nicht angemeldet")
			return
		}
		sess, ok := s.sessions.get(ck.Value)
		if !ok {
			s.clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "Sitzung abgelaufen")
			return
		}
		ctx := contextWithSession(r.Context(), sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireCSRF enforces the synchronizer token on state-changing requests.
func (s *Server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		sess := sessionFromCtx(r.Context())
		if sess == nil || r.Header.Get(csrfHeader) != sess.csrf {
			writeError(w, http.StatusForbidden, "ungültiges CSRF-Token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAdmin rejects non-admin sessions.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFromCtx(r.Context())
		if sess == nil || !sess.isAdmin {
			writeError(w, http.StatusForbidden, "nur für Administratoren")
			return
		}
		next.ServeHTTP(w, r)
	})
}
