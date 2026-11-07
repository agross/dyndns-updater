package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Minimal local authoritative fixture. It handles A/AAAA, negative/empty
// answers, truncated UDP, TCP framing, and deliberately stalled responses.
type fakeDNS struct {
	address  string
	mode     atomic.Int32 // 0=answer, 1=NXDOMAIN, 2=empty, 3=truncate UDP, 4=stall, 5=SERVFAIL, 6=unusable
	delay    atomic.Int64
	udp, tcp atomic.Int32
}

func startDNS(t *testing.T, address string) *fakeDNS {
	t.Helper()
	var listener net.Listener
	var packet net.PacketConn
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		listener, err = net.Listen("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		packet, err = net.ListenPacket("udp", listener.Addr().String())
		if err == nil {
			break
		}
		listener.Close()
		// An ephemeral TCP port can already be occupied by an unrelated UDP
		// socket. Request a new pair rather than failing an offline test.
		if !strings.HasSuffix(address, ":0") {
			t.Fatal(err)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDNS{address: listener.Addr().String()}
	t.Cleanup(func() { packet.Close(); listener.Close() })
	answer := func(query []byte, tcp bool) []byte {
		if len(query) < 16 {
			return nil
		}
		mode := d.mode.Load()
		time.Sleep(time.Duration(d.delay.Load()))
		if mode == 4 {
			return nil
		}
		end := 12
		for end < len(query) && query[end] != 0 {
			end += int(query[end]) + 1
		}
		end += 5
		if end > len(query) {
			return nil
		}
		response := append([]byte(nil), query[:end]...)
		response[2] = 0x81
		response[3] = 0x80
		for i := 6; i < 12; i++ {
			response[i] = 0
		}
		if mode == 1 {
			response[3] = 0x83
			return response
		}
		if mode == 5 {
			response[3] = 0x82
			return response
		}
		if mode == 3 && !tcp {
			response[2] |= 2
			return response
		}
		if mode == 2 || binary.BigEndian.Uint16(query[end-4:end-2]) != 1 {
			return response
		}
		response[7] = 1
		ip := []byte{127, 0, 0, 1}
		if mode == 6 {
			ip = []byte{0, 0, 0, 0}
		}
		response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4)
		return append(response, ip...)
	}
	go func() {
		for {
			b := make([]byte, 2048)
			n, addr, err := packet.ReadFrom(b)
			if err != nil {
				return
			}
			d.udp.Add(1)
			if response := answer(b[:n], false); response != nil {
				packet.WriteTo(response, addr)
			}
		}
	}()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				var size [2]byte
				if _, err := io.ReadFull(conn, size[:]); err != nil {
					return
				}
				query := make([]byte, binary.BigEndian.Uint16(size[:]))
				if _, err := io.ReadFull(conn, query); err != nil {
					return
				}
				d.tcp.Add(1)
				response := answer(query, true)
				if response == nil {
					return
				}
				binary.BigEndian.PutUint16(size[:], uint16(len(response)))
				conn.Write(append(size[:], response...))
			}()
		}
	}()
	return d
}

func localSystemDNS(t *testing.T, d *fakeDNS) {
	t.Helper()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Resolver: &net.Resolver{}}).DialContext(ctx, network, d.address)
	}}
	t.Cleanup(func() { net.DefaultResolver = previous })
}

func trustedProvider(t *testing.T, handler http.HandlerFunc) (string, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	previous := http.DefaultTransport
	transport := previous.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	http.DefaultTransport = transport
	t.Cleanup(func() { transport.CloseIdleConnections(); http.DefaultTransport = previous })
	return strings.Replace(server.URL, "127.0.0.1", "example.com", 1), server
}
func (j *journey) fallback(servers ...string) {
	j.t.Helper()
	b, err := os.ReadFile(j.config)
	if err != nil {
		j.t.Fatal(err)
	}
	if len(servers) == 0 {
		b = append(b, []byte("fallback_dns: []\n")...)
	} else {
		b = append(b, []byte("fallback_dns:\n")...)
	}
	for _, server := range servers {
		b = append(b, []byte(fmt.Sprintf("  - %q\n", server))...)
	}
	if err := os.WriteFile(j.config, b, 0600); err != nil {
		j.t.Fatal(err)
	}
}

func TestCLIFallbackDNSOrderAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		system, first         int32
		status                int
		body                  string
		want                  int
		firstUsed, secondUsed bool
	}{
		{"system success", 0, 0, 200, "good", 0, false, false},
		{"first fallback", 1, 0, 200, "good", 0, true, false},
		{"later after negative", 1, 1, 200, "good", 0, true, true},
		{"later after empty", 2, 2, 200, "good", 0, true, true},
		{"later after servfail", 5, 5, 200, "good", 0, true, true},
		{"later after unusable", 6, 6, 200, "good", 0, true, true},
		{"tcp truncation", 1, 3, 200, "good", 0, true, false},
		{"http not DNS failure", 0, 0, 500, "good", 1, false, false},
		{"provider not DNS failure", 0, 0, 200, "bad", 1, false, false},
		{"fallback HTTP failure stops", 1, 0, 500, "good", 1, true, false},
		{"fallback provider failure stops", 1, 0, 200, "bad", 1, true, false},
		{"redirect not followed", 1, 0, 307, "good", 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			system.mode.Store(tc.system)
			localSystemDNS(t, system)
			first := startDNS(t, "127.0.0.1:0")
			first.mode.Store(tc.first)
			second := startDNS(t, "127.0.0.1:0")
			var calls atomic.Int32
			endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if !strings.HasPrefix(r.Host, "example.com:") || r.TLS.ServerName != "example.com" || r.URL.Query().Get("myipv6") != "preserve" || r.Header.Get("Authorization") != "Token synthetic-secret" {
					t.Error("hostname/TLS/provider contract changed")
				}
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			j := newJourney(t)
			j.write(profile("dns", "desec", endpoint))
			j.fallback(first.address, second.address)
			j.run(tc.want)
			if calls.Load() != 1 {
				t.Fatalf("provider requests=%d", calls.Load())
			}
			if (first.udp.Load() > 0) != tc.firstUsed || (second.udp.Load() > 0) != tc.secondUsed {
				t.Fatalf("fallback order: %d %d", first.udp.Load(), second.udp.Load())
			}
			if tc.first == 3 && first.tcp.Load() == 0 {
				t.Fatal("truncated UDP did not use TCP")
			}
		})
	}
}

func TestCLIFallbackFailureRecovery(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(1)
	localSystemDNS(t, system)
	resolver := startDNS(t, "127.0.0.1:0")
	var calls atomic.Int32
	endpoint, server := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") })
	j := newJourney(t)
	write := func() {
		j.write(profile("dns", "desec", endpoint) + profile("independent", "desec", server.URL))
		j.fallback(resolver.address)
	}
	write()
	j.run(0)
	confirmed := j.state().Profiles["dns"]
	resolver.mode.Store(1)
	j.ip = "192.0.2.11"
	j.run(1)
	j.run(1)
	failed := j.state().Profiles["dns"]
	if failed.Address != confirmed.Address || !failed.Updated.Equal(confirmed.Updated) || !failed.Failing || failed.Retry != nil || j.state().Profiles["independent"].Address != j.ip || len(j.mails) != 2 {
		t.Fatal("failure altered confirmation, retry, independent outcome or mail")
	}
	// Resolver configuration is not profile identity and does not erase state.
	resolver.mode.Store(0)
	write()
	j.run(0)
	j.run(0)
	if len(j.mails) != 3 || j.state().Profiles["dns"].Address != j.ip || calls.Load() != 4 {
		t.Fatalf("recovery/deduplication: mails=%d calls=%d", len(j.mails), calls.Load())
	}
	// Even a new, failing resolver list must not invalidate confirmed identity.
	before := j.state().Profiles["dns"]
	j.write(profile("dns", "desec", endpoint) + profile("independent", "desec", server.URL))
	j.fallback(system.address)
	j.run(0)
	if after := j.state().Profiles["dns"]; after.Identity != before.Identity || !after.Updated.Equal(before.Updated) || calls.Load() != 4 || len(j.mails) != 3 {
		t.Fatal("resolver-only configuration change invalidated confirmed state")
	}
}

func TestCLIFallbackDeadline(t *testing.T) {
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(1)
	localSystemDNS(t, system)
	slow := startDNS(t, "127.0.0.1:0")
	slow.mode.Store(4)
	later := startDNS(t, "127.0.0.1:0")
	var calls atomic.Int32
	endpoint, _ := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, "good") })
	j := newJourney(t)
	j.write(profile("dns", "desec", endpoint))
	j.fallback(slow.address, later.address)
	start := time.Now()
	j.run(1)
	if time.Since(start) > time.Second || slow.udp.Load() == 0 || later.udp.Load() != 0 || calls.Load() != 0 {
		t.Fatal("request deadline did not bound DNS chain")
	}
	before := slow.udp.Load()
	time.Sleep(150 * time.Millisecond)
	if slow.udp.Load() != before {
		t.Fatal("DNS work continued after cancellation")
	}
}

func TestCLIFallbackTLSValidationAndConnectionFailure(t *testing.T) {
	for _, kind := range []string{"hostname", "untrusted", "connection"} {
		t.Run(kind, func(t *testing.T) {
			system := startDNS(t, "127.0.0.1:0")
			localSystemDNS(t, system)
			fallback := startDNS(t, "127.0.0.1:0")
			endpoint, server := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("request reached provider with invalid transport")
			})
			if kind == "hostname" {
				endpoint = strings.Replace(endpoint, "example.com", "wrong.invalid", 1)
			}
			if kind == "untrusted" {
				http.DefaultTransport.(*http.Transport).TLSClientConfig.RootCAs = x509.NewCertPool()
			}
			if kind == "connection" {
				server.Close()
			}
			j := newJourney(t)
			j.write(profile("dns", "desec", endpoint))
			j.fallback(fallback.address)
			j.run(1)
			if fallback.udp.Load() != 0 {
				t.Fatal("fallback after non-DNS failure")
			}
		})
	}
}

func TestCLIFallbackIPv6ResolverAndLiteralEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("IPv6 loopback unavailable")
	}
	listener.Close()
	resolver := startDNS(t, "[::1]:0")
	system := startDNS(t, "127.0.0.1:0")
	system.mode.Store(1)
	localSystemDNS(t, system)
	endpoint, server := trustedProvider(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "good") })
	j := newJourney(t)
	j.write(profile("dns", "desec", endpoint))
	j.fallback(resolver.address)
	j.run(0)
	count := resolver.udp.Load()
	if count == 0 {
		t.Fatal("IPv6 resolver unused")
	}
	j.write(profile("literal", "desec", server.URL))
	j.fallback(resolver.address)
	j.run(0)
	if resolver.udp.Load() != count {
		t.Fatal("literal endpoint triggered DNS")
	}
}
