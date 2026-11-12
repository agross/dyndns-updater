package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Route the real HTTPS transport to a local fixture without changing URL, Host,
// SNI, trust verification or DNS lookup. Unknown destinations fail closed.
func namedDiscoveryFixture(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	cert := *srv.Certificate()
	cert.DNSNames = []string{"ifconfig.me", "checkip.dns.he.net", "api.ipify.org", "example.com"}
	key := srv.TLS.Certificates[0].PrivateKey.(crypto.Signer)
	der, err := x509.CreateCertificate(rand.Reader, &cert, &cert, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	srv.TLS.Certificates = []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	previous := http.DefaultTransport
	transport := previous.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "127.0.0.1:443" {
			host, _, err := net.SplitHostPort(address)
			if err != nil || host != "127.0.0.1" {
				return nil, fmt.Errorf("unexpected fixture destination")
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		if network != "tcp4" {
			return nil, fmt.Errorf("discovery did not select IPv4")
		}
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	http.DefaultTransport = transport
	t.Cleanup(func() { transport.CloseIdleConnections(); http.DefaultTransport = previous })
	return srv
}

// HE HTML shape verified directly from https://checkip.dns.he.net/; address
// replaced with a synthetic value. No network retrieval happens in the suite.
const heDiscoveryHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>What is my IP address?</title></head><body>Your IP address is : 8.8.4.4</body></html>`

func TestCLIDiscoveryNamedServices(t *testing.T) {
	for _, tc := range []struct {
		name, body, path, want string
		valid                  bool
	}{
		{"ifconfig.me", " \n8.8.4.4	", "/ip", "8.8.4.4", true},
		{"ipify", "8.8.4.4", "/", "8.8.4.4", true},
		{"he", heDiscoveryHTML, "/", "8.8.4.4", true},
		{"he", `<p>Other: 9.9.9.9</p><div>Your IP address is : <b>8.8.4.4</b></div>`, "/", "8.8.4.4", true},
		{"he", `<b>8.8.4.4</b>`, "/", "9.9.9.9", false},
		{"he", `Your IP address is : 8.8.4.4 Your IP address is : 9.9.9.9`, "/", "9.9.9.9", false},
		{"he", `Your IP address is : invalid <p>8.8.4.4</p>`, "/", "9.9.9.9", false},
	} {
		t.Run(tc.name+tc.body[:min(12, len(tc.body))], func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			localSystemDNS(t, system)
			calls := 0
			namedDiscoveryFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				expected := map[string]string{"ifconfig.me": "ifconfig.me", "ipify": "api.ipify.org", "he": "checkip.dns.he.net"}[tc.name]
				if r.Host != expected || r.TLS.ServerName != expected || r.URL.Path != tc.path || r.Header.Get("Authorization") != "" {
					t.Error("named endpoint/HTTPS/auth contract changed")
				}
				fmt.Fprint(w, tc.body)
			})
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("myipv4") != tc.want {
					t.Error("wrong detected address")
				}
				fmt.Fprint(w, "good")
			}))
			defer provider.Close()
			j := newJourney(t)
			j.ip = "9.9.9.9"
			j.write(sourceProfile("dns", provider.URL, "web:"+tc.name, "if:wan"))
			j.fallback()
			out := j.run(0)
			if calls != 1 || j.state().Profiles["dns"].Address != tc.want || strings.Contains(out, "https://") {
				t.Fatal("named discovery failed")
			}
			if tc.valid && strings.Contains(out, "source 2") {
				t.Fatal("discovery did not stop")
			}
		})
	}
}

func TestCLIDiscoveryMixedNamedAndCustom(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(fmt.Sprint(custom), func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			localSystemDNS(t, system)
			var hosts []string
			namedDiscoveryFixture(t, func(w http.ResponseWriter, r *http.Request) {
				hosts = append(hosts, r.Host)
				if r.Host == "ifconfig.me" {
					fmt.Fprint(w, "not an address")
					return
				}
				if r.Host == "checkip.dns.he.net" {
					fmt.Fprint(w, heDiscoveryHTML)
					return
				}
				t.Error("later service contacted")
			})
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "good") }))
			defer provider.Close()
			j := newJourney(t)
			j.ip = "9.9.9.9"
			source := "web:he"
			want := "8.8.4.4"
			if custom {
				source = "url:https://checkip.dns.he.net/"
				want = j.ip
			}
			j.write(sourceProfile("dns", provider.URL, "web:ifconfig.me", source, "if:wan", "web:ipify"))
			j.fallback()
			j.run(0)
			if strings.Join(hosts, ",") != "ifconfig.me,checkip.dns.he.net" || j.state().Profiles["dns"].Address != want {
				t.Fatal("custom HTML was not plain text, or named fallback failed")
			}
		})
	}
}
