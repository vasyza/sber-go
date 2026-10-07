package cli

import (
	"github.com/spf13/pflag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var docInline = regexp.MustCompile("`[^`]+`")
var docLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
var docSentence = regexp.MustCompile(`[.!?](?:\s+|$)`)
var docStep = regexp.MustCompile(`^[0-9]+\.\s+`)
var docContraction = regexp.MustCompile(`(?i)\b[a-z]+['’](t|re|ve|ll|d|m|s)\b`)

func TestCLIDocumentationCommandsOptionsAndExamplesMatchExecutable(t *testing.T) {
	reference, err := os.ReadFile(filepath.Join("..", "..", "docs", "CLI-REFERENCE.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		if !strings.Contains(string(reference), "| `"+command.name+"` |") {
			t.Errorf("missing command %s", command.name)
		}
		flags, _ := commandFlags(command.name)
		flags.VisitAll(func(f *pflag.Flag) {
			if !strings.Contains(string(reference), "`--"+f.Name) {
				t.Errorf("missing option --%s", f.Name)
			}
		})
	}
	manual, err := os.ReadFile(filepath.Join("..", "..", "docs", "CLI.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(manual), "\\\n", " ")
	examples := regexp.MustCompile(`(?m)^\s*\./bin/sber\s+([^\n]+)`).FindAllStringSubmatch(text, -1)
	for _, example := range examples {
		words := regexp.MustCompile(`"[^"]*"|[^\s]+`).FindAllString(example[1], -1)
		for i := range words {
			words[i] = strings.ReplaceAll(strings.Trim(words[i], `"`), "$HOME", "/synthetic/private")
		}
		if words[0] == "--help" || words[0] == "help" {
			continue
		}
		if _, ok := findCommand(words[0]); !ok {
			t.Fatalf("example has unknown command %s", words[0])
		}
		flags, args := commandFlags(words[0])
		if err := flags.Parse(words[1:]); err != nil || flags.NArg() != 0 || !validateCommand(words[0], args) {
			t.Errorf("invalid documented example for %s", words[0])
		}
	}
	if len(examples) < 20 {
		t.Fatal("operator manual lacks executable examples")
	}
}

func TestCLIDocumentationSentenceAndParagraphLimits(t *testing.T) {
	for _, name := range []string{"CLI.md", "CLI-REFERENCE.md", "DEVELOPMENT.md", "STATUS.md", "RENTAL-CLI.md"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "docs", name))
		if err != nil {
			t.Fatal(err)
		}
		fenced := false
		paragraphSentences := 0
		for i, rawLine := range strings.Split(string(raw), "\n") {
			line := strings.TrimSpace(rawLine)
			if strings.HasPrefix(line, "```") {
				fenced = !fenced
				paragraphSentences = 0
				continue
			}
			if fenced || strings.HasPrefix(line, "#") || line == "" {
				paragraphSentences = 0
				continue
			}
			procedure := docStep.MatchString(line)
			line = docStep.ReplaceAllString(line, "")
			line = docInline.ReplaceAllString(line, "LITERAL")
			line = docLink.ReplaceAllString(line, "$1")
			if docContraction.MatchString(line) {
				t.Errorf("%s:%d contraction", name, i+1)
			}
			limit := 25
			if procedure {
				limit = 20
			}
			for _, sentence := range docSentence.Split(line, -1) {
				words := strings.Fields(strings.Trim(sentence, "| -*"))
				if len(words) > limit {
					t.Errorf("%s:%d has %d words; limit %d", name, i+1, len(words), limit)
				}
			}
			if strings.HasPrefix(line, "|") || strings.HasPrefix(line, "- ") || procedure {
				paragraphSentences = 0
				continue
			}
			paragraphSentences += len(docSentence.FindAllString(line, -1))
			if paragraphSentences > 6 {
				t.Errorf("%s:%d paragraph exceeds six sentences", name, i+1)
			}
		}
		if fenced {
			t.Errorf("%s has an unclosed code block", name)
		}
	}
}
