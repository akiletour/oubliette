package app_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestMemberDeletesAMessageForMembersAndVisitors(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	link := member.Share(shop.ID, "Client")
	i.CaptureIn(shop, "Order")
	i.CaptureIn(shop, "Invoice")
	held := member.MessagesIn(shop.ID)
	invoice, order := held[0], held[1]

	if deleted := member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID+"/messages/"+invoice.ID, ""); deleted.Status != http.StatusNoContent {
		t.Fatalf("delete the Invoice = %d %s, want 204", deleted.Status, deleted.Body)
	}

	if left := member.MessagesIn(shop.ID); len(left) != 1 || left[0].ID != order.ID {
		t.Errorf("Messages after deleting the Invoice = %+v, want only the Order", left)
	}
	if shown := member.InboxNamed("Shop").MessageCount; shown != 1 {
		t.Errorf("Shop counts %d Messages, want 1", shown)
	}
	if opened := member.Get("/api/inboxes/" + shop.ID + "/messages/" + invoice.ID); opened.Status != http.StatusNotFound {
		t.Errorf("the Member opening the deleted Message = %d, want 404", opened.Status)
	}
	if opened := i.Browser().Get("/api/share/" + link.Token + "/messages/" + invoice.ID); opened.Status != http.StatusNotFound {
		t.Errorf("the Visitor opening the deleted Message = %d, want 404", opened.Status)
	}
	var shared struct{ Messages []message }
	i.Browser().Get("/api/share/"+link.Token+"/messages").JSON(t, &shared)
	if len(shared.Messages) != 1 || shared.Messages[0].ID != order.ID {
		t.Errorf("the Visitor's list after deleting the Invoice = %+v, want only the Order", shared.Messages)
	}
}

func TestDeletingWhatIsNotThereIsNotFound(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	i.CaptureIn(shop, "Order")
	order := member.MessagesIn(shop.ID)[0].ID
	member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID+"/messages/"+order, "")

	for _, attempt := range []struct{ path, refusal string }{
		{"/api/inboxes/" + shop.ID + "/messages/" + order, "no such Message"},
		{"/api/inboxes/no-such-inbox/messages/" + order, "no such Inbox"},
		{"/api/inboxes/no-such-inbox/messages", "no such Inbox"},
	} {
		refused := member.Do(http.MethodDelete, attempt.path, "")
		if refused.Status != http.StatusNotFound || !strings.Contains(refused.Body, attempt.refusal) {
			t.Errorf("DELETE %s = %d %s, want 404 %q", attempt.path, refused.Status, refused.Body, attempt.refusal)
		}
	}
}

func TestOnlyAMemberOfTheInboxDeletesOrEmpties(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	blog := member.CreateInbox("Blog")
	link := member.Share(shop.ID, "Client")
	i.CaptureIn(shop, "Order")
	order := member.MessagesIn(shop.ID)[0].ID
	visitor, anonymous := i.Browser(), i.Browser()

	for _, attempt := range []struct {
		who  *browser
		path string
	}{
		{visitor, "/api/share/" + link.Token + "/messages/" + order},
		{visitor, "/api/share/" + link.Token + "/messages"},
		{anonymous, "/api/inboxes/" + shop.ID + "/messages/" + order},
		{anonymous, "/api/inboxes/" + shop.ID + "/messages"},
		{member, "/api/inboxes/" + blog.ID + "/messages/" + order},
	} {
		if refused := attempt.who.Do(http.MethodDelete, attempt.path, ""); refused.Status < http.StatusBadRequest {
			t.Errorf("DELETE %s = %d, want a refusal", attempt.path, refused.Status)
		}
	}

	if held := member.MessagesIn(shop.ID); len(held) != 1 || held[0].ID != order {
		t.Errorf("Messages of Shop after the refused attempts = %+v, want the Order kept", held)
	}
}

func TestEmptyingAnInboxKeepsItsCredentialsAndShareLinks(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	blog := member.CreateInbox("Blog")
	link := member.Share(shop.ID, "Client")
	i.CaptureIn(shop, "Order")
	i.CaptureIn(shop, "Invoice")
	i.CaptureIn(blog, "Blog post")

	if emptied := member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID+"/messages", ""); emptied.Status != http.StatusNoContent {
		t.Fatalf("empty Shop = %d %s, want 204", emptied.Status, emptied.Body)
	}

	if left := member.MessagesIn(shop.ID); len(left) != 0 {
		t.Errorf("Messages of the emptied Shop = %+v, want none", left)
	}
	if kept := member.MessagesIn(blog.ID); len(kept) != 1 {
		t.Errorf("Messages of Blog = %+v, want its Blog post kept", kept)
	}
	if links := member.ShareLinks(shop.ID); len(links) != 1 || !i.Browser().Opens(link) {
		t.Errorf("Share Links of the emptied Shop = %+v, want the Client link kept and open", links)
	}
	i.CaptureIn(shop, "Next round")
	if held := member.MessagesIn(shop.ID); len(held) != 1 || held[0].Subject != "Next round" {
		t.Errorf("Messages sent after emptying = %+v, want the Credentials still working", held)
	}
}
