package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"weft/internal/directory"
)

// ctxKey is the context key type for the current session.
type ctxKey int

const sessionKey ctxKey = 0

const (
	sessionCookie = "weft_session"
	csrfCookie    = "weft_csrf" // readable by JS so the SPA can echo it back
	csrfHeader    = "X-CSRF-Token"
	maxBodyBytes  = 1 << 20 // 1 MiB
)

// errorResponse is the JSON body for any non-2xx API response.
type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// writeDirError maps directory sentinel errors to HTTP status codes.
func writeDirError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, directory.ErrNotFound):
		writeError(w, http.StatusNotFound, "nicht gefunden")
	case errors.Is(err, directory.ErrAlreadyExists):
		writeError(w, http.StatusConflict, "existiert bereits")
	case errors.Is(err, directory.ErrPermission):
		writeError(w, http.StatusForbidden, "keine Berechtigung")
	case errors.Is(err, directory.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "ungültige Anmeldedaten")
	case errors.Is(err, directory.ErrRangeExhausted):
		writeError(w, http.StatusConflict, "uid/gid-Bereich erschöpft")
	default:
		writeError(w, http.StatusBadGateway, "Verzeichnisfehler")
	}
}

// readJSON decodes a JSON request body with the default size limit and strict
// fields.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return readJSONMax(w, r, dst, maxBodyBytes)
}

// readJSONMax is readJSON with an explicit size limit (the bulk import accepts
// larger bodies than the 1 MiB default).
func readJSONMax(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

// clientIP extracts a best-effort client IP for rate limiting. When weft runs
// behind relayd/httpd, configure the proxy to set X-Forwarded-For; otherwise
// the proxy address is used (a coarse but safe default).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := indexComma(xff); i >= 0 {
			return trimSpace(xff[:i])
		}
		return trimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// sessionFromCtx returns the session attached by the auth middleware.
func sessionFromCtx(ctx context.Context) *session {
	s, _ := ctx.Value(sessionKey).(*session)
	return s
}

// contextWithSession attaches a session to a context.
func contextWithSession(ctx context.Context, s *session) context.Context {
	return context.WithValue(ctx, sessionKey, s)
}
