package app_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"oubliette/internal/app"
)

func TestEverySettingHasAVariableAndIsDocumentedWithItsDefault(t *testing.T) {
	t.Parallel()
	var help bytes.Buffer
	if code := app.Main(context.Background(), []string{"--help"}, strings.NewReader(""), io.Discard, &help, func(string) string { return "" }); code != 0 {
		t.Fatalf("--help exited with %d", code)
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	rows, row := map[string]string{}, regexp.MustCompile("^\\| `--([a-z-]+)`")
	for line := range strings.Lines(string(readme)) {
		if flag := row.FindStringSubmatch(line); flag != nil {
			rows[flag[1]] = line
		}
	}

	settings := regexp.MustCompile(`(?m)^  -([a-z-]+).*\n\s+.*\((OUBLIETTE_[A-Z_]+)\)(?: \(default "?([^")]*)"?\))?$`).FindAllStringSubmatch(help.String(), -1)
	if flags := strings.Count(help.String(), "\n  -"); len(settings) != flags || flags < 11 {
		t.Fatalf("%d of %d flags name their variable in --help:\n%s", len(settings), flags, help.String())
	}
	for _, setting := range settings {
		name, variable, fallback := setting[1], setting[2], setting[3]
		if want := "OUBLIETTE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")); variable != want {
			t.Errorf("--%s is set by %s, want %s", name, variable, want)
		}
		line, documented := rows[name]
		if !documented || !strings.Contains(line, "`"+variable+"`") || (fallback != "" && !strings.Contains(line, "`"+fallback+"`")) {
			t.Errorf("the README has no row naming --%s, %s and its default %q", name, variable, fallback)
		}
	}
}
