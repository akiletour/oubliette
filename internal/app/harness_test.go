package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
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

type instance struct {
	t      *testing.T
	Data   string
	Clock  *clock
	URL    string
	server *app.Server
}

func start(t *testing.T) *instance {
	t.Helper()
	t.Parallel()
	i := &instance{
		t:     t,
		Data:  filepath.Join(t.TempDir(), "oubliette.db"),
		Clock: &clock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)},
	}
	i.boot()
	t.Cleanup(i.stop)
	return i
}

func (i *instance) boot() {
	i.t.Helper()
	server, err := app.Start(app.Config{
		DataFile: i.Data,
		HTTPAddr: "127.0.0.1:0",
		Now:      i.Clock.Now,
		Log:      slog.New(slog.DiscardHandler),
	})
	if err != nil {
		i.t.Fatalf("start server: %v", err)
	}
	i.server = server
	i.URL = "http://" + server.HTTPAddr()
}

func (i *instance) stop() {
	i.t.Helper()
	if err := i.server.Stop(context.Background()); err != nil {
		i.t.Errorf("stop server: %v", err)
	}
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
