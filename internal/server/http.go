package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"

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

// clientIP returns the client IP the login rate limiter keys on.
//
// X-Forwarded-For is only believed when the TCP peer (RemoteAddr) is one of the
// trusted proxies: its leftmost entries are whatever the client sent, and every
// proxy appends to the right. The header is then walked from right to left,
// skipping further trusted proxies, and the first untrusted address wins (the
// rightmost-untrusted algorithm). A malformed entry ends the walk, as does
// running out of entries; both fall back to the peer address.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	peer, ok := parseHostAddr(r.RemoteAddr)
	if !ok {
		return r.RemoteAddr // e.g. a Unix socket listener
	}
	if !isTrusted(peer, trusted) {
		return peer.String()
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])
		if hop == "" {
			continue
		}
		a, ok := parseHostAddr(hop)
		if !ok {
			break
		}
		if !isTrusted(a, trusted) {
			return a.String()
		}
	}
	return peer.String()
}

// parseHostAddr parses an IP with or without a port ("192.0.2.1",
// "192.0.2.1:1234", "[2001:db8::1]:1234"); IPv4-mapped IPv6 is unmapped.
func parseHostAddr(s string) (netip.Addr, bool) {
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap(), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), true
	}
	return netip.Addr{}, false
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
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
