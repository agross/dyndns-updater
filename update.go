package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type dependencies struct {
	address func(string) (string, error)
	now     func() time.Time
	mail    func(Config, string) error
}

func productionDependencies() dependencies { return dependencies{interfaceIPv4, time.Now, submitMail} }
func interfaceIPv4(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", errors.New("interface unavailable")
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", errors.New("interface addresses unavailable")
	}
	for _, a := range addrs {
		ip, _, e := net.ParseCIDR(a.String())
		if e == nil && ip.To4() != nil && !ip.IsUnspecified() && !ip.IsMulticast() {
			return ip.To4().String(), nil
		}
	}
	return "", errors.New("interface has no IPv4 address")
}
func identity(p Profile) string {
	b, _ := json.Marshal([]string{p.Provider, p.Target, p.Interface, p.Endpoint})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func providerUpdate(c Config, p Profile, ip string, now func() time.Time) error {
	u, _ := url.Parse(p.Endpoint)
	q := u.Query()
	q.Set("hostname", p.Target)
	if p.Provider == "desec" {
		q.Set("myipv4", ip)
		q.Set("myipv6", "preserve")
	} else {
		q.Set("myip", ip)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return errors.New("invalid provider request")
	}
	req.Header.Set("User-Agent", "dyndns-updater/1 (one-shot)")
	if p.Provider == "desec" {
		req.Header.Set("Authorization", "Token "+p.Token)
	} else {
		req.SetBasicAuth(p.Username, p.Password)
	}
	client := &http.Client{Timeout: c.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("provider transport failure (DNS, connection, or TLS)")
	}
	defer resp.Body.Close()
	if p.Provider == "desec" {
		switch resp.StatusCode {
		case 401:
			return providerFailure{kind: "desec-auth"}
		case 400:
			return providerFailure{kind: "desec-hostname"}
		case 404:
			return providerFailure{kind: "desec-target"}
		case 429:
			return providerFailure{kind: "desec-rate", until: retryDeadline(resp.Header, now())}
		}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("provider HTTP failure (%d)", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(body) > 4096 {
		return errors.New("invalid provider response")
	}
	fields := strings.Fields(string(body))
	if p.Provider == "tunnelbroker" && strings.TrimSpace(string(body)) == "abuse" {
		return providerFailure{kind: "he-abuse"}
	}
	if p.Provider == "desec" {
		if len(fields) == 1 && fields[0] == "good" {
			return nil
		}
	} else if len(fields) == 2 && (fields[0] == "good" || fields[0] == "nochg") && fields[1] == ip {
		return nil
	}
	return errors.New("provider rejected update or returned malformed success")
}
func submitMail(c Config, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Sendmail, "-oi", "--", c.Recipient)
	cmd.Stdin = strings.NewReader("To: " + c.Recipient + "\nSubject: dyndns-updater profile changes\nContent-Type: text/plain; charset=UTF-8\n\n" + body)
	// Never forward MTA output: wrappers can echo their input or configuration.
	if cmd.Run() != nil {
		return errors.New("local sendmail submission failed")
	}
	return nil
}
func update(c Config, d dependencies, out io.Writer) error {
	lock, err := lockState(c.State)
	if err != nil {
		return err
	}
	defer lock.Close()
	s, err := readState(c.State)
	if err != nil {
		return err
	}
	// Reconcile only after validation and under the state lock. Cleanup is not a
	// notification event: preserve pending summaries and interrupted-run intent.
	configured := make(map[string]struct{}, len(c.Profiles))
	for _, p := range c.Profiles {
		configured[p.Name] = struct{}{}
	}
	for name := range s.Profiles {
		if _, ok := configured[name]; !ok {
			delete(s.Profiles, name)
		}
	}
	// Existing checkpoints and the final atomic write persist removals even when
	// every configured profile skips its update or has no available interface.
	results := []string{}
	notifications := []string{}
	var failures []error
	for _, p := range c.Profiles {
		old := s.Profiles[p.Name]
		id := identity(p)
		if old.Identity != id {
			old = profileState{Identity: id}
		}
		current := old
		ip, e := d.address(p.Interface)
		if parsed := net.ParseIP(ip); e != nil || parsed == nil || parsed.To4() == nil || parsed.IsUnspecified() || parsed.IsMulticast() {
			message := fmt.Sprintf("%s: interface %q IPv4 unavailable", renderProfileName(p.Name), p.Interface)
			results = append(results, message)
			notifications = append(notifications, fmt.Sprintf("%s: interface %q IPv4 unavailable", p.Name, p.Interface))
			failures = append(failures, errors.New(message))
			continue
		}
		fmt.Fprintf(out, "%s: interface %q detected IPv4 %s\n", renderProfileName(p.Name), p.Interface, ip)
		reason := ""
		switch {
		case s.Profiles[p.Name].Identity != "" && s.Profiles[p.Name].Identity != id:
			reason = "profile identity changed"
		case old.Failing:
			reason = "retrying previous failure"
		case old.Updated.IsZero():
			reason = "no confirmed state"
		case old.Address != ip:
			reason = "address changed"
		case d.now().Sub(old.Updated) >= c.refresh:
			reason = "refresh due"
		}
		status := "unchanged"
		blocked := blockedRetry(s, p, old, ip, d.now(), c.refresh)
		if blocked != nil && reason != "" {
			reason = "retry restricted"
		}
		if reason != "" {
			// Emit before the request so a slow provider does not hide the decision.
			// Keep operational traces out of durable notification summaries.
			current.Retry = nil
			if blocked != nil {
				fmt.Fprintf(out, "%s: skipping update: retry restricted\n", renderProfileName(p.Name))
				current.Retry = blocked
				e = providerFailure{kind: blocked.Kind, until: blocked.Until}
			} else {
				fmt.Fprintf(out, "%s: updating provider: %s\n", renderProfileName(p.Name), reason)
				e = providerUpdate(c, p, ip, d.now)
				var failure providerFailure
				if errors.As(e, &failure) {
					var retryErr error
					current.Retry, retryErr = newRetry(p, failure)
					if retryErr != nil {
						return retryErr
					}
					if failure.kind == "he-abuse" {
						// Owner decision #16 explicitly treats abuse as accepted,
						// while retaining failure exit/notification semantics.
						current.Address = ip
						current.Updated = d.now().UTC()
					}
				}
			}
			if e != nil {
				current.Failing = true
				status = e.Error()
				failures = append(failures, fmt.Errorf("%s: %s", renderProfileName(p.Name), status))
				if !old.Failing {
					s.Dirty = true
				}
			} else {
				current.Address = ip
				current.Updated = d.now().UTC()
				current.Failing = false
				status = "confirmed " + ip
				if old.Address != ip || old.Failing {
					s.Dirty = true
				}
			}
			s.Profiles[p.Name] = current
			// Commit independent outcomes and the notification intent together, before
			// proceeding to another provider. A crash leaves Dirty for summary recovery.
			if e := saveState(c.State, s); e != nil {
				return errors.Join(append(failures, e)...)
			}
		} else {
			fmt.Fprintf(out, "%s: skipping update: address unchanged and refresh not due\n", renderProfileName(p.Name))
		}
		results = append(results, renderProfileName(p.Name)+": "+status)
		notifications = append(notifications, p.Name+": "+status)
	}
	summary := strings.Join(results, "\n") + "\n"
	fmt.Fprint(out, summary)
	// These errors are already in the profile summary. Keep them in the returned
	// error for exit status and callers, without printing them a second time.
	for i, failure := range failures {
		failures[i] = reportedError{failure}
	}
	if s.Dirty {
		s.Pending = append(s.Pending, strings.Join(notifications, "\n")+"\n")
		s.Dirty = false
	}
	// Persist notification content before submission. Successful local acceptance
	// is recorded afterwards: a crash in between may duplicate, never lose, mail.
	if err := saveState(c.State, s); err != nil {
		return errors.Join(append(failures, err)...)
	}
	for len(s.Pending) > 0 {
		if err := d.mail(c, s.Pending[0]); err != nil {
			failures = append(failures, err)
			break
		}
		s.Pending = s.Pending[1:]
		if err := saveState(c.State, s); err != nil {
			return errors.Join(append(failures, err)...)
		}
	}
	return errors.Join(failures...)
}
