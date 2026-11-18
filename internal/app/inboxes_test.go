package app_test

import (
	"errors"
	"io"
	"maps"
	"net/http"
	"net/textproto"
	"slices"
	"strings"
	"testing"
)

func TestMemberCreatesAnInboxWhoseCredentialsHoldMessagesThere(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	first := i.LoggedCredentials()[0]

	shop := member.CreateInbox("Shop staging")
	blog := member.CreateInbox("Blog staging")
	for _, created := range []createdInbox{shop, blog} {
		if created.ID == "" || len(created.SMTPUsername) < 16 || len(created.SMTPPassword) < 26 {
			t.Errorf("created Inbox = %+v, want an identifier and generated Credentials", created)
		}
	}
	if shop.SMTPUsername == blog.SMTPUsername || shop.SMTPPassword == blog.SMTPPassword || shop.SMTPUsername == first.Username {
		t.Errorf("Credentials %+v and %+v, want every Inbox to have its own", shop, blog)
	}

	if err := i.Submit(plain(shop.Credentials()), "app@shop.example", []string{"a@example.com"}, welcome); err != nil {
		t.Fatalf("submit with the new Credentials: %v", err)
	}

	counts := map[string]int{}
	for _, inbox := range member.Inboxes() {
		counts[inbox.Name] = inbox.MessageCount
	}
	if want := map[string]int{"Blog staging": 0, "Default": 0, "Shop staging": 1}; !maps.Equal(counts, want) {
		t.Errorf("Inbox counts = %v, want %v", counts, want)
	}
	held := member.MessagesIn(shop.ID)
	if len(held) != 1 || held[0].Subject != "Welcome" {
		t.Fatalf("Messages of the new Inbox = %+v, want the one its Application sent", held)
	}
	for _, other := range []string{blog.ID, member.InboxNamed("Default").ID} {
		if elsewhere := member.MessagesIn(other); len(elsewhere) != 0 {
			t.Errorf("Messages of another Inbox = %+v, want none", elsewhere)
		}
		if guessed := member.Get("/api/inboxes/" + other + "/messages/" + held[0].ID); guessed.Status != http.StatusNotFound {
			t.Errorf("opening the Message through another Inbox = %d, want 404", guessed.Status)
		}
	}
}

func TestInboxesAreListedByName(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	for _, name := range []string{"beta", "Alpha", "gamma"} {
		member.CreateInbox(name)
	}

	for _, name := range []string{"zeta", "Émile", "éclair"} {
		member.CreateInbox(name)
	}

	if listed := names(member.Inboxes()); !slices.Equal(listed, []string{"Alpha", "beta", "Default", "éclair", "Émile", "gamma", "zeta"}) {
		t.Errorf("Inboxes = %q, want them sorted by name whatever the case and the accents", listed)
	}
}

func TestDeletingEveryInboxIsNotUndoneByARestart(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	if deleted := member.Do(http.MethodDelete, "/api/inboxes/"+member.InboxNamed("Default").ID, ""); deleted.Status != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", deleted.Status)
	}

	i.Restart()

	if listed := member.Inboxes(); len(listed) != 0 {
		t.Errorf("Inboxes after a restart = %+v, want none", listed)
	}
	if logged := i.LoggedCredentials(); len(logged) != 1 {
		t.Errorf("logged Credentials = %d times, want only at the first start", len(logged))
	}
}

func TestInboxDeletedDuringATransactionRefusesItsMessage(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop staging")
	client, err := i.Connect(plain(shop.Credentials()))
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if err := errors.Join(client.Mail("app@shop.example"), client.Rcpt("a@example.com")); err != nil {
		t.Fatalf("start the transaction: %v", err)
	}

	member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID, "")

	var refusal *textproto.Error
	body, err := client.Data()
	if err == nil {
		io.WriteString(body, welcome)
		err = body.Close()
	}
	if !errors.As(err, &refusal) || refusal.Code != 535 {
		t.Errorf("data after the deletion = %v, want 535", err)
	}
	if strings.Contains(i.logs.String(), `"level":"ERROR"`) {
		t.Errorf("logs = %s, want no error for a deleted Inbox", i.logs.String())
	}
}

func TestRenamingAnInboxKeepsItsCredentialsAndMessages(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(welcome)
	inbox := member.InboxNamed("Default")

	if renamed := member.Do(http.MethodPatch, "/api/inboxes/"+inbox.ID, named(" Main shop ")); renamed.Status != http.StatusNoContent {
		t.Fatalf("rename = %d %s, want 204", renamed.Status, renamed.Body)
	}
	i.Capture(welcome)

	if listed := member.Inboxes(); len(listed) != 1 || listed[0] != (listedInbox{inbox.ID, "Main shop", 2}) {
		t.Errorf("Inboxes after renaming = %+v, want Main shop with both Messages", listed)
	}
	if refused := member.Do(http.MethodPatch, "/api/inboxes/"+inbox.ID, named("")); refused.Status != http.StatusBadRequest {
		t.Errorf("renaming to an empty name = %d, want 400", refused.Status)
	}
	if refused := member.Do(http.MethodPatch, "/api/inboxes/unknown", named("Other")); refused.Status != http.StatusNotFound {
		t.Errorf("renaming an unknown Inbox = %d, want 404", refused.Status)
	}
	if listed := names(member.Inboxes()); !slices.Equal(listed, []string{"Main shop"}) {
		t.Errorf("Inboxes after refused renamings = %q, want Main shop", listed)
	}
}

func TestDeletingAnInboxRemovesItsMessagesAndCutsOffItsCredentials(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(welcome)
	shop := member.CreateInbox("Shop staging")
	if err := i.Submit(plain(shop.Credentials()), "app@shop.example", []string{"a@example.com"}, welcome); err != nil {
		t.Fatalf("submit: %v", err)
	}
	held := member.MessagesIn(shop.ID)[0].ID
	connected, err := i.Connect(plain(shop.Credentials()))
	if err != nil {
		t.Fatalf("authenticate before the deletion: %v", err)
	}

	if deleted := member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID, ""); deleted.Status != http.StatusNoContent {
		t.Fatalf("delete = %d %s, want 204", deleted.Status, deleted.Body)
	}

	if listed := member.Inboxes(); len(listed) != 1 || listed[0].Name != "Default" || listed[0].MessageCount != 1 {
		t.Errorf("Inboxes after the deletion = %+v, want Default and its Message only", listed)
	}
	for _, path := range []string{"/api/inboxes/" + shop.ID + "/messages", "/api/inboxes/" + shop.ID + "/messages/" + held} {
		if gone := member.Get(path); gone.Status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, gone.Status)
		}
	}
	var refusal *textproto.Error
	if err := i.Submit(plain(shop.Credentials()), "app@shop.example", []string{"a@example.com"}, welcome); !errors.As(err, &refusal) || refusal.Code != 535 {
		t.Errorf("new connection with the deleted Credentials = %v, want 535", err)
	}
	if err := transmit(connected, "app@shop.example", []string{"a@example.com"}, welcome); !errors.As(err, &refusal) || refusal.Code != 535 {
		t.Errorf("connection authenticated before the deletion = %v, want 535", err)
	}
	if again := member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID, ""); again.Status != http.StatusNotFound {
		t.Errorf("deleting it again = %d, want 404", again.Status)
	}
}

func TestInboxChangesFromAnotherSiteAreRefused(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	inbox := member.InboxNamed("Default")

	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/inboxes", named("Forged")},
		{http.MethodPatch, "/api/inboxes/" + inbox.ID, named("Forged")},
		{http.MethodDelete, "/api/inboxes/" + inbox.ID, ""},
	} {
		if forged := member.Do(request.method, request.path, request.body, "Sec-Fetch-Site", "cross-site"); forged.Status != http.StatusForbidden {
			t.Errorf("cross-site %s %s = %d, want 403", request.method, request.path, forged.Status)
		}
	}
	if listed := names(member.Inboxes()); !slices.Equal(listed, []string{"Default"}) {
		t.Errorf("Inboxes = %q, want them untouched", listed)
	}
}

func TestAnInboxNeedsAName(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()

	for _, body := range []string{
		named(""), named("   "), named(strings.Repeat("n", 101)), named("​"), named("Shop\nstaging"),
		named("shop‮evil"), named("nul\x00"), `{"name":`, `{}`,
	} {
		if created := member.Do(http.MethodPost, "/api/inboxes", body); created.Status != http.StatusBadRequest {
			t.Errorf("create Inbox with %s = %d %s, want 400", body, created.Status, created.Body)
		}
	}
	if created := member.CreateInbox("  Padded  "); created.Name != "Padded" {
		t.Errorf("created name = %q, want the name without surrounding spaces", created.Name)
	}
	if created := member.CreateInbox(strings.Repeat("é", 100)); created.Name != strings.Repeat("é", 100) {
		t.Errorf("created name = %q, want a hundred accented characters accepted", created.Name)
	}
	if listed := names(member.Inboxes()); !slices.Equal(listed, []string{"Default", strings.Repeat("é", 100), "Padded"}) {
		t.Errorf("Inboxes = %q, want the refused ones absent", listed)
	}
}
