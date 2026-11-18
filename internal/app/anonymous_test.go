package app_test

import (
	"net/http"
	"net/smtp"
	"slices"
	"strings"
	"testing"

	"oubliette/internal/app"
)

func startAnonymous(t *testing.T, name string) *instance {
	t.Helper()
	return startWith(t, func(cfg *app.Config) { cfg.SMTPAnonymousInbox = name })
}

func TestAMessageSentWithoutCredentialsIsHeldInTheAnonymousInbox(t *testing.T) {
	i := startAnonymous(t, "Mailpit")

	if err := i.Submit(nil, "app@staging.example", []string{"a@example.com"}, "Subject: No credentials\r\n\r\nHello.\r\n"); err != nil {
		t.Fatalf("submit without AUTH = %v, want it held", err)
	}

	member := i.SignedInMember()
	anonymous := member.InboxNamed("Mailpit")
	if !anonymous.Anonymous || member.InboxNamed("Default").Anonymous {
		t.Errorf("inboxes = %+v, want only Mailpit marked anonymous", member.Inboxes())
	}
	if held := member.MessagesIn(anonymous.ID); len(held) != 1 || held[0].Subject != "No credentials" || held[0].EnvelopeSender != "app@staging.example" {
		t.Errorf("Anonymous Inbox holds %+v, want the Message sent without credentials", held)
	}
	if held := member.MessagesIn(member.InboxNamed("Default").ID); len(held) != 0 {
		t.Errorf("Default holds %d Messages, want none", len(held))
	}
}

func TestUnknownCredentialsFallIntoTheAnonymousInboxAndValidOnesKeepTheirInbox(t *testing.T) {
	i := startAnonymous(t, "Mailpit")
	member := i.SignedInMember()
	shop := member.CreateInbox("Shop")
	before := shop.Credentials()

	for _, sent := range []struct {
		auth    smtp.Auth
		subject string
	}{
		{plain(credentials{"placeholder", "secret"}), "Unknown pair"},
		{login(credentials{before.Username, "wrong password"}), "Wrong password"},
		{plain(before), "Shop pair"},
	} {
		if err := i.Submit(sent.auth, "app@staging.example", []string{"a@example.com"}, "Subject: "+sent.subject+"\r\n\r\nHello.\r\n"); err != nil {
			t.Fatalf("submit %q = %v, want it held", sent.subject, err)
		}
	}
	member.Do(http.MethodPost, "/api/inboxes/"+shop.ID+"/credentials", "")
	if err := i.Submit(plain(before), "app@staging.example", []string{"a@example.com"}, "Subject: Stale pair\r\n\r\nHello.\r\n"); err != nil {
		t.Fatalf("submit with the regenerated-away pair = %v, want it held", err)
	}

	if got := slices.Sorted(slices.Values(subjects(member.MessagesIn(member.InboxNamed("Mailpit").ID)))); !slices.Equal(got, []string{"Stale pair", "Unknown pair", "Wrong password"}) {
		t.Errorf("Anonymous Inbox holds %q, want the unknown, wrong and stale pairs", got)
	}
	if got := subjects(member.MessagesIn(shop.ID)); !slices.Equal(got, []string{"Shop pair"}) {
		t.Errorf("Shop holds %q, want only the Message sent with its Credentials", got)
	}
}

func TestTheAnonymousInboxBindsToAnExistingInboxOfThatName(t *testing.T) {
	i := startAnonymous(t, "Default")
	member := i.SignedInMember()
	if err := i.Submit(nil, "app@staging.example", []string{"a@example.com"}, "Subject: Anonymous\r\n\r\nHello.\r\n"); err != nil {
		t.Fatal(err)
	}
	i.Capture("Subject: With the Default pair\r\n\r\nHello.\r\n")

	if inboxes := member.Inboxes(); len(inboxes) != 1 || !inboxes[0].Anonymous {
		t.Fatalf("inboxes = %+v, want Default alone, marked anonymous", inboxes)
	}
	if held := member.Messages(); len(held) != 2 {
		t.Errorf("Default holds %d Messages, want both", len(held))
	}
	i.Restart()
	if inboxes := member.Inboxes(); len(inboxes) != 1 {
		t.Errorf("inboxes after a restart = %+v, want Default alone", inboxes)
	}
}

func TestWithACertificateAMessageWithoutAuthIsAcceptedWithoutStartTLS(t *testing.T) {
	cert := newCertificate(t)
	i := startWith(t, func(cfg *app.Config) {
		cfg.InsecureAuth = false
		cfg.SMTPCertFile, cfg.SMTPKeyFile = cert.certFile, cert.keyFile
		cfg.SMTPAnonymousInbox = "Mailpit"
	})

	if err := i.Submit(nil, "app@staging.example", []string{"a@example.com"}, "Subject: Clear\r\n\r\nHello.\r\n"); err != nil {
		t.Errorf("submit without AUTH nor STARTTLS = %v, want it held", err)
	}
	client, err := i.Connect(nil)
	if err != nil {
		t.Fatal(err)
	}
	if offered, _ := client.Extension("AUTH"); offered {
		t.Errorf("AUTH offered before STARTTLS, want it still withheld")
	}
}

func TestTheAnonymousInboxCannotBeRenamedOrDeletedButCanBeEmptied(t *testing.T) {
	i := startAnonymous(t, "Mailpit")
	member := i.SignedInMember()
	anonymous := member.InboxNamed("Mailpit").ID
	i.Submit(nil, "app@staging.example", []string{"a@example.com"}, "Subject: Anonymous\r\n\r\nHello.\r\n")

	for _, attempt := range []response{
		member.Do(http.MethodPatch, "/api/inboxes/"+anonymous, named("Renamed")),
		member.Do(http.MethodDelete, "/api/inboxes/"+anonymous, ""),
	} {
		if attempt.Status != http.StatusConflict || !strings.Contains(attempt.Body, "--smtp-anonymous-inbox") {
			t.Errorf("rename or delete of the Anonymous Inbox = %d %s, want 409 naming the setting", attempt.Status, attempt.Body)
		}
	}
	if emptied := member.Do(http.MethodDelete, "/api/inboxes/"+anonymous+"/messages", ""); emptied.Status != http.StatusNoContent {
		t.Errorf("empty the Anonymous Inbox = %d %s, want 204", emptied.Status, emptied.Body)
	}
	if member.InboxNamed("Mailpit").MessageCount != 0 {
		t.Errorf("Anonymous Inbox still holds Messages after emptying")
	}
	other := member.CreateInbox("Other")
	if renamed := member.Do(http.MethodPatch, "/api/inboxes/"+other.ID, named("Renamed")); renamed.Status != http.StatusNoContent {
		t.Errorf("rename another Inbox = %d, want 204", renamed.Status)
	}
}

func TestTheCredentialsOfTheAnonymousInboxSayNoneAreNeeded(t *testing.T) {
	i := startAnonymous(t, "Mailpit")
	member := i.SignedInMember()

	var anonymous, other struct {
		Anonymous bool `json:"anonymous"`
	}
	member.Get("/api/inboxes/"+member.InboxNamed("Mailpit").ID+"/credentials").JSON(t, &anonymous)
	member.Get("/api/inboxes/"+member.InboxNamed("Default").ID+"/credentials").JSON(t, &other)
	if !anonymous.Anonymous || other.Anonymous {
		t.Errorf("credentials flags = %v for the Anonymous Inbox and %v for Default, want true and false", anonymous.Anonymous, other.Anonymous)
	}
}

func TestAnUnusableAnonymousInboxNameStopsTheStart(t *testing.T) {
	t.Parallel()

	for name, run := range map[string]struct {
		env  map[string]string
		args []string
	}{
		"blank variable":         {map[string]string{"OUBLIETTE_SMTP_ANONYMOUS_INBOX": "   "}, nil},
		"control character flag": {nil, []string{"--smtp-anonymous-inbox", "\x07bell"}},
	} {
		env := map[string]string{"OUBLIETTE_SMTP_INSECURE_AUTH": "true"}
		for key, value := range run.env {
			env[key] = value
		}
		code, refusal := serveWith(t, env, run.args...)
		if code == 0 || !strings.Contains(refusal, "--smtp-anonymous-inbox") {
			t.Errorf("%s gave exit %d and %q, want a refusal naming the setting", name, code, refusal)
		}
	}
}

func TestTheAnonymousInboxNameIgnoresCase(t *testing.T) {
	i := startAnonymous(t, "default")
	member := i.SignedInMember()

	if inboxes := member.Inboxes(); len(inboxes) != 1 || inboxes[0].Name != "Default" || !inboxes[0].Anonymous {
		t.Errorf("inboxes = %+v, want Default alone, bound as the Anonymous Inbox", inboxes)
	}
}
