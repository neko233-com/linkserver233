package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// blockedHostnames are never allowed as redirect targets regardless of DNS.
var blockedHostnames = map[string]struct{}{
	"localhost":                {},
	"ip6-localhost":            {},
	"ip6-loopback":             {},
	"metadata":                 {},
	"metadata.google":          {},
	"metadata.google.internal": {},
}

// ValidateTargetHost rejects hosts that point at loopback, private, link-local,
// or otherwise internal addresses to mitigate open-redirect and SSRF abuse.
//
// When allowPrivate is true the check is skipped (useful for trusted internal
// deployments). DNS names are checked against a deny list and, when they are IP
// literals, against reserved ranges; full DNS resolution is intentionally not
// performed here.
func ValidateTargetHost(host string, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}

	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.TrimSuffix(strings.TrimPrefix(hostname, "["), "]")
	hostname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")

	if hostname == "" {
		return fmt.Errorf("target host is empty")
	}

	if _, blocked := blockedHostnames[hostname]; blocked {
		return fmt.Errorf("target host %q is not allowed", hostname)
	}
	if strings.HasSuffix(hostname, ".localhost") {
		return fmt.Errorf("target host %q is not allowed", hostname)
	}

	if ip := net.ParseIP(hostname); ip != nil {
		if isInternalIP(ip) {
			return fmt.Errorf("target host %q resolves to a private address", hostname)
		}
	}

	return nil
}

// ValidateTargetURL parses and validates a redirect target URL.
func ValidateTargetURL(raw string, allowPrivate bool) (string, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid target_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("target_url must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("target_url must include a host")
	}
	if err := ValidateTargetHost(parsed.Host, allowPrivate); err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func isInternalIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	// Carrier-grade NAT 100.64.0.0/10 and IPv4 benchmarking 198.18.0.0/15.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1]&0xc0 == 0x40 {
			return true
		}
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return true
		}
	}

	return false
}
