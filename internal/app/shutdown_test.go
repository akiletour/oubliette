package app_test

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoppingFinishesTheMessageBeingReceivedAndLeavesACopyableDataFile(t *testing.T) {
	i := start(t)
	sending, err := i.Connect(plain(i.LoggedCredentials()[0]))
	if err != nil {
		t.Fatal(err)
	}
	if err := sending.Mail("app@staging.example"); err != nil {
		t.Fatal(err)
	}
	if err := sending.Rcpt("a@example.com"); err != nil {
		t.Fatal(err)
	}
	body, err := sending.Data()
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(body, "Subject: In flight\r\n\r\nFirst half, ")

	stopped := i.StopInBackground()
	waitUntilRefused(t, i.SMTPAddr)
	waitUntilRefused(t, strings.TrimPrefix(i.URL, "http://"))
	time.Sleep(3 * time.Second)
	io.WriteString(body, "second half.\r\n")
	if err := body.Close(); err != nil {
		t.Errorf("finishing a slow Message after the stop began = %v, want it held", err)
	}
	if err := <-stopped; err != nil {
		t.Errorf("stop = %v", err)
	}

	i.Data = copyOnly(t, i.Data)
	i.boot()
	held := i.SignedInMember().Messages()
	if len(held) != 1 || held[0].Subject != "In flight" || held[0].Preview != "First half, second half." {
		t.Errorf("Messages in a copy of the data file alone = %+v, want the one in flight when the stop began", held)
	}
}

func TestAConnectionKeptOpenAfterItsMessageDoesNotHoldTheStop(t *testing.T) {
	i := start(t)
	pooled, err := i.Connect(plain(i.LoggedCredentials()[0]))
	if err != nil {
		t.Fatal(err)
	}
	if err := pooled.Mail("app@staging.example"); err != nil {
		t.Fatal(err)
	}
	if err := pooled.Rcpt("a@example.com"); err != nil {
		t.Fatal(err)
	}
	body, err := pooled.Data()
	if err != nil {
		t.Fatal(err)
	}
	stopped := i.StopInBackground()
	waitUntilRefused(t, i.SMTPAddr)
	io.WriteString(body, "Subject: Last one\r\n\r\nHeld.\r\n")
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("stop = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("a connection left open after its Message kept the server from stopping")
	}
	if err := pooled.Mail("app@staging.example"); err == nil {
		t.Errorf("a new Message on that connection after the stop was accepted")
	}
}

func waitUntilRefused(t *testing.T, addr string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		connection, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		connection.Close()
	}
	t.Fatal(addr + " still accepts connections")
}

func copyOnly(t *testing.T, data string) string {
	t.Helper()
	content, err := os.ReadFile(data)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err := os.WriteFile(backup, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return backup
}
