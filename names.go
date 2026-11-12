package main

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

// Profiles keeps the execution representation ordered while requiring mapping
// keys (not a setting inside the value) to supply the exact decoded identity.
type Profiles []Profile

var errProfilesMapping = errors.New("profiles must be a mapping keyed by name; convert list entries' name fields to mapping keys")

func (profiles *Profiles) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return errProfilesMapping
	}
	result := make(Profiles, 0, len(node.Content)/2)
	seen := map[string]bool{}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !validProfileName(key.Value) || seen[key.Value] {
			return errors.New("invalid or duplicate profile name")
		}
		seen[key.Value] = true
		if value.Kind != yaml.MappingNode {
			return errors.New("profile settings must be a mapping")
		}
		// Node.Decode does not inherit Decoder.KnownFields. Check the value schema
		// explicitly, including duplicate fields, before decoding settings.
		fields := map[string]bool{}
		for j := 0; j < len(value.Content); j += 2 {
			field := value.Content[j]
			if field.Kind != yaml.ScalarNode || field.Tag != "!!str" || fields[field.Value] {
				return errors.New("invalid profile setting")
			}
			switch field.Value {
			case "provider", "sources", "target", "token", "username", "password", "endpoint":
			default:
				return errors.New("unknown profile setting")
			}
			fields[field.Value] = true
		}
		var p Profile
		if err := value.Decode(&p); err != nil {
			return errors.New("invalid profile settings")
		}
		p.Name = key.Value
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	*profiles = result
	return nil
}

func validProfileName(name string) bool {
	if name == "" || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		// Include format controls (e.g. bidi overrides) and Unicode line/paragraph
		// separators: none are ordinary visible name text, and all can forge output.
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

// Names are validated at configuration/state boundaries to reject controls and
// line separators. Render the exact visible name without adding quotes or escapes.
func renderProfileName(name string) string { return name }

var httpFailureStatus = regexp.MustCompile(`^provider HTTP failure \([1-9][0-9][0-9]\)$`)

// Pending messages are untrusted durable input, not arbitrary mail bodies. Accept
// only generated status grammar with safe names, including older quoted and legacy names.
// Validate without rewriting: existing queue order/content and at-least-once
// acknowledgements are preserved. Invalid content blocks the run before updates.
func validPendingSummary(summary string) bool {
	if !utf8.ValidString(summary) || !strings.HasSuffix(summary, "\n") {
		return false
	}
	lines := strings.Split(strings.TrimSuffix(summary, "\n"), "\n")
	if value, ok := strings.CutPrefix(lines[0], "Summary generated at: "); ok {
		// A historical profile can have this exact name. Preserve valid old
		// status lines; otherwise accept only the canonical generated UTC time.
		if _, legacy := pendingSummaryName(lines[0]); !legacy {
			generated, err := time.Parse(time.RFC3339, value)
			if err != nil || generated.UTC().Format(time.RFC3339) != value {
				return false
			}
			lines = lines[1:]
		}
	}
	if len(lines) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, line := range lines {
		name, ok := pendingSummaryName(line)
		if !ok || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

// Names are literal visible text. Old quoted names are also safe literal text;
// do not decode their escapes or conflate them with a different raw name.
func pendingSummaryName(line string) (string, bool) {
	// Inspect complete status suffixes, not the first delimiter in a name.
	for end := len(line); end > 0; {
		i := strings.LastIndex(line[:end], ": ")
		if i < 0 {
			break
		}
		if validSummaryStatus(line[i+2:]) && validProfileName(line[:i]) {
			return line[:i], true
		}
		end = i
	}
	return "", false
}

func validSummaryStatus(status string) bool {
	for _, kind := range []string{"desec-auth", "desec-hostname", "desec-target"} {
		if status == retryMessage(kind, time.Time{}) {
			return true
		}
	}
	for _, prefix := range []string{"deSEC rate limited; retry at ", "HE abuse response; update unconfirmed; retry at "} {
		if value, ok := strings.CutPrefix(status, prefix); ok {
			deadline, err := time.Parse(time.RFC3339, value)
			return err == nil && deadline.UTC().Format(time.RFC3339) == value
		}
	}
	switch status {
	case "IPv4 detection failed (all sources exhausted)", "unchanged", "invalid provider request", "invalid provider response", "provider transport failure (DNS, connection, or TLS)", "provider rejected update or returned malformed success":
		return true
	}
	if httpFailureStatus.MatchString(status) {
		return true
	}
	if address, ok := strings.CutPrefix(status, "confirmed "); ok {
		ip := net.ParseIP(address)
		return ip != nil && ip.To4() != nil && ip.To4().String() == address && !ip.IsUnspecified() && !ip.IsMulticast()
	}
	if source, ok := strings.CutPrefix(status, "interface "); ok {
		quoted, err := strconv.QuotedPrefix(source)
		if err != nil {
			return false
		}
		iface, err := strconv.Unquote(quoted)
		return err == nil && iface != "" && fmt.Sprintf("%q IPv4 unavailable", iface) == source
	}
	return false
}
