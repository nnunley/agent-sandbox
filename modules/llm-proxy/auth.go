package main

import (
	"fmt"
	"net"
	"strings"
)

// anthropicOAuthBeta is the beta flag Anthropic requires on requests that
// authenticate with a Claude subscription's OAuth token.
const anthropicOAuthBeta = "oauth-2025-04-20"

// anthropicSpec builds the Anthropic route. An OAuth token, when configured,
// wins over an API key: the fleet runs on the subscription.
func anthropicSpec(apiKey, oauthToken string) routeSpec {
	s := routeSpec{prefix: "/anthropic", upstream: "https://api.anthropic.com", apiKey: apiKey, provider: "anthropic", requiresKey: true}
	if oauthToken != "" {
		s.apiKey, s.oauth = oauthToken, true
	}
	return s
}

// withBeta adds flag to a comma-separated anthropic-beta header value unless
// it is already there.
func withBeta(header, flag string) string {
	for _, f := range strings.Split(header, ",") {
		if strings.TrimSpace(f) == flag {
			return header
		}
	}
	if strings.TrimSpace(header) == "" {
		return flag
	}
	return header + "," + flag
}

// localSubnetsToken in an allowlist stands for every subnet the host is
// directly attached to: the Incus network the worker containers share and the
// microVM bridge.
const localSubnetsToken = "local-subnets"

// parseAllow parses a comma-separated list of CIDRs and the local-subnets
// token. An empty list is an error: a proxy nobody may use is a misconfiguration.
func parseAllow(spec string, local func() ([]*net.IPNet, error)) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		switch {
		case item == "":
			continue
		case item == localSubnetsToken:
			l, err := local()
			if err != nil {
				return nil, fmt.Errorf("allowlist: reading local subnets: %w", err)
			}
			nets = append(nets, l...)
		default:
			_, n, err := net.ParseCIDR(item)
			if err != nil {
				return nil, fmt.Errorf("allowlist: %q is not a CIDR or %q", item, localSubnetsToken)
			}
			nets = append(nets, n)
		}
	}
	if len(nets) == 0 {
		return nil, fmt.Errorf("allowlist %q names no networks", spec)
	}
	return nets, nil
}

// localSubnets returns the networks of the host's interface addresses,
// loopback included.
func localSubnets() ([]*net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var nets []*net.IPNet
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			nets = append(nets, &net.IPNet{IP: ipn.IP.Mask(ipn.Mask), Mask: ipn.Mask})
		}
	}
	return nets, nil
}

func loopbackNets() []*net.IPNet {
	_, v4, _ := net.ParseCIDR("127.0.0.0/8")
	_, v6, _ := net.ParseCIDR("::1/128")
	return []*net.IPNet{v4, v6}
}

// callerAllowed reports whether a request's RemoteAddr falls in the fleet
// allowlist. It reads the socket address only; forwarding headers are not trusted.
func (s *Server) callerAllowed(remoteAddr string) bool {
	return s.callerAllowedFor(remoteAddr, classFleet)
}

// callerAllowedFor checks RemoteAddr against the allowlist of the route's
// class: interactive routes skip the reserve cap, so they have their own.
func (s *Server) callerAllowedFor(remoteAddr string, class routeClass) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if class == classInteractive {
		return inNets(s.interactiveAllow, ip)
	}
	return inNets(s.allow, ip)
}

func inNets(nets []*net.IPNet, ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// anthropicPathAllowed reports whether a cleaned upstream path is one an
// agent needs: messages, token counting and the model list.
func anthropicPathAllowed(p string) bool {
	switch p {
	case "/v1/messages", "/v1/messages/count_tokens", "/v1/models":
		return true
	}
	return strings.HasPrefix(p, "/v1/models/")
}
