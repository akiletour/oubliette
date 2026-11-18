package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"slices"
	"strings"
	"testing"
	"time"

	"oubliette/internal/app"
)

func sized(subject string, size int) string {
	head := "Subject: " + subject + "\r\n\r\n"
	return head + strings.Repeat("x", size-len(head)-2) + "\r\n"
}

func TestAMessageLargerThanTheSizeLimitIsRefused(t *testing.T) {
	i := startWith(t, func(cfg *app.Config) { cfg.MaxMessageBytes = 1000 })
	application := plain(i.LoggedCredentials()[0])

	err := i.Submit(application, "app@example.com", []string{"a@example.com"}, sized("One byte too many", 1001))

	var refusal *textproto.Error
	if !errors.As(err, &refusal) || refusal.Code != 552 || !strings.Contains(strings.ToLower(refusal.Msg), "size") {
		t.Fatalf("an oversized Message gave %v, want a 552 refusal that mentions the size", err)
	}
	if err := i.Submit(application, "app@example.com", []string{"a@example.com"}, sized("Exactly the limit", 1000)); err != nil {
		t.Fatalf("a Message of exactly the size limit: %v", err)
	}
	messages := i.SignedInMember().Messages()
	if len(messages) != 1 || messages[0].Subject != "Exactly the limit" {
		t.Fatalf("held Messages = %+v, want only the one within the limit", messages)
	}
}

func TestTheSizeLimitIsAdvertisedAndADeclaredSizeAboveItIsRefused(t *testing.T) {
	i := startWith(t, func(cfg *app.Config) { cfg.MaxMessageBytes = 1000 })
	client, err := i.Connect(plain(i.LoggedCredentials()[0]))
	if err != nil {
		t.Fatal(err)
	}
	if _, advertised := client.Extension("SIZE"); advertised == "" {
		t.Fatal("the size limit is not advertised")
	}

	id, err := client.Text.Cmd("MAIL FROM:<app@example.com> SIZE=5000")
	if err != nil {
		t.Fatal(err)
	}
	client.Text.StartResponse(id)
	code, message, err := client.Text.ReadResponse(250)
	client.Text.EndResponse(id)

	if code != 552 || !strings.Contains(strings.ToLower(message), "size") {
		t.Fatalf("MAIL with a declared size above the limit = %d %q (%v), want a 552 refusal", code, message, err)
	}
}

func TestTheOldestMessagesLeaveAnInboxAtItsCap(t *testing.T) {
	i := startWith(t, func(cfg *app.Config) { cfg.InboxCap = 3 })
	member := i.SignedInMember()
	watching := member.Stream("/api/inboxes/" + member.InboxID())
	for n := range 5 {
		i.Capture(fmt.Sprintf("Subject: Message %d\r\n\r\nHello\r\n", n))
		i.Clock.Advance(time.Second)
	}

	var subjects []string
	for _, message := range member.Messages() {
		subjects = append(subjects, message.Subject)
	}
	if want := []string{"Message 4", "Message 3", "Message 2"}; !slices.Equal(subjects, want) {
		t.Fatalf("held Messages = %q, want %q", subjects, want)
	}
	for removed := 0; removed < 2; {
		next, open := watching.Next()
		if !open {
			t.Fatal("the stream closed before announcing the removed Messages")
		}
		if next.Name == "removed" {
			removed++
		}
	}
}

func TestMessagesOlderThanTheAgeLimitArePurged(t *testing.T) {
	i := startWith(t, func(cfg *app.Config) {
		cfg.AgeLimit = 7 * 24 * time.Hour
		cfg.PurgeEvery = 10 * time.Millisecond
	})
	member := i.SignedInMember()
	i.Capture("Subject: Old\r\n\r\nHello\r\n")
	old := member.Messages()[0]
	i.Clock.Advance(2 * 24 * time.Hour)
	i.Capture("Subject: Young\r\n\r\nHello\r\n")
	watching := member.Stream("/api/inboxes/" + member.InboxID())

	i.Clock.Advance(5*24*time.Hour + time.Second)

	for next, open := watching.Next(); next.Name != "removed" || next.ID != old.ID; next, open = watching.Next() {
		if !open {
			t.Fatal("the stream closed before announcing the purge")
		}
	}
	if messages := member.Messages(); len(messages) != 1 || messages[0].Subject != "Young" {
		t.Fatalf("held Messages after the purge = %+v, want only the young one", messages)
	}
	if gone := member.Get(member.MessagePath(old.ID)); gone.Status != http.StatusNotFound {
		t.Fatalf("GET a purged Message = %d, want 404", gone.Status)
	}
}

func TestMessagesThatAgedWhileTheServerWasStoppedArePurgedAtStart(t *testing.T) {
	i := startWith(t, func(cfg *app.Config) {
		cfg.AgeLimit = 7 * 24 * time.Hour
		cfg.PurgeEvery = time.Hour
	})
	member := i.SignedInMember()
	i.Capture("Subject: Old\r\n\r\nHello\r\n")

	i.Clock.Advance(8 * 24 * time.Hour)
	i.Restart()

	if messages := member.Messages(); len(messages) != 0 {
		t.Fatalf("held Messages right after a start = %+v, want the old one purged", messages)
	}
}

func TestAWatcherNeverSilentlyMissesAPurge(t *testing.T) {
	const backlog = 300
	i := startWith(t, func(cfg *app.Config) {
		cfg.AgeLimit = 7 * 24 * time.Hour
		cfg.PurgeEvery = 10 * time.Millisecond
	})
	client, err := i.Connect(plain(i.LoggedCredentials()[0]))
	if err != nil {
		t.Fatal(err)
	}
	for n := range backlog {
		if err := transmit(client, "app@example.com", []string{"a@example.com"}, fmt.Sprintf("Subject: Message %d\r\n\r\nHello\r\n", n)); err != nil {
			t.Fatal(err)
		}
	}
	member := i.SignedInMember()
	watching := member.Stream("/api/inboxes/" + member.InboxID())

	i.Clock.Advance(8 * 24 * time.Hour)

	for removed := 0; removed < backlog; {
		next, open := watching.Next()
		if !open {
			return
		}
		if next.Name == "removed" {
			removed++
		}
	}
}

func TestTheLimitsDefaultTo30Days1000MessagesAnd25MB(t *testing.T) {
	i := start(t)
	client, err := i.Connect(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, advertised := client.Extension("SIZE"); advertised != "26214401" {
		t.Errorf("SIZE advertised by default as %q, want 25 MB, the last byte included", advertised)
	}

	var help strings.Builder
	app.Main(context.Background(), []string{"--help"}, strings.NewReader(""), io.Discard, &help, func(string) string { return "" })

	for _, setting := range []string{"-retention-days value", "(default 30)", "-inbox-cap value", "(default 1000)", "-max-message-mb value", "(default 25)"} {
		if !strings.Contains(help.String(), setting) {
			t.Errorf("the help does not show %q:\n%s", setting, help.String())
		}
	}
}

func TestServerRefusesToStartWithAnUnusableLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, want string
		env        map[string]string
		args       []string
	}{
		{"zero cap", "-inbox-cap", nil, []string{"--inbox-cap", "0"}},
		{"negative age", "-retention-days", nil, []string{"--retention-days", "-1"}},
		{"size that is not a number", "-max-message-mb", nil, []string{"--max-message-mb", "25MB"}},
		{"size beyond what opening a Message can bear", "-max-message-mb", nil, []string{"--max-message-mb", "1000"}},
		{"cap from an unreadable variable", "OUBLIETTE_INBOX_CAP", map[string]string{"OUBLIETTE_INBOX_CAP": "many"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			code, refusal := serveWith(t, c.env, append([]string{"--smtp-insecure-auth"}, c.args...)...)

			if code == 0 || !strings.Contains(refusal, c.want) {
				t.Fatalf("exit %d and %q, want a refusal naming %s", code, refusal, c.want)
			}
		})
	}
}

func TestAnUnusableLimitDoesNotStopTheMemberCommand(t *testing.T) {
	i := start(t)
	env := map[string]string{"OUBLIETTE_DATA": i.Data, "OUBLIETTE_INBOX_CAP": "many"}
	var stderr strings.Builder

	code := app.Main(context.Background(), []string{"member", "list"}, strings.NewReader(""), io.Discard, &stderr, func(key string) string { return env[key] })

	if code != 0 {
		t.Fatalf("member list with an unusable limit exited %d: %s", code, stderr.String())
	}
}
