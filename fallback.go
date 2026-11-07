package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Normalize once, before acquiring state locks or making external requests.
func resolverAddress(value string) (string, error) {
	if ip := net.ParseIP(value); ip != nil {
		return net.JoinHostPort(ip.String(), "53"), nil
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || net.ParseIP(host) == nil {
		return "", errors.New("invalid resolver")
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return "", errors.New("invalid port")
	}
	return net.JoinHostPort(host, strconv.FormatUint(n, 10)), nil
}

func usableAddresses(addresses []net.IPAddr) []net.IPAddr {
	result := make([]net.IPAddr, 0, len(addresses))
	for _, address := range addresses {
		if address.IP != nil && !address.IP.IsUnspecified() && !address.IP.IsMulticast() {
			result = append(result, address)
		}
	}
	return result
}

// Overridden only by nonparallel tests to route public resolver addresses to
// local DNS fixtures without sending traffic outside the test host.
var fallbackDNSDial = (&net.Dialer{Resolver: &net.Resolver{}}).DialContext

type dnsLookupEvent int

const (
	dnsLookupStarted dnsLookupEvent = iota
	dnsLookupFailed
	dnsLookupSucceeded
)

func lookupProvider(ctx context.Context, host string, servers []string, report func(string, dnsLookupEvent)) ([]net.IPAddr, error) {
	// DNS workers may finish asynchronously after request cancellation. Capture
	// the dial function before starting them, including when tests replace it.
	dialDNS := fallbackDNSDial
	report("system resolver", dnsLookupStarted)
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if addresses = usableAddresses(addresses); err == nil && len(addresses) > 0 && ctx.Err() == nil {
		report("system resolver", dnsLookupSucceeded)
		return addresses, nil
	}
	report("system resolver", dnsLookupFailed)
	for _, server := range servers {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			// Respect Go's UDP/TCP selection, including TCP after truncated UDP.
			return dialDNS(ctx, network, server)
		}}
		report(fmt.Sprintf("fallback resolver %q", server), dnsLookupStarted)
		addresses, err = resolver.LookupIPAddr(ctx, host)
		if addresses = usableAddresses(addresses); err == nil && len(addresses) > 0 && ctx.Err() == nil {
			report(fmt.Sprintf("fallback resolver %q", server), dnsLookupSucceeded)
			return addresses, nil
		}
		report(fmt.Sprintf("fallback resolver %q", server), dnsLookupFailed)
	}
	return nil, errors.New("provider DNS lookup failed")
}

// Keep URL, Host, SNI, proxy policy and certificate checks untouched. Only the
// destination dial for the provider hostname is replaced, not proxy resolution.
func fallbackTransport(ctx context.Context, host string, servers []string, report func(string, dnsLookupEvent)) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	originalDial := transport.DialContext
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		name, port, err := net.SplitHostPort(address)
		if err != nil || name != host || net.ParseIP(name) != nil {
			return originalDial(dialCtx, network, address)
		}
		// HTTP transport detaches dial cancellation to support connection reuse.
		// This transport serves only one request: bind its DNS/dial to that request.
		dialCtx, cancel := context.WithCancel(dialCtx)
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		defer cancel()
		if deadline, ok := ctx.Deadline(); ok {
			var deadlineCancel context.CancelFunc
			dialCtx, deadlineCancel = context.WithDeadline(dialCtx, deadline)
			defer deadlineCancel()
		}
		addresses, err := lookupProvider(dialCtx, name, servers, report)
		if err != nil {
			return nil, err
		}
		for i, ip := range addresses {
			// A failed connection must not trigger another DNS lookup. Share the
			// remaining budget among returned addresses, including mixed IPv4/IPv6.
			attemptCtx := dialCtx
			attemptCancel := func() {}
			if deadline, ok := dialCtx.Deadline(); ok {
				attemptCtx, attemptCancel = context.WithTimeout(dialCtx, time.Until(deadline)/time.Duration(len(addresses)-i))
			}
			conn, dialErr := originalDial(attemptCtx, network, net.JoinHostPort(ip.String(), port))
			attemptCancel()
			if dialErr == nil {
				return conn, nil
			}
			err = dialErr
			if dialCtx.Err() != nil {
				break
			}
		}
		return nil, err
	}
	return transport
}
