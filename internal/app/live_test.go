package app_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const patience = 5 * time.Second

type event struct {
	Name string
	ID   string
}

type stream struct {
	t      *testing.T
	events chan event
}

func (b *browser) Stream(place string) *stream {
	t := b.i.t
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.i.URL+place+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.client.Do(req)
	if err != nil {
		t.Fatalf("open the event stream of %s: %v", place, err)
	}
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		res.Body.Close()
		t.Fatalf("event stream of %s = %d %s, want 200 text/event-stream", place, res.StatusCode, res.Header.Get("Content-Type"))
	}
	s := &stream{t: t, events: make(chan event, 16)}
	go func() {
		defer res.Body.Close()
		defer close(s.events)
		lines := bufio.NewScanner(res.Body)
		var name, data string
		for lines.Scan() {
			switch line := lines.Text(); {
			case line == "":
				if name != "" {
					var received struct{ ID string }
					if err := json.Unmarshal([]byte(data), &received); err != nil {
						received.ID = "unreadable data " + data
					}
					s.events <- event{Name: name, ID: received.ID}
				}
				name, data = "", ""
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return s
}

func (s *stream) Next() (event, bool) {
	s.t.Helper()
	select {
	case next, open := <-s.events:
		return next, open
	case <-time.After(patience):
		s.t.Fatal("no event and the stream is still open")
		return event{}, false
	}
}

func (i *instance) CaptureIn(inbox createdInbox, subject string) {
	i.t.Helper()
	data := "From: app@example.com\r\nSubject: " + subject + "\r\n\r\nHello.\r\n"
	if err := i.Submit(plain(inbox.Credentials()), "app@example.com", []string{"a@example.com"}, data); err != nil {
		i.t.Fatalf("submit %s: %v", subject, err)
	}
}

func TestMembersAndVisitorsHearOfEachMessageArrivingInTheirInbox(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	blog := member.CreateInbox("Blog")
	visitor := i.Browser().Stream("/api/share/" + member.Share(shop.ID, "Client").Token)
	watching := member.Stream("/api/inboxes/" + shop.ID)

	i.CaptureIn(blog, "Blog post")
	i.CaptureIn(shop, "Order")

	order := member.MessagesIn(shop.ID)[0].ID
	for who, events := range map[string]*stream{"Visitor": visitor, "Member": watching} {
		if got, open := events.Next(); !open || got != (event{Name: "received", ID: order}) {
			t.Errorf("the %s watching Shop first hears %+v (open %v), want the Order received", who, got, open)
		}
	}
}

func TestRevokingAShareLinkClosesItsEventStream(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	link := member.Share(shop.ID, "Client")
	visitor := i.Browser()
	events := visitor.Stream("/api/share/" + link.Token)

	member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID+"/share-links/"+link.ID, "")

	if got, open := events.Next(); open {
		t.Errorf("the revoked Visitor hears %+v, want the stream closed", got)
	}
	if again := visitor.Get("/api/share/" + link.Token + "/events"); again.Status != http.StatusNotFound {
		t.Errorf("reopening the stream of a revoked link = %d, want 404", again.Status)
	}
}

func TestRevokingAShareLinkKeepsOtherStreamsOfTheInboxOpen(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	revoked := member.Share(shop.ID, "Former agency")
	kept := i.Browser().Stream("/api/share/" + member.Share(shop.ID, "Client").Token)
	watching := member.Stream("/api/inboxes/" + shop.ID)

	member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID+"/share-links/"+revoked.ID, "")
	i.CaptureIn(shop, "Order")

	for who, events := range map[string]*stream{"Visitor of the kept link": kept, "Member": watching} {
		if got, open := events.Next(); !open || got.Name != "received" {
			t.Errorf("the %s hears %+v (open %v), want the Order received", who, got, open)
		}
	}
}

func TestDeletingAnInboxClosesItsEventStreams(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	visitor := i.Browser().Stream("/api/share/" + member.Share(shop.ID, "Client").Token)
	watching := member.Stream("/api/inboxes/" + shop.ID)

	member.Do(http.MethodDelete, "/api/inboxes/"+shop.ID, "")

	for who, events := range map[string]*stream{"Visitor": visitor, "Member": watching} {
		if got, open := events.Next(); open {
			t.Errorf("the %s of the deleted Inbox hears %+v, want the stream closed", who, got)
		}
	}
}

func TestEventStreamsAreScoped(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	inbox := member.InboxID()

	if anonymous := i.Browser().Get("/api/inboxes/" + inbox + "/events"); anonymous.Status != http.StatusUnauthorized {
		t.Errorf("event stream without a session = %d, want 401", anonymous.Status)
	}
	if unknown := member.Get("/api/inboxes/no-such-inbox/events"); unknown.Status != http.StatusNotFound {
		t.Errorf("event stream of an unknown Inbox = %d, want 404", unknown.Status)
	}
	if wrong := i.Browser().Get("/api/share/no-such-token/events"); wrong.Status != http.StatusNotFound {
		t.Errorf("event stream of an unknown Share Link = %d, want 404", wrong.Status)
	}
}

func TestStopDoesNotWaitForAnOpenEventStream(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	member.Stream("/api/inboxes/" + member.InboxID())

	stopped := make(chan error, 1)
	go func() { stopped <- i.server.Stop(context.Background()) }()

	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("stop: %v", err)
		}
	case <-time.After(patience):
		t.Fatal("stop is still waiting for the open event stream")
	}
}
