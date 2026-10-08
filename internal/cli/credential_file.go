package cli

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// This bounded dotenv subset accepts assignments, export, quotes and comments.
// Values are literal: no variable expansion, command substitution or execution.
func parseCredentialFile(raw []byte) (map[string]string, error) {
	if !utf8.Valid(raw) || strings.IndexByte(string(raw), 0) >= 0 {
		return nil, errCredentialFile
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
			line = strings.TrimSpace(line[len("export"):])
		}
		name, text, found := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !found || !environmentName.MatchString(name) {
			return nil, errCredentialFile
		}
		value, ok := credentialFileValue(strings.TrimSpace(text))
		if !ok || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, errCredentialFile
		}
		switch name {
		case "SBER_LOGIN", "SBER_PASSWORD", "SBER_PINCODE", "SBER_PHONE", "SBER_CARD_NUMBER":
			values[name] = value
		}
	}
	return values, nil
}

func credentialFileValue(text string) (string, bool) {
	if text == "" {
		return "", true
	}
	quote := text[0]
	if quote == '\'' || quote == '"' {
		for i := 1; i < len(text); i++ {
			if quote == '"' && text[i] == '\\' {
				i++
				continue
			}
			if text[i] != quote {
				continue
			}
			rest := strings.TrimSpace(text[i+1:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return "", false
			}
			if quote == '\'' {
				return text[1:i], true
			}
			value, err := strconv.Unquote(text[:i+1])
			return value, err == nil
		}
		return "", false
	}
	for i := range text {
		if text[i] == '#' && (i == 0 || text[i-1] == ' ' || text[i-1] == '\t') {
			return strings.TrimSpace(text[:i]), true
		}
	}
	return text, true
}
