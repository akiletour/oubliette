package app_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"oubliette/internal/app"
)

const password = "correct horse battery"

func TestHealthAnswersWithoutAuthentication(t *testing.T) {
	i := start(t)

	res := i.Browser().Get("/healthz")

	if res.Status != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", res.Status)
	}
}

func TestInterfaceIsServedFromTheBinary(t *testing.T) {
	i := start(t)
	b := i.Browser()

	page := b.Get("/")
	if page.Status != http.StatusOK || !strings.HasPrefix(page.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("GET / = %d %s, want an HTML page", page.Status, page.Header.Get("Content-Type"))
	}
	if !strings.Contains(page.Body, "<title>Oubliette</title>") {
		t.Fatalf("GET / is not the Oubliette interface: %s", page.Body)
	}

	script := regexp.MustCompile(`src="(/[^"]+\.js)"`).FindStringSubmatch(page.Body)
	if script == nil {
		t.Fatalf("the interface loads no script: %s", page.Body)
	}
	if asset := b.Get(script[1]); asset.Status != http.StatusOK || !strings.Contains(asset.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("GET %s = %d %s, want a script", script[1], asset.Status, asset.Header.Get("Content-Type"))
	}
}

func TestOperatorAddsAndListsMembers(t *testing.T) {
	i := start(t)

	i.AddMember("bob@example.com", "another long password")
	i.AddMember("alice@example.com", "correct horse battery")

	list := i.Member("", "list")
	if list.Code != 0 || list.Stdout != "alice@example.com\nbob@example.com\n" {
		t.Fatalf("member list: exit %d, stdout %q, stderr %q", list.Code, list.Stdout, list.Stderr)
	}
}

func TestOperatorRemovesAMember(t *testing.T) {
	i := start(t)
	i.AddMember("bob@example.com", "another long password")
	i.AddMember("alice@example.com", password)
	b := i.Browser()
	b.SignIn("alice@example.com", password)

	if removed := i.Member("", "remove", "alice@example.com"); removed.Code != 0 {
		t.Fatalf("member remove: exit %d: %s", removed.Code, removed.Stderr)
	}

	if list := i.Member("", "list"); list.Stdout != "bob@example.com\n" {
		t.Fatalf("member list after removal = %q", list.Stdout)
	}
	if session := b.Get("/api/session"); session.Status != http.StatusUnauthorized {
		t.Errorf("session of a removed Member = %d, want 401", session.Status)
	}
	if signedIn := i.Browser().SignIn("alice@example.com", password); signedIn.Status != http.StatusUnauthorized {
		t.Errorf("sign in as a removed Member = %d, want 401", signedIn.Status)
	}
	i.AddMember("carol@example.com", "yet another password")
	if session := b.Get("/api/session"); session.Status != http.StatusUnauthorized {
		t.Errorf("session of a removed Member once another Member is added = %d %s, want 401", session.Status, session.Body)
	}
	if again := i.Member("", "remove", "alice@example.com"); again.Code == 0 {
		t.Fatal("removing an unknown Member succeeded")
	}
}

func TestOperatorCannotAddAnUnusableMember(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", "correct horse battery")

	for name, attempt := range map[string]struct{ email, password string }{
		"duplicate email":   {"Alice@Example.com", "another long password"},
		"invalid email":     {"not-an-email", "another long password"},
		"short password":    {"bob@example.com", "short"},
		"missing password":  {"bob@example.com", ""},
		"oversize password": {"bob@example.com", strings.Repeat("x", 73)},
	} {
		if added := i.Member(attempt.password+"\n", "add", attempt.email); added.Code == 0 {
			t.Errorf("%s: member add succeeded", name)
		}
	}

	if list := i.Member("", "list"); list.Stdout != "alice@example.com\n" {
		t.Fatalf("member list = %q, want only alice@example.com", list.Stdout)
	}
}

func TestMemberSignsInAndOut(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	b := i.Browser()

	signedIn := b.SignIn("Alice@example.com", password)
	if signedIn.Status != http.StatusNoContent {
		t.Fatalf("sign in = %d %s, want 204", signedIn.Status, signedIn.Body)
	}
	if session := b.Get("/api/session"); session.Status != http.StatusOK || !strings.Contains(session.Body, `"alice@example.com"`) {
		t.Fatalf("session after sign in = %d %s, want 200 naming alice@example.com", session.Status, session.Body)
	}

	if signedOut := b.Do(http.MethodDelete, "/api/session", ""); signedOut.Status != http.StatusNoContent {
		t.Fatalf("sign out = %d %s, want 204", signedOut.Status, signedOut.Body)
	}
	if session := b.Get("/api/session"); session.Status != http.StatusUnauthorized {
		t.Fatalf("session after sign out = %d, want 401", session.Status)
	}
	cookie := signedIn.Cookie(t)
	if replayed := i.Browser().Do(http.MethodGet, "/api/session", "", "Cookie", cookie.Name+"="+cookie.Value); replayed.Status != http.StatusUnauthorized {
		t.Fatalf("session replayed after sign out = %d, want 401", replayed.Status)
	}
}

func TestSessionCookieIsProtectedAndSurvivesABrowserRestart(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)

	plain := i.Browser().SignIn("alice@example.com", password).Cookie(t)
	behindHTTPS := i.Browser().SignIn("alice@example.com", password, "X-Forwarded-Proto", "https").Cookie(t)
	behindTwoProxies := i.Browser().SignIn("alice@example.com", password, "X-Forwarded-Proto", "https, http").Cookie(t)

	if !plain.HttpOnly {
		t.Error("session cookie is readable by scripts")
	}
	if plain.SameSite != http.SameSiteLaxMode && plain.SameSite != http.SameSiteStrictMode {
		t.Error("session cookie is sent with cross-site requests")
	}
	if plain.MaxAge <= 0 {
		t.Error("session cookie ends with the browser session")
	}
	if plain.Secure {
		t.Error("session cookie over plain HTTP is marked secure and would never be sent back")
	}
	if !behindHTTPS.Secure || !behindTwoProxies.Secure {
		t.Error("session cookie of a request that came over HTTPS is not marked secure")
	}
}

func TestSessionExpires(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	b := i.Browser()
	b.SignIn("alice@example.com", password)

	i.Clock.Advance(29 * 24 * time.Hour)
	if session := b.Get("/api/session"); session.Status != http.StatusOK {
		t.Fatalf("session after 29 days = %d, want 200", session.Status)
	}

	i.Clock.Advance(2 * 24 * time.Hour)
	if session := b.Get("/api/session"); session.Status != http.StatusUnauthorized {
		t.Fatalf("session after 31 days = %d, want 401", session.Status)
	}
}

func TestWrongEmailAndWrongPasswordAreRefusedAlike(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)

	wrongPassword := i.Browser().SignIn("alice@example.com", "not the password")
	wrongEmail := i.Browser().SignIn("nobody@example.com", password)

	if wrongPassword.Status != http.StatusUnauthorized {
		t.Fatalf("sign in with a wrong password = %d, want 401", wrongPassword.Status)
	}
	if wrongEmail.Status != wrongPassword.Status || wrongEmail.Body != wrongPassword.Body {
		t.Fatalf("wrong email answered %d %q, wrong password answered %d %q",
			wrongEmail.Status, wrongEmail.Body, wrongPassword.Status, wrongPassword.Body)
	}
	if len(wrongEmail.Header.Values("Set-Cookie"))+len(wrongPassword.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("a refused sign in set a cookie")
	}
}

func TestAPIRefusesRequestsWithoutAValidSession(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	cookieName := i.Browser().SignIn("alice@example.com", password).Cookie(t).Name

	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/session"},
		{http.MethodDelete, "/api/session"},
		{http.MethodGet, "/api/inboxes"},
		{http.MethodPost, "/api/inboxes"},
	} {
		if anonymous := i.Browser().Do(request.method, request.path, ""); anonymous.Status != http.StatusUnauthorized {
			t.Errorf("%s %s without a session = %d, want 401", request.method, request.path, anonymous.Status)
		}
		if forged := i.Browser().Do(request.method, request.path, "", "Cookie", cookieName+"=FORGEDFORGEDFORGEDFORGED"); forged.Status != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged session = %d, want 401", request.method, request.path, forged.Status)
		}
	}
}

func TestRepeatedFailuresOnOneAccountAreSlowedDown(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	i.AddMember("bob@example.com", password)
	b := i.Browser()

	for attempt := range 5 {
		if refused := b.SignIn("alice@example.com", "not the password"); refused.Status != http.StatusUnauthorized {
			t.Fatalf("failed sign in %d = %d, want 401", attempt+1, refused.Status)
		}
	}

	if slowed := b.SignIn("alice@example.com", password); slowed.Status != http.StatusTooManyRequests {
		t.Fatalf("sign in after 5 failures = %d, want 429", slowed.Status)
	}
	if slowed := b.SignIn(" ALICE@example.com", password); slowed.Status != http.StatusTooManyRequests {
		t.Fatalf("sign in after 5 failures, email spelled differently = %d, want 429", slowed.Status)
	}
	if other := i.Browser().SignIn("bob@example.com", password); other.Status != http.StatusNoContent {
		t.Fatalf("sign in to another account = %d, want 204", other.Status)
	}

	i.Clock.Advance(15 * time.Minute)
	if later := b.SignIn("alice@example.com", password); later.Status != http.StatusNoContent {
		t.Fatalf("sign in 15 minutes later = %d, want 204", later.Status)
	}
}

func TestSlowingDownDoesNotRevealWhetherAnAccountExists(t *testing.T) {
	i := start(t)
	b := i.Browser()

	for attempt := range 5 {
		if refused := b.SignIn("nobody@example.com", password); refused.Status != http.StatusUnauthorized {
			t.Fatalf("failed sign in %d = %d, want 401", attempt+1, refused.Status)
		}
	}

	if slowed := b.SignIn("nobody@example.com", password); slowed.Status != http.StatusTooManyRequests {
		t.Fatalf("sign in to an unknown account after 5 failures = %d, want 429 as for a known one", slowed.Status)
	}
}

func TestSuccessfulSignInForgetsEarlierFailures(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	b := i.Browser()

	for round := range 2 {
		for range 4 {
			b.SignIn("alice@example.com", "not the password")
		}
		if signedIn := b.SignIn("alice@example.com", password); signedIn.Status != http.StatusNoContent {
			t.Fatalf("round %d: sign in after 4 failures = %d, want 204", round+1, signedIn.Status)
		}
	}
}

func TestCrossSiteStateChangingRequestsAreRefused(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	b := i.Browser()

	if forged := b.SignIn("alice@example.com", password, "Sec-Fetch-Site", "cross-site"); forged.Status != http.StatusForbidden {
		t.Fatalf("cross-site sign in = %d, want 403", forged.Status)
	}
	if forged := b.SignIn("alice@example.com", password, "Origin", "https://evil.example"); forged.Status != http.StatusForbidden {
		t.Fatalf("sign in from another origin = %d, want 403", forged.Status)
	}

	b.SignIn("alice@example.com", password)
	if forged := b.Do(http.MethodDelete, "/api/session", "", "Sec-Fetch-Site", "cross-site"); forged.Status != http.StatusForbidden {
		t.Fatalf("cross-site sign out = %d, want 403", forged.Status)
	}
	if session := b.Get("/api/session"); session.Status != http.StatusOK {
		t.Fatalf("session after a cross-site sign out = %d, want 200", session.Status)
	}
	if own := b.Do(http.MethodDelete, "/api/session", "", "Sec-Fetch-Site", "same-origin"); own.Status != http.StatusNoContent {
		t.Fatalf("same-origin sign out = %d, want 204", own.Status)
	}
}

func TestPasswordIsNeverAcceptedAsAnArgument(t *testing.T) {
	i := start(t)

	if added := i.Member("", "add", "alice@example.com", password); added.Code == 0 {
		t.Fatal("member add accepted a password as an argument")
	}
	if list := i.Member("", "list"); list.Stdout != "" {
		t.Fatalf("member list = %q, want no Member", list.Stdout)
	}

	i.AddMember("alice@example.com", password)
	if set := i.Member("", "set-password", "alice@example.com", "a brand new password"); set.Code == 0 {
		t.Fatal("member set-password accepted a password as an argument")
	}
	if signedIn := i.Browser().SignIn("alice@example.com", "a brand new password"); signedIn.Status != http.StatusUnauthorized {
		t.Fatalf("sign in with the password given as an argument = %d, want 401", signedIn.Status)
	}
}

func TestOperatorSetsAMembersPassword(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	b := i.Browser()
	b.SignIn("alice@example.com", password)

	if set := i.Member("a brand new password\n", "set-password", "alice@example.com"); set.Code != 0 {
		t.Fatalf("member set-password: exit %d: %s", set.Code, set.Stderr)
	}

	if session := b.Get("/api/session"); session.Status != http.StatusUnauthorized {
		t.Errorf("session opened with the old password = %d, want 401", session.Status)
	}
	if old := i.Browser().SignIn("alice@example.com", password); old.Status != http.StatusUnauthorized {
		t.Errorf("sign in with the old password = %d, want 401", old.Status)
	}
	if fresh := i.Browser().SignIn("alice@example.com", "a brand new password"); fresh.Status != http.StatusNoContent {
		t.Errorf("sign in with the new password = %d, want 204", fresh.Status)
	}
	if unknown := i.Member("a brand new password\n", "set-password", "nobody@example.com"); unknown.Code == 0 {
		t.Error("setting the password of an unknown Member succeeded")
	}
}

func TestDataSurvivesARestart(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	b := i.Browser()
	b.SignIn("alice@example.com", password)

	i.Restart()

	if session := b.Get("/api/session"); session.Status != http.StatusOK {
		t.Errorf("session after a restart = %d, want 200", session.Status)
	}
	if signedIn := i.Browser().SignIn("alice@example.com", password); signedIn.Status != http.StatusNoContent {
		t.Errorf("sign in after a restart = %d, want 204", signedIn.Status)
	}
}

func TestSecretsAreNotStoredInClear(t *testing.T) {
	i := start(t)
	i.AddMember("alice@example.com", password)
	token := i.Browser().SignIn("alice@example.com", password).Cookie(t).Value

	files, err := filepath.Glob(i.Data + "*")
	if err != nil {
		t.Fatal(err)
	}
	var data []byte
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, content...)
	}

	if !bytes.Contains(data, []byte("alice@example.com")) {
		t.Fatal("the data file does not hold the Member")
	}
	if bytes.Contains(data, []byte(password)) {
		t.Error("the data file holds the password in clear")
	}
	if bytes.Contains(data, []byte(token)) {
		t.Error("the data file holds the session token in clear")
	}
}

func TestServeCommandRunsUntilInterrupted(t *testing.T) {
	t.Parallel()
	ctx, interrupt := context.WithCancel(context.Background())
	defer interrupt()
	logs, stdout := io.Pipe()
	exit := make(chan int, 1)
	go func() {
		args := []string{"--data", filepath.Join(t.TempDir(), "oubliette.db"), "--http-addr", "127.0.0.1:0", "serve"}
		exit <- app.Main(ctx, args, strings.NewReader(""), stdout, io.Discard, func(string) string { return "" })
		stdout.Close()
	}()

	var addr string
	for lines := bufio.NewScanner(logs); addr == "" && lines.Scan(); {
		addr = regexp.MustCompile(`127\.0\.0\.1:\d+`).FindString(lines.Text())
	}
	go io.Copy(io.Discard, logs)
	if addr == "" {
		t.Fatalf("serve exited with %d before listening", <-exit)
	}
	health, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz = %d, want 200", health.StatusCode)
	}

	interrupt()
	select {
	case code := <-exit:
		if code != 0 {
			t.Errorf("serve exited with %d after an interrupt, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop after an interrupt")
	}
}
