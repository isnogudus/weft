package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"weft/internal/applog"
	"weft/internal/directory"
	"weft/internal/service"
)

// withConn binds to the directory as the current session and runs fn with the
// connection, closing it afterwards.
func (s *Server) withConn(w http.ResponseWriter, r *http.Request, fn func(c directory.Conn)) {
	sess := sessionFromCtx(r.Context())
	c, err := sess.connect(r.Context(), s.dir)
	if err != nil {
		writeDirError(w, err)
		return
	}
	defer c.Close()
	fn(c)
}

// --- auth / session ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.login.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "zu viele Versuche, bitte später erneut")
		return
	}
	var req loginReq
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Benutzername und Passwort erforderlich")
		return
	}

	isAdmin := s.cfg.IsAdminUID(req.Username)
	if isAdmin && !s.cfg.AllowAdmin {
		writeError(w, http.StatusForbidden, "Admin-Anmeldung ist deaktiviert")
		return
	}
	var (
		c   directory.Conn
		err error
	)
	if isAdmin {
		c, err = s.dir.BindAdmin(r.Context(), req.Password)
	} else {
		c, err = s.dir.BindUser(r.Context(), req.Username, req.Password)
	}
	if err != nil {
		if errors.Is(err, directory.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "ungültige Anmeldedaten")
			return
		}
		writeDirError(w, err)
		return
	}
	// Ask the same connection whether the base structure exists -- the only
	// moment weft holds credentials for a read. A non-admin cannot be missing
	// it: their own entry lives under the people OU, so a successful user bind
	// proves the directory is provisioned.
	needsSetup := false
	if isAdmin {
		ok, perr := c.Provisioned(r.Context())
		if perr != nil {
			// Don't block the login on a failed probe; the app itself will
			// report the real error on the first operation.
			applog.Warnf("login: provisioned check failed: %v", perr)
		}
		needsSetup = perr == nil && !ok
	}
	c.Close() // login only verifies; per-request binds happen later

	s.login.reset(ip)
	sess, err := s.sessions.create(req.Username, req.Password, isAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Sitzung konnte nicht erstellt werden")
		return
	}
	sess.needsSetup = needsSetup
	s.setSessionCookie(w, sess)
	writeJSON(w, http.StatusOK, meDTO{UID: sess.uid, IsAdmin: sess.isAdmin, CSRF: sess.csrf, NeedsSetup: sess.needsSetup})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(sessionCookie); err == nil {
		s.sessions.destroy(ck.Value)
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromCtx(r.Context())
	writeJSON(w, http.StatusOK, meDTO{
		UID: sess.uid, IsAdmin: sess.isAdmin, CSRF: sess.csrf, NeedsSetup: sess.needsSetup,
	})
}

// handleMeProfile returns the logged-in user's own directory entry. Works for
// non-admins (ldapd lets a user read its own entry); the admin is synthetic and
// has no entry, so it returns an empty profile.
func (s *Server) handleMeProfile(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromCtx(r.Context())
	if sess.isAdmin {
		writeJSON(w, http.StatusOK, userDTO{UID: sess.uid})
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		u, err := c.GetUser(r.Context(), sess.uid)
		if err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toUserDTO(u))
	})
}

// handleMeGroups returns the logged-in user's own effective groups. The admin
// is synthetic and has none.
func (s *Server) handleMeGroups(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromCtx(r.Context())
	if sess.isAdmin {
		writeJSON(w, http.StatusOK, []groupDTO{})
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		gs, err := c.EffectiveGroups(r.Context(), sess.uid)
		if err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toGroupDTOs(gs))
	})
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	c := s.cfg
	attrs := make([]userAttrDTO, 0, len(c.UserAttrs))
	for _, a := range c.UserAttrs {
		var opts []userAttrOptionDTO
		if len(a.Options) > 0 {
			opts = make([]userAttrOptionDTO, len(a.Options))
			for i, o := range a.Options {
				opts[i] = userAttrOptionDTO{Value: o.Value, LabelDE: o.Label("de"), LabelEN: o.Label("en")}
			}
		}
		attrs = append(attrs, userAttrDTO{
			Attr: a.Attr, LabelDE: a.Label("de"), LabelEN: a.Label("en"), Required: a.Required, Options: opts,
		})
	}
	writeJSON(w, http.StatusOK, metaDTO{
		BaseDN: c.BaseDN, UserIDAttr: c.UserIDAttr, PeopleOU: c.PeopleOU, GroupsOU: c.GroupsOU,
		PrimaryGroup: c.PrimaryGroup, DefaultShell: c.DefaultShell, HomeTemplate: c.HomeTemplate,
		UIDMin: c.UIDMin, UIDMax: c.UIDMax, GIDMin: c.GIDMin, GIDMax: c.GIDMax,
		MaxPwdLength: c.MaxPasswordLength, MailAttr: c.MailAttr, MailAliasAttr: c.MailAliasAttr,
		SessionTimeoutSeconds:     int(c.SessionTimeout.D().Seconds()),
		UserAttrs:                 attrs,
		TestUserGenerator:         c.TestUserGenerator,
		AdminCanChangeOwnPassword: c.AdminCanChangeOwnPassword,
	})
}

// rootPWMsg answers an admin password change that weft cannot carry out: the
// admin DN is the server's rootdn, whose password is configuration, not data.
const rootPWMsg = "das Admin-Passwort ist das rootpw des Verzeichnisservers und wird in dessen Konfiguration geändert (ldapd.conf rootpw / olcRootPW)"

// unverifiedPWMsg answers an admin password change that was written but could
// not be verified because the directory failed during the check.
const unverifiedPWMsg = "Passwort geschrieben, aber nicht bestätigt (Verzeichnisfehler) – bitte neu anmelden, um zu prüfen, welches Passwort gilt"

func (s *Server) handleChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromCtx(r.Context())
	if sess.isAdmin && !s.cfg.AdminCanChangeOwnPassword {
		writeError(w, http.StatusForbidden, "das Admin-Passwort kann in weft nicht geändert werden (admin_can_change_own_password = false)")
		return
	}
	var req passwordReq
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	if req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "neues Passwort erforderlich")
		return
	}
	// Verify the current password by re-binding, to defend against a hijacked
	// session changing the password.
	var (
		check directory.Conn
		err   error
	)
	if sess.isAdmin {
		check, err = s.dir.BindAdmin(r.Context(), req.OldPassword)
	} else {
		check, err = s.dir.BindUser(r.Context(), sess.uid, req.OldPassword)
	}
	if err != nil {
		writeError(w, http.StatusUnauthorized, "aktuelles Passwort ist falsch")
		return
	}
	check.Close()

	s.withConn(w, r, func(c directory.Conn) {
		// The admin's uid is only the login name; its entry is admin_dn, not
		// UserDN(uid). A synthetic rootdn has no entry at all -- its password
		// lives in the server's configuration and cannot be changed here.
		var err error
		if sess.isAdmin {
			err = s.svc.SetAdminPassword(r.Context(), c, req.NewPassword)
			if errors.Is(err, directory.ErrNotFound) {
				writeError(w, http.StatusConflict, rootPWMsg)
				return
			}
			// A rootdn that also exists as an entry (osixia/openldap creates
			// one) still binds against rootpw/olcRootPW, not the entry's
			// userPassword. Only report success if the new password binds.
			if err == nil {
				check, berr := s.dir.BindAdmin(r.Context(), req.NewPassword)
				switch {
				case berr == nil:
					check.Close()
				case errors.Is(berr, directory.ErrInvalidCredentials):
					// Set the entry to the password BindAdmin accepted above. For
					// a rootdn that is rootpw/olcRootPW, which need not be what
					// the entry held before; weft never reads password hashes
					// back, so this re-hashes the plaintext (bcrypt) rather than
					// restoring the old value.
					if rerr := s.svc.SetAdminPassword(r.Context(), c, req.OldPassword); rerr != nil {
						applog.Warnf("admin password: resetting the entry to the bind password failed: %v", rerr)
					}
					writeError(w, http.StatusConflict, rootPWMsg)
					return
				default:
					// The directory failed for another reason (unreachable,
					// timeout). The write went through, so a rollback would
					// undo a change that may well be in effect; report the
					// uncertainty instead. The session keeps the old password.
					applog.Warnf("admin password: written, but the verification bind failed: %v", berr)
					writeError(w, http.StatusBadGateway, unverifiedPWMsg)
					return
				}
			}
		} else {
			err = s.svc.SetPassword(r.Context(), c, sess.uid, req.NewPassword)
		}
		if err != nil {
			handleServiceError(w, err)
			return
		}
		// Keep the session usable with the new credentials.
		sess.password = req.NewPassword
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

// --- health ---

// handleHealth is the liveness/readiness probe for supervisors, container
// runtimes and monitoring: 200 while the directory server answers, 503 while it
// does not, so a poller can act on the status code alone. The body says no more
// than that -- no admin DN, no LDAP address, no error text, since this endpoint
// is unauthenticated; the reason is logged server-side instead.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.directoryUp(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, healthDTO{Status: "down", LDAP: "down"})
		return
	}
	writeJSON(w, http.StatusOK, healthDTO{Status: "ok", LDAP: "up"})
}

// directoryUp probes the directory server, reusing a recent result for
// healthTTL. Probes are polls: a container health check runs every few seconds
// and the SPA calls /setup/status on every page load, so without the cache each
// one would open its own LDAP connection. The lock is held across the dial on
// purpose -- concurrent probes then coalesce into the one connection instead of
// stampeding a server that may already be struggling.
func (s *Server) directoryUp(ctx context.Context) error {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	if !s.healthAt.IsZero() && time.Since(s.healthAt) < s.healthTTL {
		return s.healthErr
	}
	err := s.dir.Ping(ctx)
	s.healthAt, s.healthErr = time.Now(), err
	if err != nil {
		applog.Warnf("health: directory unreachable: %v", err)
	}
	return err
}

// --- setup ---

// handleSetupStatus answers the SPA's first request: is the directory server
// reachable at all, and under which identity does the admin bind? Whether the
// base structure exists is deliberately NOT answered here -- that needs a read,
// and an unauthenticated read is at the mercy of the server's ACLs (OpenLDAP
// commonly denies anonymous access, which used to make weft mistake a
// provisioned directory for a fresh one). The provisioned check happens after
// login, over the bound connection; see handleLogin.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	reachable := s.directoryUp(r.Context()) == nil
	writeJSON(w, http.StatusOK, setupStatusDTO{
		Reachable: reachable,
		AdminUID:  s.cfg.AdminUID,
		AdminDN:   s.cfg.AdminBindDN(),
	})
}

// handleBootstrap creates the base structure. It runs on the admin session's
// own connection -- no password is asked for a second time, and there is no
// unauthenticated endpoint that would tell an anonymous caller whether a
// guessed rootpw is correct.
func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromCtx(r.Context())
	s.withConn(w, r, func(c directory.Conn) {
		if err := s.svc.Bootstrap(r.Context(), c); err != nil {
			handleServiceError(w, err)
			return
		}
		sess.needsSetup = false
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

// --- users ---

// defaultUserPageSize / maxUserPageSize / minUserPageSize bound the "pageSize"
// query param on GET /users.
const (
	defaultUserPageSize = 25
	minUserPageSize     = 1
	maxUserPageSize     = 200
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("q")
	page := queryInt(r, "page", 1, 1, 1<<30)
	pageSize := queryInt(r, "pageSize", defaultUserPageSize, minUserPageSize, maxUserPageSize)
	s.withConn(w, r, func(c directory.Conn) {
		// LDAP has no offset-based pagination (see ldapclient.ListUsers): the
		// full (term-filtered) result set is fetched and sorted server-side as
		// today, and only the requested page is serialised to the client.
		us, err := c.ListUsers(r.Context(), term)
		if err != nil {
			writeDirError(w, err)
			return
		}
		total := len(us)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		writeJSON(w, http.StatusOK, userListDTO{
			Users: toUserDTOs(us[start:end]), Total: total, Page: page, PageSize: pageSize,
		})
	})
}

// queryInt parses an integer query param, clamped to [min, max]; an absent or
// invalid value falls back to def.
func queryInt(r *http.Request, key string, def, min, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	s.withConn(w, r, func(c directory.Conn) {
		u, err := c.GetUser(r.Context(), uid)
		if err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toUserDTO(u))
	})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		u, err := s.svc.CreateUser(r.Context(), c, service.NewUser{
			UID: req.UID, CN: req.CN, SN: req.SN,
			GivenName: req.GivenName, DisplayName: req.DisplayName,
			Password: req.Password, POSIX: req.POSIX.toInput(), Mail: req.Mail.toProfile(),
			Extra: req.Extra,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toUserDTO(u))
	})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	var req updateUserReq
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		u, err := s.svc.UpdateUser(r.Context(), c, service.NewUser{
			UID: uid, CN: req.CN, SN: req.SN,
			GivenName: req.GivenName, DisplayName: req.DisplayName,
			POSIX: req.POSIX.toInput(), Mail: req.Mail.toProfile(),
			Extra: req.Extra,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toUserDTO(u))
	})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	s.withConn(w, r, func(c directory.Conn) {
		if err := c.DeleteUser(r.Context(), uid); err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	var req passwordReq
	if err := readJSON(w, r, &req); err != nil || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "neues Passwort erforderlich")
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		if err := s.svc.SetPassword(r.Context(), c, uid, req.NewPassword); err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func (s *Server) handleRenameUser(w http.ResponseWriter, r *http.Request) {
	oldUID := chi.URLParam(r, "uid")
	var req struct {
		NewUID string `json:"newUid"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	if !service.ValidName(req.NewUID) {
		writeError(w, http.StatusBadRequest, "ungültige uid")
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		if err := c.RenameUID(r.Context(), oldUID, req.NewUID); err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func (s *Server) handleUserGroups(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	s.withConn(w, r, func(c directory.Conn) {
		gs, err := c.EffectiveGroups(r.Context(), uid)
		if err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toGroupDTOs(gs))
	})
}

// --- groups ---

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	s.withConn(w, r, func(c directory.Conn) {
		gs, err := c.ListGroups(r.Context())
		if err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toGroupDTOs(gs))
	})
}

func (s *Server) handleGetGroup(w http.ResponseWriter, r *http.Request) {
	cn := chi.URLParam(r, "cn")
	s.withConn(w, r, func(c directory.Conn) {
		g, err := c.GetGroup(r.Context(), cn)
		if err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toGroupDTO(g))
	})
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupReq
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ungültige Anfrage")
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		g, err := s.svc.CreateGroup(r.Context(), c, req.CN, req.GIDNumber)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toGroupDTO(g))
	})
}

func (s *Server) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	cn := chi.URLParam(r, "cn")
	s.withConn(w, r, func(c directory.Conn) {
		if err := c.DeleteGroup(r.Context(), cn); err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	cn := chi.URLParam(r, "cn")
	var req memberReq
	if err := readJSON(w, r, &req); err != nil || req.UID == "" {
		writeError(w, http.StatusBadRequest, "uid erforderlich")
		return
	}
	s.withConn(w, r, func(c directory.Conn) {
		if err := c.AddMember(r.Context(), cn, req.UID); err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	cn := chi.URLParam(r, "cn")
	uid := chi.URLParam(r, "uid")
	s.withConn(w, r, func(c directory.Conn) {
		if err := c.RemoveMember(r.Context(), cn, uid); err != nil {
			writeDirError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

// handleServiceError maps service/validation errors. Validation errors (plain
// errors) become 400; directory sentinels keep their mapped status.
func handleServiceError(w http.ResponseWriter, err error) {
	if isDirError(err) {
		writeDirError(w, err)
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func isDirError(err error) bool {
	for _, e := range []error{
		directory.ErrNotFound, directory.ErrAlreadyExists, directory.ErrPermission,
		directory.ErrInvalidCredentials, directory.ErrRangeExhausted,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
