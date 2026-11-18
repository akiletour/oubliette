package httpapi

import (
	"context"
	"net/http"

	"oubliette/internal/store"
)

type session struct {
	member store.Member
	token  string
}

type scope struct {
	session *session
	shared  *store.Inbox
}

type scopeKey struct{}

func scopeOf(r *http.Request) scope {
	return r.Context().Value(scopeKey{}).(scope)
}

func within(r *http.Request, current scope) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), scopeKey{}, current))
}

func unlisted(w http.ResponseWriter) {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (a *api) member(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		member, valid, err := a.store.SessionMember(r.Context(), cookie.Value, a.now())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if !valid {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		next(w, within(r, scope{session: &session{member: member, token: cookie.Value}}))
	})
}

func (a *api) visitor(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unlisted(w)
		inbox, found, err := a.store.OpenShareLink(r.Context(), r.PathValue("token"), a.now())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "this link is no longer active")
			return
		}
		next(w, within(r, scope{shared: &inbox}))
	})
}

func (a *api) inboxInScope(w http.ResponseWriter, r *http.Request) (store.Inbox, bool) {
	if shared := scopeOf(r).shared; shared != nil {
		return *shared, true
	}
	inbox, found, err := a.store.Inbox(r.Context(), r.PathValue("inbox"))
	if err != nil {
		a.fail(w, r, err)
		return store.Inbox{}, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "no such Inbox")
		return store.Inbox{}, false
	}
	return inbox, true
}
