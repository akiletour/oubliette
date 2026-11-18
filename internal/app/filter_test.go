package app_test

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func (b *browser) Filtered(place, query, before string) messageList {
	b.i.t.Helper()
	var page messageList
	b.Get(place+"/messages?q="+url.QueryEscape(query)+"&before="+url.QueryEscape(before)).JSON(b.i.t, &page)
	return page
}

func subjects(messages []message) []string {
	var listed []string
	for _, held := range messages {
		listed = append(listed, held.Subject)
	}
	return listed
}

func TestFilterMatchesSubjectSenderAndRecipientsWhateverTheCase(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	for _, data := range []string{
		"From: Shop <shop@example.com>\r\nTo: anna@example.com\r\nSubject: Votre Commande ÉTÉ\r\n\r\nHello.\r\n",
		"From: Billing <billing@example.com>\r\nTo: bob@example.com\r\nSubject: Invoice\r\n\r\nHello.\r\n",
		"From: news@example.com\r\nTo: carl@example.com\r\nCc: Zoé Martin <zoe@example.com>\r\nSubject: Weekly\r\n\r\nShop news.\r\n",
		"From: app@example.com\r\nTo: dan@example.com\r\nSubject: 100% off_now\r\n\r\nHello.\r\n",
	} {
		i.Capture(data)
		i.Clock.Advance(time.Second)
	}
	inbox := "/api/inboxes/" + member.InboxID()

	for query, want := range map[string][]string{
		"commande été": {"Votre Commande ÉTÉ"},
		"BILLING@":     {"Invoice"},
		"zoé martin":   {"Weekly"},
		"shop":         {"Votre Commande ÉTÉ"},
		"%":            {"100% off_now"},
		"f_n":          {"100% off_now"},
		"o%o":          nil,
		"":             {"100% off_now", "Weekly", "Invoice", "Votre Commande ÉTÉ"},
	} {
		if got := subjects(member.Filtered(inbox, query, "").Messages); !slices.Equal(got, want) {
			t.Errorf("filter %q lists %q, want %q", query, got, want)
		}
	}
}

func TestFilterAndPaginationWorkTogether(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	for n := 1; n <= 52; n++ {
		topic := "Receipt"
		if n%2 == 0 {
			topic = "Reminder"
		}
		i.Capture(fmt.Sprintf("From: app@example.com\r\nSubject: %s %d\r\n\r\nHello.\r\n", topic, n))
		i.Capture(fmt.Sprintf("From: app@example.com\r\nSubject: Receipt extra %d\r\n\r\nHello.\r\n", n))
	}
	inbox := "/api/inboxes/" + member.InboxID()

	first := member.Filtered(inbox, "receipt", "")
	second := member.Filtered(inbox, "receipt", first.Next)

	if len(first.Messages) != 50 || first.Next == "" || len(second.Messages) != 28 || second.Next != "" {
		t.Fatalf("filtered pages hold %d then %d Messages (next %q then %q), want 50 with a next page, then 28 without",
			len(first.Messages), len(second.Messages), first.Next, second.Next)
	}
	for _, listed := range append(first.Messages, second.Messages...) {
		if !strings.HasPrefix(listed.Subject, "Receipt") {
			t.Errorf("filter receipt lists %q", listed.Subject)
		}
	}
}

func TestVisitorFiltersTheSharedInbox(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture("From: app@example.com\r\nSubject: Welcome\r\n\r\nHello.\r\n")
	i.Capture("From: app@example.com\r\nSubject: Goodbye\r\n\r\nHello.\r\n")
	link := member.Share(member.InboxID(), "Client")

	got := subjects(i.Browser().Filtered("/api/share/"+link.Token, "WELC", "").Messages)

	if !slices.Equal(got, []string{"Welcome"}) {
		t.Errorf("a Visitor filtering WELC sees %q, want only Welcome", got)
	}
}
