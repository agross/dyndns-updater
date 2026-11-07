package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	State            string   `yaml:"state"`
	Refresh          string   `yaml:"refresh"`
	Timeout          string   `yaml:"timeout"`
	Sendmail         string   `yaml:"sendmail"`
	Recipient        string   `yaml:"recipient"`
	Profiles         Profiles `yaml:"profiles"`
	FallbackDNS      []string `yaml:"fallback_dns"`
	refresh, timeout time.Duration
}
type Profile struct {
	Name      string `yaml:"-"`
	Provider  string `yaml:"provider"`
	Interface string `yaml:"interface"`
	Target    string `yaml:"target"`
	Token     string `yaml:"token"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Endpoint  string `yaml:"endpoint"`
}

var hostName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func loadConfig(path string) (Config, error) {
	c := Config{State: "/var/lib/dyndns-updater/state.json", Refresh: "600h", Timeout: "30s", Sendmail: "/usr/sbin/sendmail", Recipient: "root"}
	f, err := os.Open(path)
	if err != nil {
		return c, errors.New("cannot read configuration")
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(content) > 1<<20 {
		return c, errors.New("configuration unreadable or exceeds 1 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		if errors.Is(err, errProfilesMapping) {
			return c, errProfilesMapping
		}
		return c, errors.New("invalid YAML configuration")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return c, errors.New("configuration must contain one document")
	}
	c.refresh, err = time.ParseDuration(c.Refresh)
	if err != nil || c.refresh <= 0 {
		return c, errors.New("refresh must be a positive Go duration")
	}
	c.timeout, err = time.ParseDuration(c.Timeout)
	if err != nil || c.timeout <= 0 {
		return c, errors.New("timeout must be a positive Go duration")
	}
	if !filepath.IsAbs(c.State) || !filepath.IsAbs(c.Sendmail) || len(c.Profiles) == 0 || strings.ContainsAny(c.Recipient, "\r\n\x00") || c.Recipient == "" || strings.HasPrefix(c.Recipient, "-") {
		return c, errors.New("invalid state, sendmail, recipient, or empty profiles")
	}
	for i, server := range c.FallbackDNS {
		address, err := resolverAddress(server)
		if err != nil {
			return c, errors.New("invalid fallback DNS address (literal IP with optional port required)")
		}
		c.FallbackDNS[i] = address
	}
	seen := map[string]bool{}
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if !validProfileName(p.Name) || seen[p.Name] || p.Interface == "" || strings.ContainsAny(p.Interface, "\r\n\x00") || !hostName.MatchString(p.Target) {
			return c, errors.New("invalid profile name, target, or interface")
		}
		seen[p.Name] = true
		switch p.Provider {
		case "desec":
			if p.Token == "" || strings.ContainsAny(p.Token, "\r\n\x00") {
				return c, errors.New("invalid deSEC credentials")
			}
			if p.Endpoint == "" {
				p.Endpoint = "https://update.dedyn.io/"
			}
		case "tunnelbroker":
			if p.Username == "" || p.Password == "" || strings.ContainsAny(p.Username+p.Password, "\r\n\x00") || strings.Contains(p.Username, ":") {
				return c, errors.New("invalid Tunnelbroker credentials")
			}
			if p.Endpoint == "" {
				p.Endpoint = "https://ipv4.tunnelbroker.net/nic/update"
			}
		default:
			return c, errors.New("unsupported provider")
		}
		u, e := url.Parse(p.Endpoint)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, errors.New("invalid provider endpoint")
		}
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
			return c, errors.New("provider endpoint requires HTTPS (HTTP allowed only for literal loopback tests)")
		}
	}
	return c, nil
}
