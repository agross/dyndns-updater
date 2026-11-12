package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Source retains the configured selector and value, not resolved service URLs.
type Source struct {
	Selector string
	Value    string
}
type Sources []Source

func (s *Sources) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 {
		return errors.New("nonempty sources list required")
	}
	var result Sources
	for _, v := range n.Content {
		if v.Kind != yaml.MappingNode || len(v.Content) != 2 {
			return errors.New("source requires exactly one selector")
		}
		key, value := v.Content[0], v.Content[1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return errors.New("source selector and value must be strings")
		}
		source := Source{key.Value, value.Value}
		if _, _, err := parseSource(source); err != nil {
			return err
		}
		result = append(result, source)
	}
	*s = result
	return nil
}

func parseSource(source Source) (kind, argument string, err error) {
	kind, argument = source.Selector, source.Value
	invalid := errors.New("invalid address source")
	if strings.TrimSpace(argument) == "" || !validProfileName(argument) {
		return "", "", invalid
	}
	if kind == "if" {
		return kind, argument, nil
	}
	if kind == "web" {
		switch argument {
		case "ifconfig.me":
			return "plain", "https://ifconfig.me/ip", nil
		case "ipify":
			return "plain", "https://api.ipify.org/", nil
		case "he":
			return "he", "https://checkip.dns.he.net/", nil
		}
		return "", "", invalid
	}
	if kind != "url" {
		return "", "", invalid
	}
	u, e := url.Parse(argument)
	if e != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || strings.Contains(argument, "#") || u.Opaque != "" {
		return "", "", invalid
	}
	if port := u.Port(); port != "" {
		n, e := strconv.ParseUint(port, 10, 16)
		if e != nil || n == 0 {
			return "", "", invalid
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", "", invalid
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return "", "", invalid
	}
	return "plain", argument, nil
}

var nonPublicIPv4 = func() []netip.Prefix {
	var blocks []netip.Prefix
	for _, cidr := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	} {
		blocks = append(blocks, netip.MustParsePrefix(cidr))
	}
	return blocks
}()

func publicIPv4(value string) bool {
	ip, err := netip.ParseAddr(value)
	if err != nil || !ip.Is4() {
		return false
	}
	for _, block := range nonPublicIPv4 {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

func detectAddress(c Config, p Profile, d dependencies, out io.Writer) (string, error) {
	for i, source := range p.Sources {
		kind, argument, _ := parseSource(source) // all profiles validated before state locking
		fmt.Fprintf(out, "%s: source %d (%s) attempting IPv4 detection\n", p.Name, i+1, source.Selector)
		var ip string
		var err error
		if kind == "if" {
			ip, err = d.address(argument)
		} else {
			ip, err = discoverWeb(c, p.Name, kind, argument, out)
		}
		if err == nil && publicIPv4(ip) {
			if kind == "if" {
				fmt.Fprintf(out, "%s: interface %q detected IPv4 %s\n", p.Name, argument, ip)
			} else {
				fmt.Fprintf(out, "%s: source %d detected IPv4 %s\n", p.Name, i+1, ip)
			}
			return ip, nil
		}
		fmt.Fprintf(out, "%s: source %d failed IPv4 detection\n", p.Name, i+1)
	}
	return "", errors.New("IPv4 detection failed (all sources exhausted)")
}

var heHidden = regexp.MustCompile(`(?is)<!--.*?-->|<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>`)
var heTags = regexp.MustCompile(`<[^>]*>`)

func discoverWeb(c Config, name, kind, endpoint string, out io.Writer) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", errors.New("invalid discovery request")
	}
	req.Header.Set("User-Agent", "dyndns-updater/1 (one-shot)")
	transport, flush := diagnosticTransport(ctx, req.URL.Hostname(), c.FallbackDNS, name, out, true)
	defer flush()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: c.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("discovery transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("discovery HTTP failed")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(body) > 65536 {
		return "", errors.New("discovery body invalid")
	}
	ip := strings.TrimSpace(string(body))
	if kind == "he" {
		// Parse only the address immediately associated with the unique HE
		// marker. Never scan the document for an arbitrary IPv4 literal.
		text := heHidden.ReplaceAllString(string(body), " ")
		text = html.UnescapeString(heTags.ReplaceAllString(text, " "))
		const marker = "Your IP address is :"
		if strings.Count(text, marker) != 1 {
			return "", errors.New("discovery marker invalid")
		}
		_, suffix, _ := strings.Cut(text, marker)
		fields := strings.Fields(suffix)
		if len(fields) == 0 {
			return "", errors.New("discovery marker invalid")
		}
		ip = fields[0]
	}
	if !publicIPv4(ip) {
		return "", errors.New("discovery address invalid")
	}
	return ip, nil
}
