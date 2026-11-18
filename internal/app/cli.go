package app

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"oubliette/internal/store"
)

const usage = `Usage: oubliette [flags] <command>

Commands:
  serve                         run the server
  member add <email>            add a Member, reading the password from standard input
  member list                   list Members
  member remove <email>         remove a Member
  member set-password <email>   set a Member's password, reading it from standard input

Flags:
`

const stopBudget = 8 * time.Second

var errUsage = errors.New("usage")

func Main(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	flags := flag.NewFlagSet("oubliette", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprint(stderr, usage)
		flags.PrintDefaults()
	}
	data := flags.String("data", cmp.Or(getenv("OUBLIETTE_DATA"), "oubliette.db"), "data file (OUBLIETTE_DATA)")
	httpAddr := flags.String("http-addr", cmp.Or(getenv("OUBLIETTE_HTTP_ADDR"), ":8080"),
		"address the web interface listens on (OUBLIETTE_HTTP_ADDR)")
	smtpAddr := flags.String("smtp-addr", cmp.Or(getenv("OUBLIETTE_SMTP_ADDR"), ":1025"),
		"address the SMTP server listens on (OUBLIETTE_SMTP_ADDR)")
	insecureAuthFromEnv, insecureAuthUnreadable := envBool(getenv, "OUBLIETTE_SMTP_INSECURE_AUTH")
	insecureAuth := flags.Bool("smtp-insecure-auth", insecureAuthFromEnv,
		"accept SMTP authentication without encryption (OUBLIETTE_SMTP_INSECURE_AUTH)")
	tlsCert := flags.String("smtp-tls-cert", getenv("OUBLIETTE_SMTP_TLS_CERT"),
		"PEM certificate file the SMTP server offers through STARTTLS (OUBLIETTE_SMTP_TLS_CERT)")
	tlsKey := flags.String("smtp-tls-key", getenv("OUBLIETTE_SMTP_TLS_KEY"),
		"PEM private key file of that certificate (OUBLIETTE_SMTP_TLS_KEY)")
	advertisedHost := flags.String("smtp-advertised-host", getenv("OUBLIETTE_SMTP_ADVERTISED_HOST"),
		"SMTP host shown to Members, the host of the page when empty (OUBLIETTE_SMTP_ADVERTISED_HOST)")
	advertisedPort := flags.String("smtp-advertised-port", getenv("OUBLIETTE_SMTP_ADVERTISED_PORT"),
		"SMTP port shown to Members, the listening port when empty (OUBLIETTE_SMTP_ADVERTISED_PORT)")
	limits := registerLimits(flags, getenv)
	if err := flags.Parse(args); errors.Is(err, flag.ErrHelp) {
		return 0
	} else if err != nil {
		return 2
	}
	err := errUsage
	switch args = flags.Args(); {
	case len(args) == 1 && args[0] == "serve":
		var cfg Config
		cfg, err = limits.apply(Config{
			DataFile:           *data,
			HTTPAddr:           *httpAddr,
			SMTPAddr:           *smtpAddr,
			InsecureAuth:       *insecureAuth,
			SMTPCertFile:       *tlsCert,
			SMTPKeyFile:        *tlsKey,
			SMTPAdvertisedHost: *advertisedHost,
			SMTPAdvertisedPort: *advertisedPort,
			Now:                time.Now,
			Log:                slog.New(slog.NewTextHandler(stdout, nil)),
		})
		if err = errors.Join(insecureAuthUnreadable, err); err == nil {
			err = serve(ctx, cfg)
		}
	case len(args) > 1 && args[0] == "member":
		err = member(ctx, *data, args[1], args[2:], stdin, stdout, stderr)
	}
	if errors.Is(err, errUsage) {
		flags.Usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "oubliette:", err)
		return 1
	}
	return 0
}

func envBool(getenv func(string) string, variable string) (bool, error) {
	text := getenv(variable)
	if text == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(text)
	if err != nil {
		return false, errors.New(variable + " must be true or false")
	}
	return value, nil
}

func serve(ctx context.Context, cfg Config) error {
	server, err := Start(cfg)
	if err != nil {
		return err
	}
	<-ctx.Done()
	cfg.Log.Info("shutting down")
	stopping, cancel := context.WithTimeout(context.Background(), stopBudget)
	defer cancel()
	return server.Stop(stopping)
}

func member(ctx context.Context, data, action string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	arities := map[string]int{"add": 1, "list": 0, "remove": 1, "set-password": 1}
	if arity, known := arities[action]; !known || len(args) != arity {
		return errUsage
	}
	_, missing := os.Stat(data)
	storage, err := store.Open(data)
	if err != nil {
		return err
	}
	defer storage.Close()
	if errors.Is(missing, fs.ErrNotExist) {
		fmt.Fprintln(stderr, "oubliette: created data file", data)
	}
	switch action {
	case "list":
		emails, err := storage.Members(ctx)
		for _, email := range emails {
			fmt.Fprintln(stdout, email)
		}
		return err
	case "remove":
		return storage.RemoveMember(ctx, args[0])
	}
	password, err := readPassword(ctx, stdin, stderr)
	if err != nil {
		return err
	}
	if action == "add" {
		return storage.AddMember(ctx, args[0], password)
	}
	return storage.SetMemberPassword(ctx, args[0], password)
}

func readPassword(ctx context.Context, stdin io.Reader, stderr io.Writer) (string, error) {
	read := func() (string, error) { return readLine(stdin) }
	if file, isFile := stdin.(*os.File); isFile && term.IsTerminal(int(file.Fd())) {
		terminal := int(file.Fd())
		echoing, err := term.GetState(terminal)
		if err != nil {
			return "", err
		}
		defer term.Restore(terminal, echoing)
		defer fmt.Fprintln(stderr)
		read = func() (string, error) { return promptTwice(terminal, stderr) }
	}
	type outcome struct {
		password string
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		password, err := read()
		done <- outcome{password, err}
	}()
	select {
	case got := <-done:
		return got.password, got.err
	case <-ctx.Done():
		return "", errors.New("interrupted")
	}
}

func readLine(stdin io.Reader) (string, error) {
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func promptTwice(terminal int, stderr io.Writer) (string, error) {
	var entries [2]string
	for n, prompt := range []string{"Password: ", "\nConfirm password: "} {
		fmt.Fprint(stderr, prompt)
		entry, err := term.ReadPassword(terminal)
		if err != nil {
			return "", err
		}
		entries[n] = string(entry)
	}
	if entries[0] != entries[1] {
		return "", errors.New("the passwords do not match")
	}
	return entries[0], nil
}
