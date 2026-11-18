package app_test

import (
	"fmt"
	"mime"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

const invoice = "From: Shop <shop@example.com>\r\nTo: Customer <customer@example.com>\r\n" +
	"Subject: =?utf-8?q?Votre_facture_n=C2=B01?=\r\nX-Campaign: spring\r\n  sale\r\nMIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=mixed\r\n\r\n" +
	"--mixed\r\nContent-Type: multipart/alternative; boundary=alt\r\n\r\n" +
	"--alt\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nYour invoice is attached.\r\n" +
	"--alt\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Your <b>invoice</b> is attached.</p>\r\n" +
	"--alt--\r\n" +
	"--mixed\r\nContent-Type: application/pdf; name=\"invoice.pdf\"\r\nContent-Disposition: attachment; filename=\"invoice.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQK\r\n" +
	"--mixed\r\nContent-Type: text/html\r\nContent-Disposition: attachment; filename*=utf-8''r%C3%A9sum%C3%A9%22%0D%0A.html\r\n\r\n" +
	"<script>parent.document.title='owned'</script>\r\n" +
	"--mixed--\r\n"

func TestMemberInspectsTheHTMLSourceTheHeadersAndTheEnvelope(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	if err := i.Submit(plain(i.LoggedCredentials()[0]), "bounces@shop.example",
		[]string{"customer@example.com", "audit@agency.example"}, invoice); err != nil {
		t.Fatalf("submit: %v", err)
	}

	var inspected openedMessage
	member.Get(member.MessagePath(member.Messages()[0].ID)).JSON(t, &inspected)
	if inspected.HTML != "<p>Your <b>invoice</b> is attached.</p>" {
		t.Errorf("HTML source = %q, want the HTML part as sent", inspected.HTML)
	}
	want := []field{
		{"From", "Shop <shop@example.com>"},
		{"To", "Customer <customer@example.com>"},
		{"Subject", "Votre facture n°1"},
		{"X-Campaign", "spring sale"},
		{"MIME-Version", "1.0"},
		{"Content-Type", "multipart/mixed; boundary=mixed"},
	}
	if !slices.Equal(inspected.Headers, want) {
		t.Errorf("headers = %q, want %q in the order they were sent", inspected.Headers, want)
	}
	if inspected.EnvelopeSender != "bounces@shop.example" ||
		!slices.Equal(inspected.EnvelopeRecipients, []string{"customer@example.com", "audit@agency.example"}) {
		t.Errorf("envelope = %s → %q, want the SMTP sender and every recipient, the one absent from the headers too",
			inspected.EnvelopeSender, inspected.EnvelopeRecipients)
	}
}

func TestAttachmentsAreListedWithTheirNameTypeAndSizeButNotTheBodiesNorInlineImages(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(invoice)
	i.Clock.Advance(time.Minute)
	i.Capture(related(`<img src="cid:logo@shop.example">`, "iVBORw0KGgo="))
	held := member.Messages()

	var withFiles, withLogo openedMessage
	member.Get(member.MessagePath(held[1].ID)).JSON(t, &withFiles)
	member.Get(member.MessagePath(held[0].ID)).JSON(t, &withLogo)
	want := []attachment{{"invoice.pdf", "application/pdf", 9}, {"résumé\".html", "text/html", 46}}
	if !slices.Equal(withFiles.Attachments, want) {
		t.Errorf("attachments = %+v, want %+v, decoded and without control characters in their names", withFiles.Attachments, want)
	}
	if withFiles.Text != "Your invoice is attached." || !strings.Contains(withFiles.HTML, "<b>invoice</b>") {
		t.Errorf("bodies = %q and %q, want the alternative parts, not the attachments", withFiles.Text, withFiles.HTML)
	}
	if len(withLogo.Attachments) != 0 {
		t.Errorf("attachments of a Message with an inline image = %+v, want none", withLogo.Attachments)
	}
}

type reader struct {
	who     string
	browser *browser
	message string
}

func readersOfTheInvoice(i *instance) []reader {
	member := i.SignedInMember()
	i.Capture(invoice)
	shared := member.InboxNamed("Default")
	held := member.Messages()[0].ID
	link := member.Share(shared.ID, "Reviewer")
	return []reader{
		{"Member", member, "/api/inboxes/" + shared.ID + "/messages/" + held},
		{"Visitor", i.Browser(), "/api/share/" + link.Token + "/messages/" + held},
	}
}

func assertSandboxed(t *testing.T, request string, got response, contentType string) {
	t.Helper()
	for name, value := range map[string]string{"Content-Type": contentType, "X-Content-Type-Options": "nosniff"} {
		if got.Header.Get(name) != value {
			t.Errorf("%s: %s = %q, want %q", request, name, got.Header.Get(name), value)
		}
	}
	if directives(got.Header.Get("Content-Security-Policy"))["sandbox"] == nil {
		t.Errorf("%s: Content-Security-Policy = %q, want a sandbox", request, got.Header.Get("Content-Security-Policy"))
	}
}

func TestAttachmentsAreAlwaysDownloadedByAMemberAndAVisitor(t *testing.T) {
	i := start(t)
	for _, reader := range readersOfTheInvoice(i) {
		for n, want := range []struct{ name, body string }{
			{"invoice.pdf", "%PDF-1.4\n"},
			{"résumé\".html", "<script>parent.document.title='owned'</script>"},
		} {
			request := fmt.Sprintf("%s GET %s/attachments/%d", reader.who, reader.message, n)
			got := reader.browser.Get(fmt.Sprintf("%s/attachments/%d", reader.message, n))
			disposition, params, err := mime.ParseMediaType(got.Header.Get("Content-Disposition"))
			if got.Status != http.StatusOK || got.Body != want.body || err != nil || disposition != "attachment" || params["filename"] != want.name {
				t.Errorf("%s = %d %q with Content-Disposition %q, want %q downloaded as %q",
					request, got.Status, got.Body, got.Header.Get("Content-Disposition"), want.body, want.name)
			}
			assertSandboxed(t, request, got, "application/octet-stream")
		}
		for _, index := range []string{"2", "-1", "first"} {
			if got := reader.browser.Get(reader.message + "/attachments/" + index); got.Status != http.StatusNotFound {
				t.Errorf("%s download of attachment %s = %d %s, want 404", reader.who, index, got.Status, got.Body)
			}
		}
	}
}

func TestRawSourceIsShownAsPlainTextToAMemberAndAVisitor(t *testing.T) {
	i := start(t)
	for _, reader := range readersOfTheInvoice(i) {
		request := reader.who + " GET " + reader.message + "/raw"
		raw := reader.browser.Get(reader.message + "/raw")
		if raw.Status != http.StatusOK || raw.Body != invoice {
			t.Errorf("%s = %d %q, want the source exactly as sent", request, raw.Status, raw.Body)
		}
		assertSandboxed(t, request, raw, "text/plain; charset=utf-8")
	}
}

func TestVisitorReachesNoSourceNorAttachmentOfAnotherInbox(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	other := member.CreateInbox("Secret project")
	if err := i.Submit(plain(other.Credentials()), "app@secret.example", []string{"a@example.com"}, invoice); err != nil {
		t.Fatalf("submit to the other Inbox: %v", err)
	}
	hidden := member.MessagesIn(other.ID)[0].ID
	link := member.Share(member.InboxNamed("Default").ID, "Reviewer")
	visitor := i.Browser()

	for _, view := range []string{"", "/raw", "/attachments/0"} {
		path := "/api/share/" + link.Token + "/messages/" + hidden + view
		if got := visitor.Get(path); got.Status != http.StatusNotFound {
			t.Errorf("Visitor GET %s = %d %s, want 404", path, got.Status, got.Body)
		}
	}
}

func TestImageWithAContentIDMarkedAsAttachmentIsShownInlineAndListed(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(strings.Replace(related(`<img src="cid:logo@shop.example">`, "iVBORw0KGgo="),
		"Content-ID: <logo@shop.example>", "Content-ID: <logo@shop.example>\r\nContent-Disposition: attachment; filename=logo.png", 1))

	var opened openedMessage
	member.Get(member.MessagePath(member.Messages()[0].ID)).JSON(t, &opened)
	if want := []attachment{{"logo.png", "image/png", 8}}; !slices.Equal(opened.Attachments, want) {
		t.Errorf("attachments = %+v, want %+v", opened.Attachments, want)
	}
	if rendered := member.Rendered(); !strings.Contains(rendered, "data:image/png;base64,iVBORw0KGgo=") {
		t.Errorf("rendered = %s, want the image inline", rendered)
	}
}

func TestMessageWithoutAttachmentsListsNone(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(welcome)

	opened := member.Get(member.MessagePath(member.Messages()[0].ID)).Body
	if !strings.Contains(opened, `"attachments":[]`) {
		t.Errorf("opened Message = %s, want an empty list of attachments", opened)
	}
}

func TestImageNotShownInTheHTMLIsListedAsAnAttachment(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(strings.Replace(related(`<p>No image here.</p>`, "iVBORw0KGgo="),
		"Content-Type: image/png", "Content-Type: image/png; name=chart.png", 1))

	var opened openedMessage
	member.Get(member.MessagePath(member.Messages()[0].ID)).JSON(t, &opened)
	if want := []attachment{{"chart.png", "image/png", 8}}; !slices.Equal(opened.Attachments, want) {
		t.Errorf("attachments = %+v, want %+v", opened.Attachments, want)
	}
}

func TestMalformedPartHeadersStillShowTheBodyAndTheAttachments(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture("Subject: Report\r\nX-Note: safe‮gnp.exe\r\nContent-Type: multipart/mixed; boundary=\"b\"; broken\r\n\r\n" +
		"--b\r\nContent-Type: text/plain\r\n\r\nSee the report.\r\n" +
		"--b\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=report final.pdf\r\n\r\n%PDF\r\n" +
		"--b--\r\n")

	var opened openedMessage
	member.Get(member.MessagePath(member.Messages()[0].ID)).JSON(t, &opened)
	if want := []attachment{{"report final.pdf", "application/pdf", 4}}; opened.Text != "See the report." || !slices.Equal(opened.Attachments, want) {
		t.Errorf("text %q and attachments %+v, want the text body and %+v", opened.Text, opened.Attachments, want)
	}
	if want := (field{"X-Note", "safegnp.exe"}); !slices.Contains(opened.Headers, want) {
		t.Errorf("headers = %q, want %q without the direction override", opened.Headers, want)
	}
}
