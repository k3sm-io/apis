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

import "fmt"

// DefaultNDots is the Kubernetes default ndots value: a query name with fewer
// than this many dots is tried against the search domains before being resolved
// as absolute. The getaddrinfo shim applies this when DNSConfig.NDots is zero.
const DefaultNDots = 5

// DNSConfig is the cluster-DNS configuration the getaddrinfo DYLD shim consumes
// inside each pod (macOS getaddrinfo goes through mDNSResponder/configd and never
// reads /etc/resolv.conf, so the shim must carry this itself — see
// k3sm/docs/DESIGN.md §6). It is also the wiring the server uses to point pods at
// CoreDNS. It is a resolv.conf-equivalent expressed as Go data.
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

// Validate reports whether the config is usable by the shim: a CoreDNS IP and a
// cluster domain must be set, and NDots (if specified) must be non-negative. A
// zero NDots is allowed and means DefaultNDots. Errors wrap ErrInvalid.
func (c DNSConfig) Validate() error {
	if c.ClusterDNSIP == "" {
		return fmt.Errorf("%w: dns config missing clusterDNSIP", ErrInvalid)
	}
	if c.ClusterDomain == "" {
		return fmt.Errorf("%w: dns config missing clusterDomain", ErrInvalid)
	}
	if c.NDots < 0 {
		return fmt.Errorf("%w: dns config ndots %d is negative", ErrInvalid, c.NDots)
	}
	return nil
}
