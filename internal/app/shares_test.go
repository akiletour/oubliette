package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

type sharedInbox struct {
	Name         string
	MessageCount int
}

func TestVisitorReadsAnInboxThroughAShareLink(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(newsletter)
	link := member.Share(member.InboxNamed("Default").ID, "Client Lumière")
	if link.Label != "Client Lumière" || len(link.Token) < 26 {
		t.Fatalf("created Share Link = %+v, want its label and a token of at least 128 bits", link)
	}
	visitor := i.Browser()
	i.Clock.Advance(time.Minute)
	i.Capture(welcome)

	page := visitor.Get("/s/" + link.Token)
	if page.Status != http.StatusOK || !strings.Contains(page.Body, `<div id="root">`) {
		t.Fatalf("Share Link page = %d %s, want the interface", page.Status, page.Body)
	}
	for name, want := range map[string]string{"X-Robots-Tag": "noindex, nofollow", "Referrer-Policy": "no-referrer"} {
		if got := page.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if nested := visitor.Get("/s/" + link.Token + "/messages/any"); nested.Status != http.StatusOK {
		t.Errorf("Message page under the Share Link = %d, want the interface", nested.Status)
	}

	var shared sharedInbox
	visitor.Get("/api/share/"+link.Token).JSON(t, &shared)
	if shared != (sharedInbox{"Default", 2}) {
		t.Errorf("shared Inbox = %+v, want Default with its two Messages", shared)
	}
	var listed messageList
	visitor.Get("/api/share/"+link.Token+"/messages").JSON(t, &listed)
	if len(listed.Messages) != 2 || listed.Messages[0].Subject != "Welcome" || listed.Messages[1].Subject != "Spring sale" {
		t.Fatalf("shared Messages = %+v, want both, including the one received before the link", listed.Messages)
	}

	path := "/api/share/" + link.Token + "/messages/" + listed.Messages[1].ID
	var opened openedMessage
	visitor.Get(path).JSON(t, &opened)
	if opened.Subject != "Spring sale" || opened.HTML == "" {
		t.Errorf("opened Message = %+v, want the sale with its HTML version", opened)
	}
	rendered := visitor.Get(path + "/rendered")
	if rendered.Status != http.StatusOK || rendered.Header.Get("Referrer-Policy") != "no-referrer" ||
		!strings.Contains(rendered.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("rendered Message = %d %v, want the sandboxed HTML without referrer", rendered.Status, rendered.Header)
	}
}

func TestShareLinkReadsOnlyItsInboxAndChangesNothing(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shared := member.InboxNamed("Default")
	other := member.CreateInbox("Secret project")
	if err := i.Submit(plain(other.Credentials()), "app@secret.example", []string{"a@example.com"}, welcome); err != nil {
		t.Fatalf("submit to the other Inbox: %v", err)
	}
	hidden := member.MessagesIn(other.ID)[0].ID
	link := member.Share(shared.ID, "Reviewer")
	visitor := i.Browser()
	base := "/api/share/" + link.Token

	for _, path := range []string{base, base + "/messages"} {
		body := visitor.Get(path).Body
		for _, secret := range []string{"Secret project", other.ID, other.SMTPUsername, other.SMTPPassword, hidden, shared.ID, "Reviewer"} {
			if strings.Contains(body, secret) {
				t.Errorf("GET %s = %s, want no trace of %q", path, body, secret)
			}
		}
	}
	for _, path := range []string{
		base + "/messages/" + hidden, base + "/messages/" + hidden + "/rendered", "/api/share/" + link.Token + "x/messages",
		"/api/share%2F" + link.Token + "/messages", "/api/share/" + link.Token + "%2F..%2F..%2Finboxes%2F" + other.ID + "/messages",
	} {
		if guessed := visitor.Get(path); guessed.Status != http.StatusNotFound || strings.Contains(guessed.Body, "Welcome") {
			t.Errorf("GET %s = %d %s, want 404", path, guessed.Status, guessed.Body)
		}
	}

	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, base + "/messages", welcome},
		{http.MethodDelete, base, ""},
		{http.MethodPatch, base, named("Renamed")},
		{http.MethodPost, base + "/share-links", labelled("Another")},
		{http.MethodPost, "/api/inboxes", named("Forged")},
		{http.MethodPatch, "/api/inboxes/" + shared.ID, named("Renamed")},
		{http.MethodDelete, "/api/inboxes/" + other.ID, ""},
		{http.MethodPost, "/api/inboxes/" + shared.ID + "/share-links", labelled("Another")},
		{http.MethodGet, "/api/inboxes", ""},
		{http.MethodGet, "/api/inboxes/" + other.ID + "/messages", ""},
	} {
		if refused := visitor.Do(request.method, request.path, request.body); refused.Status < 400 {
			t.Errorf("%s %s with a Share Link only = %d, want a refusal", request.method, request.path, refused.Status)
		}
	}
	if listed := names(member.Inboxes()); len(listed) != 2 || listed[0] != "Default" || listed[1] != "Secret project" {
		t.Errorf("Inboxes after the Visitor's attempts = %q, want them untouched", listed)
	}
}

func TestUnknownOrOrphanedShareLinkIsNoLongerActive(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop staging")
	link := member.Share(shop.ID, "Reviewer")
	member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID, "")
	visitor := i.Browser()

	for _, token := range []string{"unknown", link.Token} {
		answer := visitor.Get("/api/share/" + token)
		if answer.Status != http.StatusNotFound || !strings.Contains(answer.Body, "no longer active") {
			t.Errorf("Share Link %s = %d %s, want 404 no longer active", token, answer.Status, answer.Body)
		}
		if page := visitor.Get("/s/" + token); page.Status != http.StatusOK || page.Header.Get("X-Robots-Tag") == "" {
			t.Errorf("page of Share Link %s = %d, want the interface, which explains it", token, page.Status)
		}
	}
}

func TestShareLinksAreCreatedByMembersOnly(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	inbox := member.InboxNamed("Default")

	first, second := member.Share(inbox.ID, "First"), member.Share(inbox.ID, "Second")
	if first.Token == second.Token || first.ID == second.ID {
		t.Errorf("Share Links %+v and %+v, want distinct tokens", first, second)
	}
	if refused := member.Do(http.MethodPost, "/api/inboxes/"+inbox.ID+"/share-links", labelled("  ")); refused.Status != http.StatusBadRequest {
		t.Errorf("Share Link without a label = %d, want 400", refused.Status)
	}
	if refused := member.Do(http.MethodPost, "/api/inboxes/unknown/share-links", labelled("Reviewer")); refused.Status != http.StatusNotFound {
		t.Errorf("Share Link of an unknown Inbox = %d, want 404", refused.Status)
	}
	if anonymous := i.Browser().Do(http.MethodPost, "/api/inboxes/"+inbox.ID+"/share-links", labelled("Reviewer")); anonymous.Status != http.StatusUnauthorized {
		t.Errorf("Share Link created without a session = %d, want 401", anonymous.Status)
	}
}
