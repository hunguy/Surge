package clipboard

import (
	"regexp"
	"strings"
)

// clipboardReadAll and clipboardWriteAll are the platform clipboard
// implementations, defined per OS in backend_linux.go and backend_other.go.
// They are package variables so tests can stub the system clipboard.
var clipboardReadAll = readAll
var clipboardWriteAll = writeAll

// Read returns the current text content of the system clipboard.
func Read() (string, error) {
	return clipboardReadAll()
}

// Write copies the given text to the system clipboard.
func Write(text string) error {
	return clipboardWriteAll(text)
}

// ParseCurl parses a standard cURL command string into a URL and headers.
func ParseCurl(text string) (string, map[string]string) {
	headers := make(map[string]string)
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "curl ") {
		return "", headers
	}

	headerRe := regexp.MustCompile(`(?:-H|--header)\s+(?:'([^:]+):\s*([^']+)'|"([^:]+):\s*([^"]+)")`)
	for _, m := range headerRe.FindAllStringSubmatch(text, -1) {
		if m[1] != "" {
			headers[strings.TrimSpace(m[1])] = strings.TrimSpace(m[2])
		} else if m[3] != "" {
			headers[strings.TrimSpace(m[3])] = strings.TrimSpace(m[4])
		}
	}

	textWithoutHeaders := headerRe.ReplaceAllString(text, "")
	urlRe := regexp.MustCompile(`(?:^|\s)['"]?(https?://[^'"\s]+)['"]?`)
	if match := urlRe.FindStringSubmatch(textWithoutHeaders); len(match) > 1 {
		return match[1], headers
	}
	return "", headers
}
