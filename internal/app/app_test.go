package app_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

func TestEveryInboxURLServesTheInterface(t *testing.T) {
	i := start(t)
	b := i.Browser()

	page := b.Get("/inboxes/any-inbox")
	if page.Status != http.StatusOK || !strings.Contains(page.Body, "<title>Oubliette</title>") {
		t.Fatalf("GET /inboxes/any-inbox = %d %s, want the interface", page.Status, page.Body)
	}
	if elsewhere := b.Get("/no-such-page"); elsewhere.Status != http.StatusNotFound {
		t.Fatalf("GET /no-such-page = %d, want 404", elsewhere.Status)
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
		{http.MethodGet, "/api/inboxes/any/messages"},
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

type inboxList struct {
	Inboxes []struct {
		ID           string
		Name         string
		MessageCount int
	}
}

func TestFirstStartCreatesOneInboxAndLogsItsCredentials(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()

	logged := i.LoggedCredentials()
	if len(logged) != 1 || logged[0].Username == "" || logged[0].Password == "" {
		t.Fatalf("first start logged %d Credentials (%+v), want one complete pair", len(logged), logged)
	}
	var first inboxList
	member.Get("/api/inboxes").JSON(t, &first)
	if len(first.Inboxes) != 1 || first.Inboxes[0].ID == "" || first.Inboxes[0].Name == "" {
		t.Fatalf("Inboxes after the first start = %+v, want one named Inbox", first.Inboxes)
	}

	i.Restart()

	if again := i.LoggedCredentials(); len(again) != 1 {
		t.Errorf("after a restart %d Credentials were logged in total, want 1", len(again))
	}
	var later inboxList
	member.Get("/api/inboxes").JSON(t, &later)
	if len(later.Inboxes) != 1 || later.Inboxes[0].ID != first.Inboxes[0].ID {
		t.Errorf("Inboxes after a restart = %+v, want the same single Inbox %+v", later.Inboxes, first.Inboxes)
	}
}

const welcome = "From: Shop <shop@example.com>\r\nTo: Customer <customer@example.com>\r\nSubject: Welcome\r\n\r\nHello.\r\n"

func TestApplicationSubmitsAMessageTheMemberSeesInTheInbox(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	application := i.LoggedCredentials()[0]

	if err := i.Submit(plain(application), "bounces@staging.example", []string{"customer@example.com"}, welcome); err != nil {
		t.Fatalf("submit with PLAIN: %v", err)
	}
	if err := i.Submit(login(application), "bounces@staging.example", []string{"customer@example.com"}, welcome); err != nil {
		t.Fatalf("submit with LOGIN: %v", err)
	}

	held := member.Messages()
	if len(held) != 2 {
		t.Fatalf("the Inbox holds %d Messages, want 2", len(held))
	}
	for _, got := range held {
		if got.Subject != "Welcome" || got.Sender != "Shop <shop@example.com>" ||
			!slices.Equal(got.Recipients, []string{"Customer <customer@example.com>"}) || !got.ReceivedAt.Equal(i.Clock.Now()) {
			t.Errorf("held Message = %+v, want Welcome from Shop to Customer received at %s", got, i.Clock.Now())
		}
	}
}

func TestSubmissionWithoutValidCredentialsIsRefusedAndNothingIsHeld(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	application := i.LoggedCredentials()[0]

	for name, attempt := range map[string]struct {
		auth smtp.Auth
		code int
	}{
		"no Credentials":            {nil, 530},
		"wrong password":            {plain(credentials{application.Username, "not the password"}), 535},
		"empty password":            {plain(credentials{application.Username, ""}), 535},
		"unknown username":          {plain(credentials{"nobody", application.Password}), 535},
		"wrong password with LOGIN": {login{application.Username, "not the password"}, 535},
	} {
		err := i.Submit(attempt.auth, "bounces@staging.example", []string{"customer@example.com"}, welcome)
		var refusal *textproto.Error
		if !errors.As(err, &refusal) || refusal.Code != attempt.code || refusal.Msg == "" {
			t.Errorf("%s: submission answered %v, want SMTP error %d with an explanation", name, err, attempt.code)
		}
	}

	if held := member.Messages(); len(held) != 0 {
		t.Fatalf("the Inbox holds %d Messages after refused submissions, want none", len(held))
	}
}

func TestEnvelopeIsKeptWhateverTheAddresses(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	sender := "noreply@unregistered-domain.invalid"
	recipients := []string{"customer@example.com", "hidden-copy@elsewhere.example", "nobody@nowhere.invalid"}

	if err := i.Submit(plain(i.LoggedCredentials()[0]), sender, recipients, welcome); err != nil {
		t.Fatalf("submit: %v", err)
	}

	held := member.Messages()
	if len(held) != 1 {
		t.Fatalf("a Message sent to %d recipients is held %d times, want once", len(recipients), len(held))
	}
	if held[0].EnvelopeSender != sender || !slices.Equal(held[0].EnvelopeRecipients, recipients) {
		t.Errorf("envelope = %q to %q, want %q to %q", held[0].EnvelopeSender, held[0].EnvelopeRecipients, sender, recipients)
	}
	if !slices.Equal(held[0].Recipients, []string{"Customer <customer@example.com>"}) {
		t.Errorf("displayed recipients = %q, want only the one named in the headers", held[0].Recipients)
	}
}

func TestListFieldsAreReadFromTheHeadersWhenThereAreSome(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	application := plain(i.LoggedCredentials()[0])
	encoded := "From: =?UTF-8?Q?J=C3=A9r=C3=B4me?= <jerome@example.com>\r\n" +
		"To: =?UTF-8?Q?Dupont=2C_Jean?= <a@example.com>\r\nCc: Bob <b@example.com>\r\n" +
		"Subject: =?UTF-8?B?UsOpaW5pdGlhbGlzYXRpb24=?=\r\n\r\nBonjour.\r\n"
	disguised := "From: =?UTF-8?Q?Bank_=3Cceo=40bank.example=3E?= <someone@elsewhere.example>\r\nSubject: Disguised\r\n\r\nHello.\r\n"

	for name, data := range map[string]string{"encoded": encoded, "disguised": disguised, "headerless": "No header at all, only text.\r\n"} {
		if err := i.Submit(application, "app@staging.example", []string{"a@example.com"}, data); err != nil {
			t.Fatalf("submit the %s Message: %v", name, err)
		}
	}

	listed := map[string]message{}
	for _, held := range member.Messages() {
		listed[held.Subject] = held
	}
	if len(listed) != 3 {
		t.Fatalf("the Inbox lists %d distinct subjects, want 3: %+v", len(listed), listed)
	}
	if decoded := listed["Réinitialisation"]; decoded.Sender != "Jérôme <jerome@example.com>" ||
		!slices.Equal(decoded.Recipients, []string{"Dupont, Jean <a@example.com>", "Bob <b@example.com>"}) {
		t.Errorf("encoded Message listed as %+v", decoded)
	}
	if sender := listed["Disguised"].Sender; !strings.HasSuffix(sender, "<someone@elsewhere.example>") || strings.Contains(sender, ",") {
		t.Errorf("a name that looks like an address is listed as %q, want one sender ending with the real address", sender)
	}
	if bare := listed[""]; bare.Sender != "app@staging.example" || !slices.Equal(bare.Recipients, []string{"a@example.com"}) {
		t.Errorf("Message without headers listed as %+v, want the envelope addresses", bare)
	}
}

func TestListShowsTheBeginningOfTheTextAndCountsTheMessages(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	application := plain(i.LoggedCredentials()[0])
	both := "Subject: Both\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nBonjour Camille,\r\n\r\n  votre commande   est partie.\r\n" +
		"--b\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Bonjour <b>Camille</b></p>\r\n--b--\r\n"
	htmlOnly := "Subject: HTML only\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<html><head><style>p{color:red}</style></head><body><p>Votre facture est disponible.</p></body></html>\r\n"
	long := "Subject: Long\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + strings.Repeat("été ", 400) + "\r\n"
	decorated := "Subject: Decorated\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<h1>Big title</h1><p>Hello, <a href=\"https://example.com/reset?token=abc\">reset your password</a>.</p>" +
		"<noscript>Enable scripts</noscript><svg><text>logo</text></svg><script>track()</script>\r\n"
	blankText := "Subject: Blank text\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n   \r\n" +
		"--b\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Only the HTML has words.</p>\r\n--b--\r\n"

	encoded := "Subject: Base64\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString([]byte("Bonjour, ceci est encodé.")) + "\r\n"
	latin := "Subject: Latin\r\nContent-Type: text/plain; charset=iso-8859-1\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"Votre commande est pr=EAte.\r\n"
	attached := "Subject: Attached\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=notes.txt\r\n\r\nNot the body.\r\n" +
		"--b\r\nContent-Type: text/plain\r\n\r\nThe body.\r\n--b--\r\n"

	for name, data := range map[string]string{
		"both": both, "HTML only": htmlOnly, "long": long, "decorated": decorated, "blank text": blankText,
		"base64": encoded, "latin": latin, "attached": attached,
	} {
		if err := i.Submit(application, "app@staging.example", []string{"a@example.com"}, data); err != nil {
			t.Fatalf("submit the %s Message: %v", name, err)
		}
	}

	previews := map[string]string{}
	for _, held := range member.Messages() {
		previews[held.Subject] = held.Preview
	}
	if previews["Both"] != "Bonjour Camille, votre commande est partie." {
		t.Errorf("preview of a Message with text and HTML = %q, want its text on one line", previews["Both"])
	}
	if previews["HTML only"] != "Votre facture est disponible." {
		t.Errorf("preview of an HTML-only Message = %q, want its visible text", previews["HTML only"])
	}
	if long := previews["Long"]; !utf8.ValidString(long) || utf8.RuneCountInString(long) != 200 || !strings.HasPrefix(long, "été été") {
		t.Errorf("preview of a long Message = %q, want its first 200 characters intact", long)
	}
	if previews["Decorated"] != "Big title Hello, reset your password." {
		t.Errorf("preview of a decorated HTML Message = %q, want only its visible words", previews["Decorated"])
	}
	if previews["Blank text"] != "Only the HTML has words." {
		t.Errorf("preview of a Message whose text part is blank = %q, want the words of its HTML", previews["Blank text"])
	}
	for subject, want := range map[string]string{
		"Base64":   "Bonjour, ceci est encodé.",
		"Latin":    "Votre commande est prête.",
		"Attached": "The body.",
	} {
		if previews[subject] != want {
			t.Errorf("preview of the %s Message = %q, want %q", subject, previews[subject], want)
		}
	}
	var inboxes inboxList
	member.Get("/api/inboxes").JSON(t, &inboxes)
	if inboxes.Inboxes[0].MessageCount != 8 {
		t.Errorf("the Inbox counts %d Messages, want 8", inboxes.Inboxes[0].MessageCount)
	}
}

func TestDeeplyNestedMessageIsHeldWithoutDelay(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	var nested strings.Builder
	nested.WriteString("Subject: Nested\r\n")
	for level := range 6000 {
		fmt.Fprintf(&nested, "Content-Type: multipart/mixed; boundary=b%d\r\n\r\n--b%d\r\n", level, level)
	}
	nested.WriteString("Content-Type: text/plain\r\n\r\nDeep.\r\n")

	submitted := time.Now()
	if err := i.Submit(plain(i.LoggedCredentials()[0]), "app@staging.example", []string{"a@example.com"}, nested.String()); err != nil {
		t.Fatalf("submit a Message nested 6000 levels deep: %v", err)
	}

	if took := time.Since(submitted); took > 4*time.Second {
		t.Errorf("holding a deeply nested Message took %s", took)
	}
	if held := member.Messages(); len(held) != 1 || held[0].Subject != "Nested" {
		t.Fatalf("held Messages = %+v, want the nested one", held)
	}
}

func TestMessageWithVeryLongLinesIsHeld(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	minified := "Subject: Minified\r\nContent-Type: text/html\r\n\r\n<p>" + strings.Repeat("x", 200_000) + "</p>\r\n"

	if err := i.Submit(plain(i.LoggedCredentials()[0]), "app@staging.example", []string{"a@example.com"}, minified); err != nil {
		t.Fatalf("submit a Message with a 200 kB line: %v", err)
	}

	if held := member.Messages(); len(held) != 1 || held[0].Subject != "Minified" {
		t.Fatalf("held Messages = %+v, want the minified one", held)
	}
}

func TestEachTransactionOnAConnectionKeepsItsOwnEnvelope(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	client, err := i.Connect(plain(i.LoggedCredentials()[0]))
	if err != nil {
		t.Fatal(err)
	}

	if err := transmit(client, "first@staging.example", []string{"one@example.com"}, "Subject: First\r\n\r\nHello.\r\n"); err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	if err := errors.Join(client.Mail("abandoned@staging.example"), client.Rcpt("stale@example.com")); err != nil {
		t.Fatalf("abandoned transaction: %v", err)
	}
	if err := transmit(client, "second@staging.example", []string{"two@example.com"}, "Subject: Second\r\n\r\nHello.\r\n"); err != nil {
		t.Fatalf("second transaction: %v", err)
	}

	envelopes := map[string]message{}
	for _, held := range member.Messages() {
		envelopes[held.Subject] = held
	}
	for subject, want := range map[string]struct{ sender, recipient string }{
		"First":  {"first@staging.example", "one@example.com"},
		"Second": {"second@staging.example", "two@example.com"},
	} {
		if got := envelopes[subject]; got.EnvelopeSender != want.sender || !slices.Equal(got.EnvelopeRecipients, []string{want.recipient}) {
			t.Errorf("%s held with envelope %q to %q, want %q to %q", subject, got.EnvelopeSender, got.EnvelopeRecipients, want.sender, want.recipient)
		}
	}
}

func TestMessagesAreListedNewestFirstInPages(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	application := plain(i.LoggedCredentials()[0])
	for n := 1; n <= 51; n++ {
		if n > 2 && n < 50 {
			i.Clock.Advance(time.Second)
		}
		data := fmt.Sprintf("From: app@example.com\r\nSubject: Message %d\r\n\r\nHello.\r\n", n)
		if err := i.Submit(application, "app@example.com", []string{"customer@example.com"}, data); err != nil {
			t.Fatalf("submit Message %d: %v", n, err)
		}
	}

	first := member.MessagePage("")
	second := member.MessagePage(first.Next)

	if len(first.Messages) != 50 || first.Next == "" || len(second.Messages) != 1 || second.Next != "" {
		t.Fatalf("pages hold %d then %d Messages (next %q then %q), want 50 with a next page, then 1 without",
			len(first.Messages), len(second.Messages), first.Next, second.Next)
	}
	if first.Messages[3].Subject != "Message 48" || first.Messages[48].Subject != "Message 3" {
		t.Errorf("first page runs from %q to %q between its ties, want Message 48 down to Message 3",
			first.Messages[3].Subject, first.Messages[48].Subject)
	}
	all := append(first.Messages, second.Messages...)
	subjects := map[string]bool{}
	for n, listed := range all {
		subjects[listed.Subject] = true
		if n > 0 && listed.ReceivedAt.After(all[n-1].ReceivedAt) {
			t.Errorf("%q is listed after an older Message", listed.Subject)
		}
	}
	if len(subjects) != 51 {
		t.Errorf("the two pages list %d distinct Messages, want all 51", len(subjects))
	}
}

func TestMessageListRefusesWhatItCannotResolve(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	var inboxes inboxList
	member.Get("/api/inboxes").JSON(t, &inboxes)

	if missing := member.Get("/api/inboxes/no-such-inbox/messages"); missing.Status != http.StatusNotFound {
		t.Errorf("Message list of an unknown Inbox = %d, want 404", missing.Status)
	}
	if malformed := member.Get("/api/inboxes/" + inboxes.Inboxes[0].ID + "/messages?before=nonsense"); malformed.Status != http.StatusBadRequest {
		t.Errorf("Message list from a malformed position = %d, want 400", malformed.Status)
	}
}

func TestStopDoesNotWaitForAnIdleApplication(t *testing.T) {
	i := start(t)
	if _, err := i.Connect(plain(i.LoggedCredentials()[0])); err != nil {
		t.Fatal(err)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- i.server.Stop(context.Background()) }()

	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("stop with an idle SMTP connection: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server waits for an idle SMTP connection to stop")
	}
	i.boot()
}

func TestFailedStartDoesNotUseUpTheFirstInbox(t *testing.T) {
	i := start(t)
	data := filepath.Join(t.TempDir(), "oubliette.db")
	var output logs
	busy := app.Config{
		DataFile:     data,
		HTTPAddr:     i.server.HTTPAddr(),
		SMTPAddr:     "127.0.0.1:0",
		InsecureAuth: true,
		Now:          i.Clock.Now,
		Log:          slog.New(slog.NewJSONHandler(&output, nil)),
	}

	if failed, err := app.Start(busy); err == nil {
		failed.Stop(context.Background())
		t.Fatal("the server started on an address already in use")
	}
	if strings.Contains(output.String(), "smtp_password") {
		t.Fatalf("a start that failed logged Credentials the Operator may never see again: %s", output.String())
	}

	busy.HTTPAddr = "127.0.0.1:0"
	recovered, err := app.Start(busy)
	if err != nil {
		t.Fatalf("start after a failed start: %v", err)
	}
	defer recovered.Stop(context.Background())
	if logged := strings.Count(output.String(), "smtp_password"); logged != 1 {
		t.Fatalf("the start after a failed start logged Credentials %d times, want once", logged)
	}
}

func TestServerRefusesToStartWithoutTheInsecureAuthenticationOverride(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	args := []string{"--data", filepath.Join(t.TempDir(), "oubliette.db"), "--http-addr", "127.0.0.1:0", "--smtp-addr", "127.0.0.1:0", "serve"}

	code := app.Main(ctx, args, strings.NewReader(""), io.Discard, &stderr, func(string) string { return "" })

	if code == 0 {
		t.Fatal("serve started without the insecure-authentication override")
	}
	if !strings.Contains(stderr.String(), "--smtp-insecure-auth") {
		t.Fatalf("the refusal does not name the override: %s", stderr.String())
	}

	stderr.Reset()
	misspelt := map[string]string{"OUBLIETTE_SMTP_INSECURE_AUTH": "yes"}
	code = app.Main(ctx, args, strings.NewReader(""), io.Discard, &stderr, func(key string) string { return misspelt[key] })
	if code == 0 || !strings.Contains(stderr.String(), "OUBLIETTE_SMTP_INSECURE_AUTH") {
		t.Fatalf("an unreadable override gave exit %d and %q, want a refusal naming the variable", code, stderr.String())
	}
}

func TestServeCommandRunsUntilInterrupted(t *testing.T) {
	t.Parallel()
	ctx, interrupt := context.WithCancel(context.Background())
	defer interrupt()
	output, stdout := io.Pipe()
	exit := make(chan int, 1)
	go func() {
		args := []string{"--data", filepath.Join(t.TempDir(), "oubliette.db"), "--http-addr", "127.0.0.1:0", "serve"}
		env := map[string]string{"OUBLIETTE_SMTP_ADDR": "127.0.0.1:0", "OUBLIETTE_SMTP_INSECURE_AUTH": "true"}
		exit <- app.Main(ctx, args, strings.NewReader(""), stdout, io.Discard, func(key string) string { return env[key] })
		stdout.Close()
	}()

	var addr string
	for lines := bufio.NewScanner(output); addr == "" && lines.Scan(); {
		if listening := regexp.MustCompile(`http=(127\.0\.0\.1:\d+)`).FindStringSubmatch(lines.Text()); listening != nil {
			addr = listening[1]
		}
	}
	var afterListening logs
	drained := make(chan struct{})
	go func() {
		io.Copy(&afterListening, output)
		close(drained)
	}()
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
	<-drained
	if strings.Contains(afterListening.String(), "level=ERROR") {
		t.Errorf("a clean stop logged an error: %s", afterListening.String())
	}
}
