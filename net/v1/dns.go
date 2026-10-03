/*
Copyright The k3sm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package netv1

import (
	"fmt"
	"net/netip"
	"strings"
	"unicode"
)

// DefaultNDots is the Kubernetes default ndots value: a query name with fewer
// than this many dots is tried against the search domains before being resolved
// as absolute. The getaddrinfo shim applies this when DNSConfig.NDots is zero.
const DefaultNDots = 5

// MaxNameservers is the most Nameservers a DNSPolicyNone config may carry. It
// matches the upstream kubelet's MaxDNSNameservers. The matching bound on
// search domains is deliberately not here: it lives in darwin-net, because it
// is a limit of the shim's ABI rather than a validation bound on the type.
const MaxNameservers = 3

// DNSPolicy selects how the in-pod resolver uses a DNSConfig. The empty string
// is the only spelling of ClusterFirst; corev1's "ClusterFirst" is not a valid
// value here and fails Validate.
type DNSPolicy string

const (
	// DNSPolicyClusterFirst is the zero value: queries go to ClusterDNSIP with
	// SearchDomains and NDots, and a name the cluster resolver does not answer
	// falls through to the host resolver.
	DNSPolicyClusterFirst DNSPolicy = ""
	// DNSPolicyNone is exclusive: queries go only to Nameservers, with no
	// cluster defaults and no fall-through to the host resolver.
	DNSPolicyNone DNSPolicy = "None"
)

// DNSOption is one resolv.conf "options" entry, e.g. {Name: "edns0"} or
// {Name: "timeout", Value: "2"}. It is lossy against corev1.PodDNSConfigOption,
// whose Value is a *string: here an empty Value always means a bare flag, so a
// present-but-empty value cannot be expressed.
type DNSOption struct {
	// Name is the option name. It must be non-empty, must not be "ndots" in any
	// case (DNSConfig.NDots is the one home of ndots), and must be a single
	// token: no whitespace, control characters, ':' or '='. A renderer joins
	// Name and Value as name[:value], so a separator inside Name would smuggle
	// in a second option (Name "ndots:1" would set ndots).
	Name string `json:"name"`
	// Value is the option value; empty means the option is a bare flag. It must
	// not contain whitespace or control characters.
	Value string `json:"value,omitempty"`
}

// DNSConfig is the in-pod DNS configuration the getaddrinfo DYLD shim consumes
// inside each pod (macOS getaddrinfo goes through mDNSResponder/configd and never
// reads /etc/resolv.conf, so the shim must carry this itself — see
// k3sm/docs/DESIGN.md §6). It is also the wiring the server uses to point pods at
// CoreDNS. It is a resolv.conf-equivalent expressed as Go data.
//
// Policy selects which fields are read:
//
//	field          ClusterFirst ("")      None
//	ClusterDNSIP   required               must be empty
//	ClusterDomain  required               optional, no policy meaning
//	SearchDomains  read                   read
//	NDots          read (0 = default)     read (0 = default)
//	Nameservers    must be empty          required, 1..MaxNameservers
//	Options        must be empty          read
//
// Servers returns the resolver addresses for either policy; consumers should
// use it rather than switching on Policy themselves.
//
// Validate checks syntax for any resolver (an IPv6 nameserver is valid), but
// the native getaddrinfo shim serves only IPv4 nameservers; a consumer feeding
// the shim must drop or reject IPv6 entries itself.
//
// ClusterDNSIP and ClusterDomain are not omitempty, so their wire shape is
// unchanged for existing readers; a None config therefore marshals with
// "clusterDNSIP":"". The new fields are omitempty, so a ClusterFirst config
// marshals to exactly the bytes it did before they existed.
type DNSConfig struct {
	// ClusterDNSIP is the IP of the in-cluster CoreDNS VIP the shim sends queries
	// to. It is the nameserver of last resort for cluster names; a single address
	// is sufficient because CoreDNS itself fans out upstream.
	ClusterDNSIP string `json:"clusterDNSIP"`
	// ClusterDomain is the cluster's DNS suffix, e.g. "cluster.local". Service and
	// pod A/AAAA records live under it (<svc>.<ns>.svc.<ClusterDomain>).
	ClusterDomain string `json:"clusterDomain"`
	// SearchDomains are appended to unqualified names in order (the resolv.conf
	// "search" list), e.g. <ns>.svc.cluster.local, svc.cluster.local,
	// cluster.local. Names with at least NDots dots skip the search list.
	SearchDomains []string `json:"searchDomains,omitempty"`
	// NDots is the resolv.conf "ndots" option: a name with fewer dots is tried
	// against SearchDomains first. Zero means use DefaultNDots; call WithDefaults
	// to materialize it.
	NDots int32 `json:"ndots,omitempty"`
	// Policy selects how the resolver uses this config. The zero value is
	// DNSPolicyClusterFirst.
	Policy DNSPolicy `json:"policy,omitempty"`
	// Nameservers are the resolver addresses for DNSPolicyNone, tried in order.
	// Each must be an IP address without a zone. Validate accepts IPv6, but the
	// native getaddrinfo shim serves only IPv4 nameservers.
	Nameservers []string `json:"nameservers,omitempty"`
	// Options are extra resolv.conf "options" entries for DNSPolicyNone.
	Options []DNSOption `json:"options,omitempty"`
}

// Servers returns the resolver addresses the config selects: ClusterDNSIP for
// DNSPolicyClusterFirst (nil when it is empty), a copy of Nameservers for
// DNSPolicyNone (nil when there are none), and nil for an unknown policy. The
// returned slice is never shared with the receiver.
func (c DNSConfig) Servers() []string {
	switch c.Policy {
	case DNSPolicyClusterFirst:
		if c.ClusterDNSIP == "" {
			return nil
		}
		return []string{c.ClusterDNSIP}
	case DNSPolicyNone:
		if len(c.Nameservers) == 0 {
			return nil
		}
		out := make([]string, len(c.Nameservers))
		copy(out, c.Nameservers)
		return out
	default:
		return nil
	}
}

// WithDefaults returns a copy of the config with NDots set to DefaultNDots when
// it is zero. It does not mutate the receiver.
func (c DNSConfig) WithDefaults() DNSConfig {
	out := c
	if out.NDots == 0 {
		out.NDots = DefaultNDots
	}
	return out
}

// Validate reports whether the config is well formed for its Policy. Errors wrap
// ErrInvalid.
//
// DNSPolicyClusterFirst ("") requires ClusterDNSIP and ClusterDomain and forbids
// Nameservers and Options, so the cluster resolver is never repointed. A zero
// NDots is allowed and means DefaultNDots; a negative one is invalid.
//
// DNSPolicyNone requires 1 to MaxNameservers Nameservers, each a valid IP
// address with no zone and none repeated (compared as parsed addresses, since a
// duplicate burns one of the few resolver slots for nothing), and requires
// ClusterDNSIP to be empty so no cluster default leaks into an exclusive config.
// ClusterDomain is optional.
//
// For both policies, every Options entry must have a non-empty name other than
// "ndots" (compared case-insensitively), the name must not contain ':' or '='
// (the name/value separators), and neither the name nor the value may contain
// whitespace or control characters, since each is written verbatim into a
// resolv.conf line. Any other Policy is invalid.
func (c DNSConfig) Validate() error {
	switch c.Policy {
	case DNSPolicyClusterFirst:
		if c.ClusterDNSIP == "" {
			return fmt.Errorf("%w: dns config missing clusterDNSIP", ErrInvalid)
		}
		if c.ClusterDomain == "" {
			return fmt.Errorf("%w: dns config missing clusterDomain", ErrInvalid)
		}
		if len(c.Nameservers) != 0 {
			return fmt.Errorf("%w: dns config policy ClusterFirst must not set nameservers", ErrInvalid)
		}
		if len(c.Options) != 0 {
			return fmt.Errorf("%w: dns config policy ClusterFirst must not set options", ErrInvalid)
		}
	case DNSPolicyNone:
		if err := validateNameservers(c.Nameservers); err != nil {
			return err
		}
		if c.ClusterDNSIP != "" {
			return fmt.Errorf("%w: dns config policy None must not set clusterDNSIP", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: dns config policy %q is unknown; the only values are %q (ClusterFirst) and %q",
			ErrInvalid, c.Policy, DNSPolicyClusterFirst, DNSPolicyNone)
	}
	if c.NDots < 0 {
		return fmt.Errorf("%w: dns config ndots %d is negative", ErrInvalid, c.NDots)
	}
	for i, o := range c.Options {
		if err := validateDNSOption(o); err != nil {
			return fmt.Errorf("dns config option %d: %w", i, err)
		}
	}
	return nil
}

// validateNameservers checks a DNSPolicyNone nameserver list: 1 to
// MaxNameservers entries, each a zoneless IP address, with no address repeated.
func validateNameservers(ns []string) error {
	if len(ns) == 0 {
		return fmt.Errorf("%w: dns config policy None requires at least one nameserver", ErrInvalid)
	}
	if len(ns) > MaxNameservers {
		return fmt.Errorf("%w: dns config has %d nameservers, at most %d allowed", ErrInvalid, len(ns), MaxNameservers)
	}
	seen := make(map[netip.Addr]struct{}, len(ns))
	for _, s := range ns {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return fmt.Errorf("%w: dns config nameserver %q is not an IP address", ErrInvalid, s)
		}
		if addr.Zone() != "" {
			return fmt.Errorf("%w: dns config nameserver %q must not carry a zone", ErrInvalid, s)
		}
		if _, dup := seen[addr]; dup {
			return fmt.Errorf("%w: dns config nameserver %q is repeated", ErrInvalid, s)
		}
		seen[addr] = struct{}{}
	}
	return nil
}

// validateDNSOption checks one resolv.conf option for a usable single-token name
// and for characters that could break out of its resolv.conf line or smuggle in
// a second option.
func validateDNSOption(o DNSOption) error {
	if o.Name == "" {
		return fmt.Errorf("%w: dns option name is empty", ErrInvalid)
	}
	if strings.EqualFold(o.Name, "ndots") {
		return fmt.Errorf("%w: dns option %q is not allowed; set ndots instead", ErrInvalid, o.Name)
	}
	if hasSpaceOrControl(o.Name) {
		return fmt.Errorf("%w: dns option name %q contains whitespace or a control character", ErrInvalid, o.Name)
	}
	if strings.ContainsAny(o.Name, ":=") {
		return fmt.Errorf("%w: dns option name %q contains a ':' or '=' separator", ErrInvalid, o.Name)
	}
	if hasSpaceOrControl(o.Value) {
		return fmt.Errorf("%w: dns option %q value %q contains whitespace or a control character", ErrInvalid, o.Name, o.Value)
	}
	return nil
}

// hasSpaceOrControl reports whether s contains any whitespace or control rune.
func hasSpaceOrControl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0
}
