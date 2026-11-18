package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/smtp"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"oubliette/internal/app"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type logs struct {
	mu      sync.Mutex
	written bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.written.Write(p)
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.written.String()
}

type instance struct {
	t         *testing.T
	Data      string
	Clock     *clock
	URL       string
	SMTPAddr  string
	logs      *logs
	server    *app.Server
	configure func(*app.Config)
}

func start(t *testing.T) *instance {
	t.Helper()
	return startWith(t, func(*app.Config) {})
}

func startWith(t *testing.T, configure func(*app.Config)) *instance {
	t.Helper()
	t.Parallel()
	i := &instance{
		t:         t,
		Data:      filepath.Join(t.TempDir(), "oubliette.db"),
		Clock:     &clock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
		logs:      &logs{},
		configure: configure,
	}
	i.boot()
	t.Cleanup(i.stop)
	return i
}

func (i *instance) boot() {
	i.t.Helper()
	cfg := app.Config{
		DataFile:     i.Data,
		HTTPAddr:     "127.0.0.1:0",
		SMTPAddr:     "127.0.0.1:0",
		InsecureAuth: true,
		Now:          i.Clock.Now,
		Log:          slog.New(slog.NewJSONHandler(i.logs, nil)),
	}
	i.configure(&cfg)
	server, err := app.Start(cfg)
	if err != nil {
		i.t.Fatalf("start server: %v", err)
	}
	i.server = server
	i.URL = "http://" + server.HTTPAddr()
	i.SMTPAddr = server.SMTPAddr()
}

type credentials struct {
	Username string `json:"smtp_username"`
	Password string `json:"smtp_password"`
}

func (i *instance) LoggedCredentials() []credentials {
	i.t.Helper()
	var logged []credentials
	for line := range strings.Lines(i.logs.String()) {
		var entry credentials
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			i.t.Fatalf("log line %q: %v", line, err)
		}
		if entry.Username != "" {
			logged = append(logged, entry)
		}
	}
	return logged
}

func plain(c credentials) smtp.Auth {
	return smtp.PlainAuth("", c.Username, c.Password, "127.0.0.1")
}

type login credentials

func (l login) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (l login) Next(challenge []byte, more bool) ([]byte, error) {
	switch {
	case !more:
		return nil, nil
	case strings.HasPrefix(strings.ToLower(string(challenge)), "username"):
		return []byte(l.Username), nil
	default:
		return []byte(l.Password), nil
	}
}

func (i *instance) Connect(auth smtp.Auth) (*smtp.Client, error) {
	i.t.Helper()
	client, err := smtp.Dial(i.SMTPAddr)
	if err != nil {
		i.t.Fatalf("connect to SMTP: %v", err)
	}
	i.t.Cleanup(func() { client.Close() })
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func transmit(client *smtp.Client, from string, to []string, data string) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	body, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(body, data); err != nil {
		return err
	}
	return body.Close()
}

func (i *instance) Submit(auth smtp.Auth, from string, to []string, data string) error {
	i.t.Helper()
	client, err := i.Connect(auth)
	if err != nil {
		return err
	}
	if err := transmit(client, from, to, data); err != nil {
		return err
	}
	return client.Quit()
}

type message struct {
	ID                 string
	ReceivedAt         time.Time
	Subject            string
	Preview            string
	Sender             string
	Recipients         []string
	EnvelopeSender     string
	EnvelopeRecipients []string
}

type messageList struct {
	Messages []message
	Next     string
}

func (i *instance) Capture(data string) {
	i.t.Helper()
	if err := i.Submit(plain(i.LoggedCredentials()[0]), "app@staging.example", []string{"a@example.com"}, data); err != nil {
		i.t.Fatalf("submit: %v", err)
	}
}

func (b *browser) InboxID() string {
	t := b.i.t
	t.Helper()
	inboxes := b.Inboxes()
	if len(inboxes) != 1 {
		t.Fatalf("got %d Inboxes, want 1", len(inboxes))
	}
	return inboxes[0].ID
}

func (b *browser) PageOf(inboxID, before string) messageList {
	b.i.t.Helper()
	var page messageList
	b.Get("/api/inboxes/"+inboxID+"/messages?before="+before).JSON(b.i.t, &page)
	return page
}

func (b *browser) MessagePage(before string) messageList {
	b.i.t.Helper()
	return b.PageOf(b.InboxID(), before)
}

func (b *browser) MessagePath(id string) string {
	b.i.t.Helper()
	return "/api/inboxes/" + b.InboxID() + "/messages/" + id
}

type createdInbox struct {
	ID           string
	Name         string
	SMTPUsername string `json:"smtpUsername"`
	SMTPPassword string `json:"smtpPassword"`
}

func (c createdInbox) Credentials() credentials {
	return credentials{c.SMTPUsername, c.SMTPPassword}
}

func named(name string) string {
	body, _ := json.Marshal(map[string]string{"name": name})
	return string(body)
}

func (b *browser) CreateInbox(name string) createdInbox {
	t := b.i.t
	t.Helper()
	created := b.Do(http.MethodPost, "/api/inboxes", named(name))
	if created.Status != http.StatusCreated {
		t.Fatalf("create Inbox %q = %d %s, want 201", name, created.Status, created.Body)
	}
	var inbox createdInbox
	if err := json.Unmarshal([]byte(created.Body), &inbox); err != nil {
		t.Fatalf("created Inbox %s: %v", created.Body, err)
	}
	return inbox
}

func (b *browser) Inboxes() []listedInbox {
	b.i.t.Helper()
	var inboxes inboxList
	b.Get("/api/inboxes").JSON(b.i.t, &inboxes)
	return inboxes.Inboxes
}

func (b *browser) MessagesIn(inboxID string) []message {
	b.i.t.Helper()
	return b.PageOf(inboxID, "").Messages
}

type shareLink struct {
	ID           string
	Label        string
	Token        string
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	LastOpenedAt *time.Time
	Expired      bool
}

func labelled(label string) string {
	body, _ := json.Marshal(map[string]string{"label": label})
	return string(body)
}

func expiring(label string, at time.Time) string {
	body, _ := json.Marshal(map[string]any{"label": label, "expiresAt": at})
	return string(body)
}

func (b *browser) Share(inboxID, label string) shareLink {
	b.i.t.Helper()
	return b.ShareWith(inboxID, labelled(label))
}

func expiredLabels(links []shareLink) []string {
	var expired []string
	for _, link := range links {
		if link.Expired {
			expired = append(expired, link.Label)
		}
	}
	return expired
}

func (b *browser) Opens(link shareLink) bool {
	b.i.t.Helper()
	return b.Get("/api/share/"+link.Token).Status == http.StatusOK
}

func (b *browser) ShareLinks(inboxID string) []shareLink {
	b.i.t.Helper()
	var listed struct{ ShareLinks []shareLink }
	b.Get("/api/inboxes/"+inboxID+"/share-links").JSON(b.i.t, &listed)
	return listed.ShareLinks
}

func (b *browser) ShareWith(inboxID, body string) shareLink {
	t := b.i.t
	t.Helper()
	created := b.Do(http.MethodPost, "/api/inboxes/"+inboxID+"/share-links", body)
	if created.Status != http.StatusCreated {
		t.Fatalf("share Inbox %s = %d %s, want 201", inboxID, created.Status, created.Body)
	}
	var link shareLink
	if err := json.Unmarshal([]byte(created.Body), &link); err != nil {
		t.Fatalf("created Share Link %s: %v", created.Body, err)
	}
	return link
}

func (b *browser) InboxNamed(name string) listedInbox {
	b.i.t.Helper()
	for _, inbox := range b.Inboxes() {
		if inbox.Name == name {
			return inbox
		}
	}
	b.i.t.Fatalf("no Inbox named %q", name)
	return listedInbox{}
}

func names(inboxes []listedInbox) []string {
	var listed []string
	for _, inbox := range inboxes {
		listed = append(listed, inbox.Name)
	}
	return listed
}

func (b *browser) Rendered() string {
	t := b.i.t
	t.Helper()
	rendered := b.Get(b.MessagePath(b.Messages()[0].ID) + "/rendered")
	if rendered.Status != http.StatusOK {
		t.Fatalf("rendered Message = %d %s, want 200", rendered.Status, rendered.Body)
	}
	return rendered.Body
}

func (b *browser) Messages() []message {
	b.i.t.Helper()
	return b.MessagePage("").Messages
}

func (i *instance) SignedInMember() *browser {
	i.t.Helper()
	i.AddMember("member@example.com", password)
	b := i.Browser()
	if signedIn := b.SignIn("member@example.com", password); signedIn.Status != http.StatusNoContent {
		i.t.Fatalf("sign in = %d %s, want 204", signedIn.Status, signedIn.Body)
	}
	return b
}

func (i *instance) stop() {
	i.t.Helper()
	if err := <-i.StopInBackground(); err != nil {
		i.t.Errorf("stop server: %v", err)
	}
}

func (i *instance) StopInBackground() <-chan error {
	server, stopped := i.server, make(chan error, 1)
	i.server = nil
	if server == nil {
		stopped <- nil
		return stopped
	}
	go func() { stopped <- server.Stop(context.Background()) }()
	return stopped
}

func (i *instance) Restart() {
	i.t.Helper()
	i.stop()
	i.boot()
}

type run struct {
	Code   int
	Stdout string
	Stderr string
}

func (i *instance) Member(stdin string, args ...string) run {
	i.t.Helper()
	var stdout, stderr bytes.Buffer
	env := map[string]string{"OUBLIETTE_DATA": i.Data}
	args = append([]string{"member"}, args...)
	code := app.Main(context.Background(), args, strings.NewReader(stdin), &stdout, &stderr, func(key string) string { return env[key] })
	return run{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

func (i *instance) AddMember(email, password string) {
	i.t.Helper()
	if added := i.Member(password+"\n", "add", email); added.Code != 0 {
		i.t.Fatalf("member add %s: exit %d: %s", email, added.Code, added.Stderr)
	}
}

type browser struct {
	i      *instance
	client *http.Client
}

type response struct {
	Status int
	Header http.Header
	Body   string
}

func (i *instance) Browser() *browser {
	i.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		i.t.Fatalf("cookie jar: %v", err)
	}
	return &browser{i: i, client: &http.Client{Jar: jar}}
}

func (b *browser) Do(method, path, body string, headers ...string) response {
	t := b.i.t
	t.Helper()
	req, err := http.NewRequest(method, b.i.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	if len(headers)%2 != 0 {
		t.Fatalf("%s %s: headers must be name and value pairs, got %q", method, path, headers)
	}
	for n := 0; n < len(headers); n += 2 {
		req.Header.Set(headers[n], headers[n+1])
	}
	res, err := b.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s %s: read body: %v", method, path, err)
	}
	return response{Status: res.StatusCode, Header: res.Header, Body: string(data)}
}

func (b *browser) SignIn(email, password string, headers ...string) response {
	b.i.t.Helper()
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		b.i.t.Fatalf("sign in: %v", err)
	}
	return b.Do(http.MethodPost, "/api/session", string(body), headers...)
}

func (r response) JSON(t *testing.T, into any) {
	t.Helper()
	if r.Status != http.StatusOK {
		t.Fatalf("response = %d %s, want 200", r.Status, r.Body)
	}
	if err := json.Unmarshal([]byte(r.Body), into); err != nil {
		t.Fatalf("response %s: %v", r.Body, err)
	}
}

func (r response) Cookie(t *testing.T) *http.Cookie {
	t.Helper()
	cookies := (&http.Response{Header: r.Header}).Cookies()
	if len(cookies) != 1 {
		t.Fatalf("response set %d cookies, want 1", len(cookies))
	}
	return cookies[0]
}

func (b *browser) Get(path string) response {
	b.i.t.Helper()
	return b.Do(http.MethodGet, path, "")
}
