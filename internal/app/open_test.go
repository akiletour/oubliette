package app_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

type field struct{ Name, Value string }

type attachment struct {
	Name, Type string
	Size       int
}

type openedMessage struct {
	message
	Text        string
	HTML        string
	Headers     []field
	Attachments []attachment
}

const newsletter = "From: Shop <shop@example.com>\r\nTo: customer@example.com\r\nSubject: Spring sale\r\n" +
	"MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=alt\r\n\r\n" +
	"--alt\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nSpring sale: everything at half price.\r\n" +
	"--alt\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
	"<!DOCTYPE html><html><head><title>Sale</title></head><body><h1>Spring sale</h1><p>Everything at half price.</p></body></html>\r\n" +
	"--alt--\r\n"

func TestMemberOpensAMessageAsHTMLOrAsText(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(newsletter)
	path := member.MessagePath(member.Messages()[0].ID)

	var opened openedMessage
	member.Get(path).JSON(t, &opened)
	if opened.Subject != "Spring sale" || opened.Sender != "Shop <shop@example.com>" ||
		opened.Text != "Spring sale: everything at half price." || opened.HTML == "" {
		t.Errorf("opened Message = %+v, want the sale with its text and an HTML version", opened)
	}
	if rendered := member.Get(path + "/rendered"); rendered.Status != http.StatusOK || !strings.Contains(rendered.Body, "<h1>Spring sale</h1>") {
		t.Errorf("rendered Message = %d %s, want the HTML of the sale", rendered.Status, rendered.Body)
	}
}

func TestTextOnlyMessageOpensAsText(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(welcome)
	path := member.MessagePath(member.Messages()[0].ID)

	var opened openedMessage
	member.Get(path).JSON(t, &opened)
	if opened.Text != "Hello." || opened.HTML != "" {
		t.Errorf("opened Message = %+v, want its text and no HTML version", opened)
	}
	if rendered := member.Get(path + "/rendered"); rendered.Status != http.StatusNotFound {
		t.Errorf("rendering a text-only Message = %d %s, want 404", rendered.Status, rendered.Body)
	}
}

func TestOpeningRefusesWhatItCannotResolve(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(newsletter)
	held := member.Messages()[0].ID

	for _, path := range []string{
		member.MessagePath("unknown"),
		"/api/inboxes/unknown/messages/" + held,
	} {
		for _, view := range []string{path, path + "/rendered", path + "/raw", path + "/attachments/0"} {
			if got := member.Get(view); got.Status != http.StatusNotFound {
				t.Errorf("GET %s = %d %s, want 404", view, got.Status, got.Body)
			}
		}
	}
}

func elements(t *testing.T, document, name string) []map[string]string {
	t.Helper()
	root, err := html.ParseWithOptions(strings.NewReader(document), html.ParseOptionEnableScripting(false))
	if err != nil {
		t.Fatalf("parse %s: %v", document, err)
	}
	var found []map[string]string
	for node := range root.Descendants() {
		if node.Type == html.ElementNode && node.Data == name {
			attributes := map[string]string{}
			for _, attribute := range node.Attr {
				attributes[attribute.Key] = attribute.Val
			}
			for child := range node.ChildNodes() {
				if child.Type == html.TextNode {
					attributes["text"] += child.Data
				}
			}
			found = append(found, attributes)
		}
	}
	return found
}

func related(html, image string) string {
	return "Subject: Logo\r\nContent-Type: multipart/related; boundary=rel\r\n\r\n" +
		"--rel\r\nContent-Type: text/html\r\n\r\n" + html + "\r\n" +
		"--rel\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\nContent-ID: <logo@shop.example>\r\n\r\n" +
		image + "\r\n--rel--\r\n"
}

func TestInlineImagesComeFromTheMessageAndRemoteImagesStayRemote(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(related("<style>.hero{background-image:url('cid:logo@shop.example')}</style>"+
		"<p><img src=\"cid:logo@shop.example\" alt=\"Shop\"><img src=\"https://cdn.example.com/banner.png\"></p>"+
		"<table><tr><td style=\"background: url(cid:logo@shop.example)\">Hero</td></tr></table>", "iVBORw0K\r\nGgo="))

	rendered := member.Rendered()
	var sources []string
	for _, image := range elements(t, rendered, "img") {
		sources = append(sources, image["src"])
	}
	inline := "data:image/png;base64,iVBORw0KGgo="
	if want := []string{inline, "https://cdn.example.com/banner.png"}; !slices.Equal(sources, want) {
		t.Errorf("image sources = %q, want %q", sources, want)
	}
	if strings.Count(rendered, inline) != 3 || strings.Contains(rendered, "cid:") {
		t.Errorf("rendered = %s, want the image of the Message in the style sheet and in the style attribute too", rendered)
	}
}

func TestRepeatedInlineImageCannotInflateTheRenderedHTML(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	image := strings.Repeat("AAAA", 10000)
	i.Capture(related(strings.Repeat("<img src=\"cid:logo@shop.example\">", 1000), image))

	if rendered := member.Rendered(); len(rendered) > 1<<20 {
		t.Errorf("rendered a 30 kB image referenced 1000 times into %d bytes, want at most 1 MB", len(rendered))
	}
}

func TestLinksOpenInANewTabWithoutOpenerNorReferrer(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture("Subject: Reset\r\nContent-Type: text/html\r\n\r\n" +
		"<p><a href=\"https://shop.example/reset?token=1\">Reset</a> " +
		"<a href=\"https://shop.example/\" target=\"_top\" rel=\"opener\">Shop</a></p>" +
		"<map><area href=\"https://shop.example/map\"></map>" +
		"<noscript><a href=\"https://shop.example/noscript\" target=\"_top\">Without scripts</a></noscript>" +
		"<svg><style><a href=\"https://shop.example/svg\" target=\"_top\">In a drawing</a></style></svg>\r\n")

	rendered := member.Rendered()
	links := append(elements(t, rendered, "a"), elements(t, rendered, "area")...)
	if len(links) != 5 {
		t.Fatalf("rendered links = %v, want 5", links)
	}
	for _, link := range links {
		rel := strings.Fields(link["rel"])
		if link["target"] != "_blank" || !slices.Contains(rel, "noopener") || !slices.Contains(rel, "noreferrer") || slices.Contains(rel, "opener") {
			t.Errorf("link %v, want it to open in a new tab without opener nor referrer", link)
		}
	}
}

func TestMessageIsShownAsDesignedWhateverTheSystemTheme(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture("Subject: Dark\r\nContent-Type: text/html\r\n\r\n" +
		"<!DOCTYPE html><html><head><meta name=\"color-scheme\" content=\"light dark\">" +
		"<style>:root{color-scheme:light dark} @media (prefers-color-scheme: dark) { body { background: #000 } }" +
		"@media (prefers-color-scheme:/* night */dark) { p { color: #fff } }</style></head>" +
		"<body><picture><source srcset=\"https://cdn.example.com/logo-dark.png\" media=\"(PREFERS-COLOR-SCHEME:DARK)\">" +
		"<img src=\"https://cdn.example.com/logo.png\"></picture><p>Write (prefers-color-scheme: dark) to follow the system.</p><!--\r\n")

	rendered := member.Rendered()
	if !strings.HasPrefix(rendered, "<!DOCTYPE html>") {
		t.Errorf("rendered = %s, want the doctype first so that the Message keeps its rendering mode", rendered)
	}
	var rules []string
	for _, style := range elements(t, rendered, "style") {
		rules = append(rules, style["text"])
	}
	for _, source := range elements(t, rendered, "source") {
		rules = append(rules, source["media"])
	}
	if slices.ContainsFunc(rules, func(rule string) bool { return strings.Contains(strings.ToLower(rule), "prefers-color-scheme") }) {
		t.Errorf("style rules = %q, want none that follows the system theme", rules)
	}
	if !slices.ContainsFunc(rules, func(rule string) bool { return strings.Contains(rule, "color-scheme:only light!important") }) {
		t.Errorf("style rules = %q, want the light scheme forced", rules)
	}
	if !strings.Contains(rendered, "Write (prefers-color-scheme: dark) to follow the system.") {
		t.Errorf("rendered = %s, want the visible text unchanged", rendered)
	}
}

func TestTextAndHTMLAreDecodedFromTheirCharset(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture("Subject: Menu\r\nContent-Type: multipart/alternative; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain; charset=iso-8859-1\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nCaf=E9 cr=E8me\r\n" +
		"--b\r\nContent-Type: text/html; charset=iso-8859-1\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n<p>Caf=E9 cr=E8me</p>\r\n" +
		"--b--\r\n")

	var opened openedMessage
	member.Get(member.MessagePath(member.Messages()[0].ID)).JSON(t, &opened)
	if opened.Text != "Café crème" {
		t.Errorf("text = %q, want %q", opened.Text, "Café crème")
	}
	if rendered := member.Rendered(); !strings.Contains(rendered, "<p>Café crème</p>") {
		t.Errorf("rendered = %s, want the HTML in UTF-8", rendered)
	}
}

func TestDeeplyNestedMessageOpensWithoutDelay(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	var nested strings.Builder
	nested.WriteString("Subject: Nested\r\n")
	for level := range 6000 {
		fmt.Fprintf(&nested, "Content-Type: multipart/mixed; boundary=b%d\r\n\r\n--b%d\r\n", level, level)
	}
	nested.WriteString("Content-Type: text/html\r\n\r\n<p>Deep.</p>\r\n")
	i.Capture(nested.String())
	path := member.MessagePath(member.Messages()[0].ID)

	opened := time.Now()
	if got := member.Get(path); got.Status != http.StatusOK {
		t.Errorf("opening a Message nested 6000 levels deep = %d %s, want 200", got.Status, got.Body)
	}
	member.Get(path + "/rendered")
	if took := time.Since(opened); took > 4*time.Second {
		t.Errorf("opening a deeply nested Message took %s", took)
	}
}

func directives(policy string) map[string][]string {
	parsed := map[string][]string{}
	for directive := range strings.SplitSeq(policy, ";") {
		if fields := strings.Fields(directive); len(fields) > 0 {
			parsed[strings.ToLower(fields[0])] = fields[1:]
		}
	}
	return parsed
}

func TestRenderedHTMLCannotRunScriptsNorRevealTheAddressOfThePage(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	i.Capture(newsletter)
	rendered := member.Get(member.MessagePath(member.Messages()[0].ID) + "/rendered")

	for name, want := range map[string]string{
		"Content-Type":           "text/html; charset=utf-8",
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rendered.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	policy := directives(rendered.Header.Get("Content-Security-Policy"))
	if sandbox, sandboxed := policy["sandbox"]; !sandboxed || slices.Contains(sandbox, "allow-scripts") ||
		slices.Contains(sandbox, "allow-same-origin") || !slices.Contains(sandbox, "allow-popups") {
		t.Errorf("sandbox = %q, want a sandbox that opens links in a new tab, without scripts and without the application's origin", sandbox)
	}
	if !slices.Equal(policy["default-src"], []string{"'none'"}) || policy["script-src"] != nil {
		t.Errorf("default-src = %q and script-src = %q, want every script forbidden", policy["default-src"], policy["script-src"])
	}
	if images := policy["img-src"]; !slices.Contains(images, "*") || !slices.Contains(images, "data:") {
		t.Errorf("img-src = %q, want remote and inline images allowed", images)
	}
}
