package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"weft/internal/directory"
)

// session holds the server-side state for one logged-in browser. Bind
// credentials live here in memory only -- never in the cookie, never logged.
// Every request re-binds to the directory with these credentials (passthrough),
// so there are no long-lived LDAP connections to go stale.
type session struct {
	id       string
	uid      string
	password string
	isAdmin  bool
	csrf     string
	expires  time.Time
}

// connect opens a fresh directory connection bound as this session's identity.
// The caller must Close it.
func (s *session) connect(ctx context.Context, dir directory.Directory) (directory.Conn, error) {
	if s.isAdmin {
		return dir.BindAdmin(ctx, s.password)
	}
	return dir.BindUser(ctx, s.uid, s.password)
}

// sessionStore is an in-memory session table with idle expiry.
type sessionStore struct {
	mu       sync.Mutex
	byID     map[string]*session
	ttl      time.Duration
	stopOnce sync.Once
	stop     chan struct{}
}

func newSessionStore(ttl time.Duration) *sessionStore {
	st := &sessionStore{
		byID: map[string]*session{},
		ttl:  ttl,
		stop: make(chan struct{}),
	}
	go st.reaper()
	return st
}

// create registers a new session for the given identity and returns it.
func (st *sessionStore) create(uid, password string, isAdmin bool) (*session, error) {
	id, err := randToken()
	if err != nil {
		return nil, err
	}
	csrf, err := randToken()
	if err != nil {
		return nil, err
	}
	s := &session{
		id:       id,
		uid:      uid,
		password: password,
		isAdmin:  isAdmin,
		csrf:     csrf,
		expires:  time.Now().Add(st.ttl),
	}
	st.mu.Lock()
	st.byID[id] = s
	st.mu.Unlock()
	return s, nil
}

// get returns the session for id if present and not expired, sliding its expiry.
func (st *sessionStore) get(id string) (*session, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	s, ok := st.byID[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(s.expires) {
		delete(st.byID, id)
		return nil, false
	}
	s.expires = time.Now().Add(st.ttl)
	return s, true
}

// destroy removes a session.
func (st *sessionStore) destroy(id string) {
	st.mu.Lock()
	delete(st.byID, id)
	st.mu.Unlock()
}

func (st *sessionStore) close() {
	st.stopOnce.Do(func() { close(st.stop) })
}

func (st *sessionStore) reaper() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-st.stop:
			return
		case <-t.C:
			now := time.Now()
			st.mu.Lock()
			for id, s := range st.byID {
				if now.After(s.expires) {
					delete(st.byID, id)
				}
			}
			st.mu.Unlock()
		}
	}
}

// randToken returns 32 bytes of URL-safe randomness.
func randToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
