package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"oubliette/internal/content"
	"oubliette/internal/store"
)

const (
	sessionCookie   = "oubliette_session"
	sessionLifetime = 30 * 24 * time.Hour
	signInAttempts  = 5
	signInLockout   = 15 * time.Minute
	renderingPolicy = "sandbox allow-popups allow-popups-to-escape-sandbox; default-src 'none'; " +
		"img-src * data:; style-src * 'unsafe-inline'; font-src * data:"
)

type api struct {
	store    *store.Store
	now      func() time.Time
	log      *slog.Logger
	throttle throttle
}

func New(storage *store.Store, ui fs.FS, now func() time.Time, log *slog.Logger) http.Handler {
	a := &api{store: storage, now: now, log: log, throttle: throttle{streaks: map[[sha256.Size]byte]streak{}}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /api/session", a.signIn)

	mux.Handle("GET /api/session", a.member(a.showSession))
	mux.Handle("DELETE /api/session", a.member(a.signOut))
	mux.Handle("GET /api/inboxes", a.member(a.listInboxes))
	mux.Handle("POST /api/inboxes", a.member(a.createInbox))
	mux.Handle("PATCH /api/inboxes/{inbox}", a.member(a.renameInbox))
	mux.Handle("DELETE /api/inboxes/{inbox}", a.member(a.deleteInbox))
	mux.Handle("GET /api/inboxes/{inbox}/messages", a.member(a.listMessages))
	mux.Handle("GET /api/inboxes/{inbox}/messages/{message}", a.member(a.openMessage))
	mux.Handle("GET /api/inboxes/{inbox}/messages/{message}/rendered", a.member(a.renderMessage))
	mux.Handle("POST /api/inboxes/{inbox}/share-links", a.member(a.createShareLink))

	mux.Handle("GET /api/share/{token}", a.visitor(a.showSharedInbox))
	mux.Handle("GET /api/share/{token}/messages", a.visitor(a.listMessages))
	mux.Handle("GET /api/share/{token}/messages/{message}", a.visitor(a.openMessage))
	mux.Handle("GET /api/share/{token}/messages/{message}/rendered", a.visitor(a.renderMessage))

	mux.HandleFunc("GET /inboxes/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, ui, "index.html")
	})
	mux.HandleFunc("GET /s/", func(w http.ResponseWriter, r *http.Request) {
		unlisted(w)
		http.ServeFileFS(w, r, ui, "index.html")
	})
	mux.Handle("/", http.FileServerFS(ui))
	return http.NewCrossOriginProtection().Handler(mux)
}

type streak struct {
	attempts int
	last     time.Time
}

type throttle struct {
	mu      sync.Mutex
	streaks map[[sha256.Size]byte]streak
}

func (t *throttle) allow(email string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	maps.DeleteFunc(t.streaks, func(_ [sha256.Size]byte, s streak) bool {
		return now.Sub(s.last) >= signInLockout
	})
	key := sha256.Sum256([]byte(email))
	current := t.streaks[key]
	if current.attempts >= signInAttempts {
		return false
	}
	t.streaks[key] = streak{attempts: current.attempts + 1, last: now}
	return true
}

func (t *throttle) forget(email string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.streaks, sha256.Sum256([]byte(email)))
}

func (a *api) signIn(w http.ResponseWriter, r *http.Request) {
	var submitted struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &submitted) {
		return
	}
	now := a.now()
	email := store.NormalizeEmail(submitted.Email)
	if !a.throttle.allow(email, now) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	member, valid, err := a.store.CheckPassword(r.Context(), email, submitted.Password)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !valid {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	a.throttle.forget(email)
	token, err := a.store.StartSession(r.Context(), member, now, now.Add(sessionLifetime))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	setSessionCookie(w, r, token, int(sessionLifetime.Seconds()))
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) showSession(w http.ResponseWriter, r *http.Request) {
	current := scopeOf(r).session
	writeJSON(w, http.StatusOK, map[string]string{"email": current.member.Email})
}

func (a *api) signOut(w http.ResponseWriter, r *http.Request) {
	current := scopeOf(r).session
	if err := a.store.EndSession(r.Context(), current.token); err != nil {
		a.fail(w, r, err)
		return
	}
	setSessionCookie(w, r, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) listInboxes(w http.ResponseWriter, r *http.Request) {
	inboxes, err := a.store.Inboxes(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"inboxes": inboxes})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request")
		return false
	}
	return true
}

func readName(w http.ResponseWriter, r *http.Request) (string, bool) {
	var submitted struct {
		Name string `json:"name"`
	}
	ok := decode(w, r, &submitted)
	return submitted.Name, ok
}

func (a *api) createInbox(w http.ResponseWriter, r *http.Request) {
	name, ok := readName(w, r)
	if !ok {
		return
	}
	inbox, err := a.store.CreateInbox(r.Context(), name, a.now())
	if a.refused(w, r, err) {
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		store.Inbox
		SMTPUsername string `json:"smtpUsername"`
		SMTPPassword string `json:"smtpPassword"`
	}{inbox, inbox.Username, inbox.Password})
}

func (a *api) renameInbox(w http.ResponseWriter, r *http.Request) {
	name, ok := readName(w, r)
	if !ok {
		return
	}
	if !a.refused(w, r, a.store.RenameInbox(r.Context(), r.PathValue("inbox"), name)) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *api) deleteInbox(w http.ResponseWriter, r *http.Request) {
	if !a.refused(w, r, a.store.DeleteInbox(r.Context(), r.PathValue("inbox"))) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *api) createShareLink(w http.ResponseWriter, r *http.Request) {
	var submitted struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &submitted) {
		return
	}
	link, err := a.store.CreateShareLink(r.Context(), r.PathValue("inbox"), submitted.Label, a.now())
	if !a.refused(w, r, err) {
		writeJSON(w, http.StatusCreated, link)
	}
}

func (a *api) showSharedInbox(w http.ResponseWriter, r *http.Request) {
	shared := scopeOf(r).shared
	writeJSON(w, http.StatusOK, struct {
		Name         string `json:"name"`
		MessageCount int    `json:"messageCount"`
	}{shared.Name, shared.MessageCount})
}

func (a *api) refused(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, store.ErrInboxName), errors.Is(err, store.ErrShareLinkLabel):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrInboxUnknown):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		a.fail(w, r, err)
	default:
		return false
	}
	return true
}

func (a *api) listMessages(w http.ResponseWriter, r *http.Request) {
	inbox, found := a.inboxInScope(w, r)
	if !found {
		return
	}
	messages, next, err := a.store.Messages(r.Context(), inbox.ID, r.URL.Query().Get("before"))
	if errors.Is(err, store.ErrPosition) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages, "next": next})
}

func (a *api) heldMessage(w http.ResponseWriter, r *http.Request) (store.Message, content.Body, bool) {
	inbox, found := a.inboxInScope(w, r)
	if !found {
		return store.Message{}, content.Body{}, false
	}
	message, found, err := a.store.Message(r.Context(), inbox.ID, r.PathValue("message"))
	if err != nil {
		a.fail(w, r, err)
		return store.Message{}, content.Body{}, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such Message")
		return store.Message{}, content.Body{}, false
	}
	return message, content.Read(message.Raw), true
}

func (a *api) openMessage(w http.ResponseWriter, r *http.Request) {
	message, body, found := a.heldMessage(w, r)
	if !found {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		store.Message
		Text    string `json:"text"`
		HasHTML bool   `json:"hasHTML"`
	}{message, body.Text, body.HTML != ""})
}

func (a *api) renderMessage(w http.ResponseWriter, r *http.Request) {
	_, body, found := a.heldMessage(w, r)
	if !found {
		return
	}
	if body.HTML == "" {
		writeError(w, http.StatusNotFound, "this Message has no HTML version")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", renderingPolicy)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.WriteString(w, body.Rendered())
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	forwardedProto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(strings.TrimSpace(forwardedProto), "https"),
	})
}

func (a *api) fail(w http.ResponseWriter, r *http.Request, err error) {
	a.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
