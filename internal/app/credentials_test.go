package app_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"oubliette/internal/app"
)

type shownCredentials struct {
	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int    `json:"smtpPort"`
	SMTPUsername string `json:"smtpUsername"`
	SMTPPassword string `json:"smtpPassword"`
}

func (b *browser) CredentialsOf(inboxID string) shownCredentials {
	b.i.t.Helper()
	var shown shownCredentials
	b.Get("/api/inboxes/"+inboxID+"/credentials").JSON(b.i.t, &shown)
	return shown
}

func listeningPort(t *testing.T, addr string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SMTP address %q: %v", addr, err)
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("SMTP port %q: %v", port, err)
	}
	return number
}

func TestMemberDisplaysTheCredentialsOfAnInboxAgain(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")

	shown := member.CredentialsOf(shop.ID)

	want := shownCredentials{
		SMTPPort:     listeningPort(t, i.SMTPAddr),
		SMTPUsername: shop.SMTPUsername,
		SMTPPassword: shop.SMTPPassword,
	}
	if shown != want {
		t.Errorf("Credentials of Shop = %+v, want %+v: no host, so the page shows its own, the SMTP listening port and the pair given at creation", shown, want)
	}
}

func TestCredentialsOfAnUnknownInboxAreNotFound(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if unknown := member.Do(method, "/api/inboxes/no-such-inbox/credentials", ""); unknown.Status != http.StatusNotFound {
			t.Errorf("%s the Credentials of an unknown Inbox = %d, want 404", method, unknown.Status)
		}
	}
}

func TestOperatorSetsTheAdvertisedSMTPHostAndPort(t *testing.T) {
	i := startWith(t, func(cfg *app.Config) {
		cfg.SMTPAdvertisedHost = "smtp.agency.example"
		cfg.SMTPAdvertisedPort = "2525"
	})
	member := i.SignedInMember()

	shown := member.CredentialsOf(member.InboxID())

	if shown.SMTPHost != "smtp.agency.example" || shown.SMTPPort != 2525 {
		t.Errorf("advertised SMTP address = %s:%d, want smtp.agency.example:2525", shown.SMTPHost, shown.SMTPPort)
	}
}

func TestServerRefusesAnAdvertisedAddressItCannotShow(t *testing.T) {
	t.Parallel()
	for _, setting := range []struct{ name, value string }{
		{"OUBLIETTE_SMTP_ADVERTISED_PORT", "25a"},
		{"OUBLIETTE_SMTP_ADVERTISED_PORT", "+25"},
		{"OUBLIETTE_SMTP_ADVERTISED_PORT", "70000"},
		{"OUBLIETTE_SMTP_ADVERTISED_HOST", "smtp.agency.example:2525"},
		{"OUBLIETTE_SMTP_ADVERTISED_HOST", "http://smtp.agency.example"},
		{"OUBLIETTE_SMTP_ADVERTISED_HOST", "smtp agency"},
	} {
		var stderr bytes.Buffer
		args := []string{"--data", filepath.Join(t.TempDir(), "oubliette.db"), "--http-addr", "127.0.0.1:0", "--smtp-addr", "127.0.0.1:0", "--smtp-insecure-auth", "serve"}
		env := map[string]string{setting.name: setting.value}

		ctx, cancel := context.WithTimeout(context.Background(), patience)
		code := app.Main(ctx, args, strings.NewReader(""), io.Discard, &stderr, func(key string) string { return env[key] })
		cancel()

		if code == 0 || !strings.Contains(stderr.String(), setting.name) {
			t.Errorf("serve with %s=%q gave exit %d and %q, want a refusal naming the setting", setting.name, setting.value, code, stderr.String())
		}
	}
}

func TestRegeneratingCredentialsCutsOffThePreviousPairAtOnce(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	before := shop.Credentials()
	connected, err := i.Connect(plain(before))
	if err != nil {
		t.Fatalf("authenticate with the first pair: %v", err)
	}

	regenerated := member.Do(http.MethodPost, "/api/inboxes/"+shop.ID+"/credentials", "")

	var issued shownCredentials
	regenerated.JSON(t, &issued)
	after := credentials{issued.SMTPUsername, issued.SMTPPassword}
	if after.Username == before.Username || after.Password == before.Password || len(after.Password) < 26 {
		t.Fatalf("regenerated pair %+v, want a new username and a new password replacing %+v", after, before)
	}
	if shown := member.CredentialsOf(shop.ID); shown.SMTPUsername != after.Username || shown.SMTPPassword != after.Password {
		t.Errorf("Credentials displayed after regenerating = %+v, want the new pair", shown)
	}
	if err := transmit(connected, "app@example.com", []string{"a@example.com"}, welcome); err == nil {
		t.Error("a connection authenticated with the previous pair still sends, want it refused")
	}
	if err := i.Submit(plain(before), "app@example.com", []string{"a@example.com"}, welcome); err == nil {
		t.Error("the previous pair still authenticates, want it refused")
	}
	if err := i.Submit(plain(after), "app@example.com", []string{"a@example.com"}, welcome); err != nil {
		t.Errorf("the new pair is refused: %v", err)
	}
	if held := member.MessagesIn(shop.ID); len(held) != 1 {
		t.Errorf("Messages of Shop = %d, want only the one sent with the new pair", len(held))
	}
}

func TestVisitorNeverSeesTheCredentials(t *testing.T) {
	i := start(t)
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	i.CaptureIn(shop, "Order")
	token := member.Share(shop.ID, "Client").Token
	visitor := i.Browser()

	for _, path := range []string{"/api/share/" + token, "/api/share/" + token + "/messages"} {
		if body := visitor.Get(path).Body; strings.Contains(body, shop.SMTPUsername) || strings.Contains(body, shop.SMTPPassword) {
			t.Errorf("GET %s shows the Visitor the Credentials: %s", path, body)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if refused := visitor.Do(method, "/api/share/"+token+"/credentials", ""); refused.Status != http.StatusNotFound {
			t.Errorf("%s the Credentials through the Share Link = %d, want 404", method, refused.Status)
		}
	}
}
