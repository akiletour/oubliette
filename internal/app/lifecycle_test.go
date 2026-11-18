package app_test

import (
	"net/http"
	"testing"
	"time"
)

func TestShareLinkExpiresByTheServerClock(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	inbox := member.InboxNamed("Default").ID
	week := member.ShareWith(inbox, expiring("Review week", i.Clock.Now().Add(48*time.Hour)))
	lasting := member.Share(inbox, "Acceptance")
	visitor := i.Browser()
	if week.ExpiresAt == nil || !week.ExpiresAt.Equal(i.Clock.Now().Add(48*time.Hour)) || lasting.ExpiresAt != nil {
		t.Errorf("created links = %+v and %+v, want the expiry only on the first", week, lasting)
	}

	i.Clock.Advance(47 * time.Hour)
	if !visitor.Opens(week) {
		t.Errorf("link before its expiry, want it to work")
	}
	if expired := expiredLabels(member.ShareLinks(inbox)); len(expired) != 0 {
		t.Errorf("expired links before the expiry = %q, want none", expired)
	}
	i.Clock.Advance(2 * time.Hour)
	if visitor.Opens(week) {
		t.Errorf("link after its expiry, want it refused")
	}
	if expired := expiredLabels(member.ShareLinks(inbox)); len(expired) != 1 || expired[0] != "Review week" {
		t.Errorf("expired links after the expiry = %q, want Review week, by the server clock", expired)
	}
	i.Clock.Advance(365 * 24 * time.Hour)
	if !visitor.Opens(lasting) {
		t.Errorf("link without expiry a year later, want it to work")
	}
}

func TestExpiryMustBeInTheFuture(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	path := "/api/inboxes/" + member.InboxNamed("Default").ID + "/share-links"

	for _, body := range []string{expiring("Past", i.Clock.Now().Add(-time.Minute)), expiring("Now", i.Clock.Now()), `{"label":"Odd","expiresAt":"next week"}`} {
		if refused := member.Do(http.MethodPost, path, body); refused.Status != http.StatusBadRequest {
			t.Errorf("create %s = %d %s, want 400", body, refused.Status, refused.Body)
		}
	}
}

func TestRevokingAShareLinkIsImmediateFinalAndLeavesTheOthers(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	inbox := member.InboxNamed("Default").ID
	first, second := member.Share(inbox, "First reviewer"), member.Share(inbox, "Second reviewer")
	visitor := i.Browser()
	revoke := "/api/inboxes/" + inbox + "/share-links/" + first.ID

	if revoked := member.Do(http.MethodDelete, revoke, ""); revoked.Status != http.StatusNoContent {
		t.Fatalf("revoke = %d %s, want 204", revoked.Status, revoked.Body)
	}
	if visitor.Opens(first) || !visitor.Opens(second) {
		t.Errorf("after revoking the first link, want it refused and the second working")
	}
	if again := member.Do(http.MethodDelete, revoke, ""); again.Status != http.StatusNotFound {
		t.Errorf("revoking it again = %d, want 404", again.Status)
	}
	if listed := member.ShareLinks(inbox); len(listed) != 1 || listed[0].ID != second.ID {
		t.Errorf("listed links = %+v, want the second only", listed)
	}

	other := member.CreateInbox("Other project")
	if elsewhere := member.Do(http.MethodDelete, "/api/inboxes/"+other.ID+"/share-links/"+second.ID, ""); elsewhere.Status != http.StatusNotFound {
		t.Errorf("revoking a link through another Inbox = %d, want 404", elsewhere.Status)
	}
	if anonymous := visitor.Do(http.MethodDelete, "/api/inboxes/"+inbox+"/share-links/"+second.ID, ""); anonymous.Status != http.StatusUnauthorized {
		t.Errorf("revoking without a session = %d, want 401", anonymous.Status)
	}
	if !visitor.Opens(second) {
		t.Errorf("second link after refused revocations, want it working")
	}
}

func TestSharingListShowsEachShareLinkAndWhenItWasLastOpened(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	inbox := member.InboxNamed("Default").ID
	created := i.Clock.Now()
	lasting := member.Share(inbox, "Acceptance")
	i.Clock.Advance(time.Minute)
	member.ShareWith(inbox, expiring("Review week", i.Clock.Now().Add(72*time.Hour)))
	visitor := i.Browser()

	listed := member.ShareLinks(inbox)
	if len(listed) != 2 || listed[0].Label != "Acceptance" || listed[0].Token != lasting.Token || !listed[0].CreatedAt.Equal(created) ||
		listed[0].LastOpenedAt != nil || listed[1].Label != "Review week" || listed[1].ExpiresAt == nil {
		t.Fatalf("listed links = %+v, want both, oldest first, with their dates and never opened", listed)
	}

	opened := i.Clock.Now()
	visitor.Opens(lasting)
	i.Clock.Advance(time.Minute)
	visitor.Get("/api/share/" + lasting.Token + "/messages")
	if seen := member.ShareLinks(inbox)[0].LastOpenedAt; seen == nil || !seen.Equal(opened) {
		t.Errorf("last opened = %v, want the first opening, recorded coarsely", seen)
	}
	i.Clock.Advance(time.Hour)
	visitor.Opens(lasting)
	if seen := member.ShareLinks(inbox)[0].LastOpenedAt; seen == nil || !seen.Equal(i.Clock.Now()) {
		t.Errorf("last opened an hour later = %v, want %v", seen, i.Clock.Now())
	}
	if shared := visitor.Get("/api/share/" + lasting.Token + "/share-links"); shared.Status != http.StatusNotFound {
		t.Errorf("listing links through a Share Link = %d, want 404", shared.Status)
	}
}

func TestEveryInactiveShareLinkGivesTheSameAnswer(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	inbox := member.InboxNamed("Default").ID
	expired := member.ShareWith(inbox, expiring("Expired", i.Clock.Now().Add(time.Hour)))
	revoked := member.Share(inbox, "Revoked")
	member.Do(http.MethodDelete, "/api/inboxes/"+inbox+"/share-links/"+revoked.ID, "")
	shop := member.CreateInbox("Shop staging")
	orphaned := member.Share(shop.ID, "Orphaned")
	member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID, "")
	i.Clock.Advance(2 * time.Hour)
	visitor := i.Browser()

	reference := visitor.Get("/api/share/UNKNOWNUNKNOWNUNKNOWNUNKNO")
	if reference.Status != http.StatusNotFound {
		t.Fatalf("unknown link = %d, want 404", reference.Status)
	}
	for _, link := range []shareLink{expired, revoked, orphaned} {
		for _, path := range []string{"/api/share/" + link.Token, "/api/share/" + link.Token + "/messages"} {
			if got := visitor.Get(path); got.Status != reference.Status || got.Body != reference.Body {
				t.Errorf("GET %s (%s) = %d %s, want the same answer as an unknown link", path, link.Label, got.Status, got.Body)
			}
		}
	}
}
