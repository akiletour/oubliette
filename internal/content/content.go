package content

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/jhillyerd/enmime/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

const (
	previewSource = 256 << 10
	previewLength = 200
)

type limits struct {
	depth, parts int
	text         int64
}

var (
	openLimits    = limits{depth: 8, parts: 200, text: math.MaxInt64}
	previewLimits = limits{depth: 3, parts: 20, text: 4096}
)

type Body struct {
	Text   string
	HTML   string
	Images map[string]Image
}

type Image struct {
	Type string
	Data []byte
}

type unreported struct{}

func (unreported) AddError(string, string, ...any)   {}
func (unreported) AddWarning(string, string, ...any) {}

func Split(raw []byte) (textproto.MIMEHeader, io.Reader, error) {
	body := bufio.NewReader(bytes.NewReader(raw))
	header, err := enmime.ReadHeader(body, unreported{})
	return header, body, err
}

func Read(raw []byte) Body {
	header, body, _ := Split(raw)
	return read(header, body, openLimits)
}

func Preview(header textproto.MIMEHeader, body io.Reader) string {
	found := read(header, io.LimitReader(body, previewSource), previewLimits)
	words := strings.Fields(found.Text)
	if len(words) == 0 {
		words = strings.Fields(visibleText(found.HTML))
	}
	beginning := []rune(strings.Join(words, " "))
	return string(beginning[:min(len(beginning), previewLength)])
}

func read(header textproto.MIMEHeader, body io.Reader, bounds limits) Body {
	found := Body{Images: map[string]Image{}}
	found.walk(header, body, bounds, bounds.depth)
	return found
}

func (b *Body) walk(header textproto.MIMEHeader, body io.Reader, bounds limits, depth int) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		mediaType = "text/plain"
	}
	id := strings.Trim(header.Get("Content-ID"), "<> ")
	switch {
	case strings.HasPrefix(mediaType, "image/") && id != "":
		data, _ := io.ReadAll(decoded(header, body))
		b.Images[id] = Image{Type: mediaType, Data: data}
	case strings.HasPrefix(strings.ToLower(header.Get("Content-Disposition")), "attachment"):
	case strings.HasPrefix(mediaType, "multipart/"):
		if depth == 0 {
			return
		}
		parts := multipart.NewReader(body, params["boundary"])
		for range bounds.parts {
			part, err := parts.NextRawPart()
			if err != nil {
				return
			}
			b.walk(part.Header, part, bounds, depth-1)
		}
	case mediaType == "text/plain" && b.Text == "":
		b.Text = strings.TrimSpace(text(header, body, bounds.text))
	case mediaType == "text/html" && b.HTML == "":
		b.HTML = text(header, body, bounds.text)
	}
}

func decoded(header textproto.MIMEHeader, body io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		return quotedprintable.NewReader(body)
	}
	return body
}

func text(header textproto.MIMEHeader, body io.Reader, limit int64) string {
	source := io.LimitReader(decoded(header, body), limit)
	converted, err := charset.NewReader(source, header.Get("Content-Type"))
	if err != nil {
		converted = source
	}
	text, _ := io.ReadAll(converted)
	return strings.ToValidUTF8(string(text), "")
}

var (
	invisible = map[string]bool{"head": true, "title": true, "style": true, "script": true, "noscript": true, "template": true, "svg": true}
	inline    = map[string]bool{
		"a": true, "abbr": true, "b": true, "code": true, "em": true, "font": true, "i": true,
		"small": true, "span": true, "strong": true, "sub": true, "sup": true, "u": true,
	}
)

func visibleText(document string) string {
	var text strings.Builder
	hidden := 0
	tokens := html.NewTokenizer(strings.NewReader(document))
	for int64(text.Len()) < previewLimits.text {
		token := tokens.Next()
		if token == html.ErrorToken {
			break
		}
		if token == html.TextToken {
			if hidden == 0 {
				text.Write(tokens.Text())
			}
			continue
		}
		name, _ := tokens.TagName()
		switch tag := string(name); {
		case invisible[tag] && token == html.StartTagToken:
			hidden++
		case invisible[tag] && token == html.EndTagToken && hidden > 0:
			hidden--
		case !inline[tag]:
			text.WriteByte(' ')
		}
	}
	return text.String()
}

const (
	cssGap          = `(?:\s|/\*.*?\*/)*`
	inlineAllowance = 4
)

var (
	systemTheme  = regexp.MustCompile(`(?is)\(` + cssGap + `prefers-color-scheme` + cssGap + `:` + cssGap + `(dark|light)` + cssGap + `\)`)
	cssReference = regexp.MustCompile(`(?i)url\(\s*['"]?(cid:[^'")\s]+)['"]?\s*\)`)
)

func asDesigned(rules string) string {
	return systemTheme.ReplaceAllStringFunc(rules, func(query string) string {
		if strings.EqualFold(systemTheme.FindStringSubmatch(query)[1], "dark") {
			return "(max-width:0)"
		}
		return "(min-width:0)"
	})
}

type page struct {
	images  map[string]Image
	encoded map[string]string
	budget  int
}

func (b Body) Rendered() string {
	document, err := html.ParseWithOptions(strings.NewReader(b.HTML), html.ParseOptionEnableScripting(false))
	if err != nil {
		return ""
	}
	p := page{images: b.Images, encoded: map[string]string{}}
	for _, image := range b.Images {
		p.budget += inlineAllowance * (len("data:;base64,") + len(image.Type) + base64.StdEncoding.EncodedLen(len(image.Data)))
	}
	var body *html.Node
	for node := range document.Descendants() {
		switch {
		case node.Type == html.ElementNode:
			p.rewrite(node)
			if node.DataAtom == atom.Body || body == nil && node.DataAtom == atom.Html {
				body = node
			}
		case node.Type == html.TextNode && node.Parent.DataAtom == atom.Style:
			node.Data = asDesigned(p.css(node.Data))
		}
	}
	scheme := &html.Node{Type: html.ElementNode, Data: "style", DataAtom: atom.Style}
	scheme.AppendChild(&html.Node{Type: html.TextNode, Data: ":root{color-scheme:only light!important}"})
	body.AppendChild(scheme)
	var rendered strings.Builder
	html.Render(&rendered, document)
	return rendered.String()
}

func (p *page) rewrite(element *html.Node) {
	for n, attribute := range element.Attr {
		switch attribute.Key {
		case "style":
			element.Attr[n].Val = p.css(attribute.Val)
		case "media":
			element.Attr[n].Val = asDesigned(attribute.Val)
		default:
			if uri, found := p.inline(attribute.Val); found {
				element.Attr[n].Val = uri
			}
		}
	}
	if element.Data == "a" || element.Data == "area" {
		element.Attr = append(slices.DeleteFunc(element.Attr, func(attribute html.Attribute) bool {
			return attribute.Key == "target" || attribute.Key == "rel"
		}), html.Attribute{Key: "target", Val: "_blank"}, html.Attribute{Key: "rel", Val: "noopener noreferrer"})
	}
}

func (p *page) css(rules string) string {
	return cssReference.ReplaceAllStringFunc(rules, func(reference string) string {
		if uri, found := p.inline(cssReference.FindStringSubmatch(reference)[1]); found {
			return `url("` + uri + `")`
		}
		return reference
	})
}

func (p *page) inline(reference string) (string, bool) {
	if len(reference) < 4 || !strings.EqualFold(reference[:4], "cid:") {
		return "", false
	}
	id, err := url.PathUnescape(reference[4:])
	if err != nil {
		id = reference[4:]
	}
	image, found := p.images[id]
	if !found {
		return "", false
	}
	uri, encoded := p.encoded[id]
	if !encoded {
		uri = "data:" + image.Type + ";base64," + base64.StdEncoding.EncodeToString(image.Data)
		p.encoded[id] = uri
	}
	if len(uri) > p.budget {
		return "", false
	}
	p.budget -= len(uri)
	return uri, true
}
