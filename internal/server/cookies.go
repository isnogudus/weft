package server

import "net/http"

// setSessionCookie writes the opaque, HttpOnly session cookie plus a readable
// CSRF cookie the SPA echoes back in the X-CSRF-Token header.
func (s *Server) setSessionCookie(w http.ResponseWriter, sess *session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sess.id,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    sess.csrf,
		Path:     "/",
		HttpOnly: false, // must be readable by the SPA
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: name == sessionCookie,
			Secure:   s.cfg.CookieSecure,
			SameSite: http.SameSiteStrictMode,
		})
	}
}
