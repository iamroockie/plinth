package plinth

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

type ipLookup int

const (
	ipMissing ipLookup = iota
	ipFound
	ipMalformed
)

const mappedIPv4PrefixBits = 96

// Errors returned by [ClientIPResolver.Resolve].
var (
	// ErrInvalidRemoteAddr means that the request's RemoteAddr is not an IP
	// address, for example on a Unix socket without
	// [ClientIPResolver.WithTrustedUnixSocket].
	ErrInvalidRemoteAddr = errors.New("invalid remote address")
	// ErrMalformedHeader means that a forwarding header has a hop that is not an
	// IP address in the part of the chain added by trusted proxies.
	ErrMalformedHeader = errors.New("malformed client IP header")
)

// ClientIPResolver finds the address of the client that sent a request. It uses
// forwarding headers only when the request comes from a trusted proxy, since
// anyone else can set them to any value.
//
// Each header is read from right to left: hops added by trusted proxies are
// skipped and the first untrusted address is the client. Addresses further left
// may be forged by the client and are never used. If every hop is trusted, the
// leftmost one is the client. A hop "unknown" means that a proxy did not know its
// client, so the header is skipped and the next one is tried.
//
// Create one with [NewClientIPResolver]: the zero value reads no headers, even
// after [ClientIPResolver.WithTrustedProxies]. A ClientIPResolver is immutable
// and safe for concurrent use.
type ClientIPResolver struct {
	trusted         []netip.Prefix
	headers         []string
	trustUnixSocket bool
}

// NewClientIPResolver returns a resolver that reads the given headers in order,
// or X-Forwarded-For if none are given. It trusts no proxies, and therefore
// ignores the headers, until [ClientIPResolver.WithTrustedProxies] is called.
func NewClientIPResolver(allowedHeaders ...string) ClientIPResolver {
	headers := []string{HeaderXForwardedFor}
	if len(allowedHeaders) > 0 {
		headers = slices.Clone(allowedHeaders)
	}
	return ClientIPResolver{headers: headers}
}

// WithTrustedProxies returns a copy of c that trusts the given proxies instead of
// the ones trusted before. Each proxy is an IP address or a CIDR prefix, such as
// "10.0.0.0/8" or "::1". IPv4-mapped IPv6 forms match the same IPv4 addresses. It
// returns an error if a proxy cannot be parsed.
func (c ClientIPResolver) WithTrustedProxies(proxies ...string) (ClientIPResolver, error) {
	trusted, err := parseProxies(proxies)
	if err != nil {
		return ClientIPResolver{}, err
	}
	c.trusted = trusted
	return c, nil
}

// WithTrustedUnixSocket returns a copy of c that trusts every peer when the
// server listens on a Unix socket, the usual setup for a reverse proxy on the same
// host. Without it, requests over a Unix socket fail with [ErrInvalidRemoteAddr].
func (c ClientIPResolver) WithTrustedUnixSocket() ClientIPResolver {
	c.trustUnixSocket = true
	return c
}

// Resolve returns the address of the client that sent r.
//
// For an untrusted peer it returns the peer address. For a trusted one it returns
// the address found in the headers, or the peer address if they have none. On a
// trusted Unix socket without a usable header the address is invalid and the
// error is nil.
//
// If RemoteAddr is not an IP address, Resolve returns an invalid address and
// [ErrInvalidRemoteAddr]. If a header is malformed, it returns the peer address
// together with [ErrMalformedHeader].
func (c ClientIPResolver) Resolve(r *http.Request) (netip.Addr, error) {
	if c.trustUnixSocket && isUnixSocket(r) {
		return c.peerFromHeaders(netip.Addr{}, r.Header)
	}

	peer, ok := parseIP(r.RemoteAddr)
	if !ok {
		return netip.Addr{}, fmt.Errorf("%w: %q", ErrInvalidRemoteAddr, r.RemoteAddr)
	}

	if !c.isTrusted(peer) {
		return peer, nil
	}

	return c.peerFromHeaders(peer, r.Header)
}

func (c ClientIPResolver) peerFromHeaders(peer netip.Addr, h http.Header) (netip.Addr, error) {
	for _, header := range c.headers {
		addr, lookup := c.peerFromHeader(header, h)
		switch lookup {
		case ipFound:
			return addr, nil
		case ipMalformed:
			return peer, fmt.Errorf("%w: %q", ErrMalformedHeader, header)
		case ipMissing:
		}
	}

	return peer, nil
}

func (c ClientIPResolver) isTrusted(addr netip.Addr) bool {
	return slices.ContainsFunc(c.trusted, func(prefix netip.Prefix) bool {
		return prefix.Contains(addr)
	})
}

func (c ClientIPResolver) peerFromHeader(header string, h http.Header) (netip.Addr, ipLookup) {
	var leftmost netip.Addr

	for _, hop := range slices.Backward(headerHops(header, h)) {
		if strings.EqualFold(hop, "unknown") {
			return netip.Addr{}, ipMissing
		}

		addr, ok := parseIP(hop)
		if !ok {
			return netip.Addr{}, ipMalformed
		}

		if !c.isTrusted(addr) {
			return addr, ipFound
		}

		leftmost = addr
	}

	if leftmost.IsValid() {
		return leftmost, ipFound
	}

	return netip.Addr{}, ipMissing
}

func isUnixSocket(r *http.Request) bool {
	_, ok := r.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr)
	return ok
}

func headerHops(header string, h http.Header) []string {
	var hops []string

	for _, value := range h.Values(header) {
		for hop := range strings.SplitSeq(value, ",") {
			if hop = strings.TrimSpace(hop); hop != "" {
				hops = append(hops, hop)
			}
		}
	}

	return hops
}

func parseIP(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)

	if addr, ok := parseBareIP(value); ok {
		return addr, true
	}

	if addr, ok := parseIPWithPort(value); ok {
		return addr, true
	}

	return parseBracketedIP(value)
}

func parseProxies(proxies []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(proxies))

	for _, proxy := range proxies {
		parsed, err := parseProxy(proxy)
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, parsed)
	}

	return prefixes, nil
}

func parseProxy(proxy string) (netip.Prefix, error) {
	value := strings.TrimSpace(proxy)
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		addr, addrErr := netip.ParseAddr(value)
		if addrErr != nil {
			if strings.Contains(value, "/") {
				return netip.Prefix{}, fmt.Errorf("invalid proxy %q: %w", proxy, err)
			}
			return netip.Prefix{}, fmt.Errorf("invalid proxy %q: %w", proxy, addrErr)
		}
		if addr.Zone() != "" {
			msg := "invalid proxy %q: IPv6 zones are not supported"
			return netip.Prefix{}, fmt.Errorf(msg, proxy)
		}
		addr = addr.Unmap()
		prefix = netip.PrefixFrom(addr, addr.BitLen())
	}

	if !prefix.Addr().Is4In6() {
		return prefix.Masked(), nil
	}

	if prefix.Bits() < mappedIPv4PrefixBits {
		msg := "invalid proxy %q: IPv4-mapped prefix shorter than /%d matches nothing"
		return netip.Prefix{}, fmt.Errorf(msg, proxy, mappedIPv4PrefixBits)
	}

	prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-mappedIPv4PrefixBits)

	return prefix.Masked(), nil
}

func parseBareIP(value string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return unzonedAddr(addr)
}

func parseIPWithPort(value string) (netip.Addr, bool) {
	addrPort, err := netip.ParseAddrPort(value)
	if err != nil {
		return netip.Addr{}, false
	}
	return unzonedAddr(addrPort.Addr())
}

func parseBracketedIP(value string) (netip.Addr, bool) {
	inner, ok := strings.CutPrefix(value, "[")
	if !ok {
		return netip.Addr{}, false
	}
	inner, ok = strings.CutSuffix(inner, "]")
	if !ok {
		return netip.Addr{}, false
	}
	return parseBareIP(inner)
}

func unzonedAddr(addr netip.Addr) (netip.Addr, bool) {
	if addr.Zone() != "" {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}
