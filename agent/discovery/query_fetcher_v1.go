// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package discovery

import (
	"context"
	"github.com/hashicorp/consul/agent/cache"
	"github.com/hashicorp/consul/agent/structs"
	libdns "github.com/hashicorp/consul/internal/dnsutil"
	"github.com/hashicorp/go-hclog"
	"github.com/miekg/dns"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/consul/agent/config"
)

const (
	// TODO (v2-dns): can we move the recursion into the data fetcher?
	maxRecursionLevelDefault = 3 // This field comes from the V1 DNS server and affects V1 catalog lookups
	maxRecurseRecords        = 5
)

type v1DataFetcherDynamicConfig struct {
	allowStale  bool
	maxStale    time.Duration
	useCache    bool
	cacheMaxAge time.Duration
	onlyPassing bool
	domain      string
	altDomain   string
	nodeTTL     time.Duration
}

type V1DataFetcher struct {
	dynamicConfig       atomic.Value
	getServiceNodesFunc func(ctx context.Context, req structs.ServiceSpecificRequest) (structs.IndexedCheckServiceNodes, cache.ResultMeta, error)
	rpc                 func(ctx context.Context, method string, args interface{}, reply interface{}) error
	getTokenFunc        func() string

	logger hclog.Logger
}

func NewV1DataFetcher(config *config.RuntimeConfig, rpc func(ctx context.Context,
	method string, args interface{}, reply interface{}) error,
	getServiceNodesFunc func(ctx context.Context, req structs.ServiceSpecificRequest) (structs.IndexedCheckServiceNodes, cache.ResultMeta, error),
	getTokenFunc func() string, logger hclog.Logger) *V1DataFetcher {
	f := &V1DataFetcher{
		rpc:                 rpc,
		getServiceNodesFunc: getServiceNodesFunc,
		getTokenFunc:        getTokenFunc,
		logger:              logger,
	}
	f.LoadConfig(config)
	return f
}

func (f *V1DataFetcher) LoadConfig(config *config.RuntimeConfig) {
	dynamicConfig := &v1DataFetcherDynamicConfig{
		allowStale:  config.DNSAllowStale,
		maxStale:    config.DNSMaxStale,
		useCache:    config.DNSUseCache,
		cacheMaxAge: config.DNSCacheMaxAge,
		onlyPassing: config.DNSOnlyPassing,
		domain:      config.DNSDomain,
		altDomain:   config.DNSAltDomain,
		nodeTTL:     config.DNSNodeTTL,
	}
	f.dynamicConfig.Store(dynamicConfig)
}

// TODO (v2-dns): Implementation of the V1 data fetcher

func (f *V1DataFetcher) FetchNodes(ctx Context, req *QueryPayload) ([]*Result, error) {
	return nil, nil
}

func (f *V1DataFetcher) FetchEndpoints(ctx Context, req *QueryPayload, lookupType LookupType) ([]*Result, error) {
	var results []*Result

	serviceTags := []string{}
	if req.Tag != "" {
		serviceTags = []string{req.Tag}
	}

	cfg := f.dynamicConfig.Load().(*v1DataFetcherDynamicConfig)
	args := structs.ServiceSpecificRequest{
		PeerName:    req.Tenancy.Peer,
		Connect:     lookupType == LookupTypeConnect,
		Ingress:     lookupType == LookupTypeConnect,
		Datacenter:  req.Tenancy.Datacenter,
		ServiceName: req.Name,
		ServiceTags: serviceTags,
		TagFilter:   req.Tag != "",
		QueryOptions: structs.QueryOptions{
			Token:            f.getTokenFunc(),
			AllowStale:       cfg.allowStale,
			MaxAge:           cfg.cacheMaxAge,
			UseCache:         cfg.useCache,
			MaxStaleDuration: cfg.maxStale,
		},
		EnterpriseMeta: getRequestEnterpriseMetaFromQueryPayload(req),
	}

	out, _, err := f.getServiceNodesFunc(context.TODO(), args)
	if err != nil {
		return nil, err
	}

	// Filter out any service nodes due to health checks
	// We copy the slice to avoid modifying the result if it comes from the cache
	nodes := make(structs.CheckServiceNodes, len(out.Nodes))
	copy(nodes, out.Nodes)
	out.Nodes = nodes.Filter(cfg.onlyPassing)

	if err != nil {
		f.logger.Warn("Unable to get list of servers", "error", err)
		return nil, nil
	}

	if len(out.Nodes) == 0 {
		f.logger.Warn("no servers found")
		return nil, nil
	}

	// shuffle the nodes to randomize the output
	out.Nodes.Shuffle()

	for _, o := range out.Nodes {
		if libdns.InvalidNameRe.MatchString(o.Node.Node) {
			f.logger.Warn("Skipping invalid node for NS records", "node", o.Node.Node)
			continue
		}
		results = append(results, convertCheckServiceNodeToResults(o, cfg))

		// don't provide more than 3 servers
		if len(results) >= 3 {
			return results, nil
		}
	}

	return results, nil
}

func (f *V1DataFetcher) FetchVirtualIP(ctx Context, req *QueryPayload) (*Result, error) {
	return nil, nil
}

func (f *V1DataFetcher) FetchRecordsByIp(ctx Context, ip net.IP) ([]*Result, error) {
	return nil, nil
}

func (f *V1DataFetcher) FetchWorkload(ctx Context, req *QueryPayload) (*Result, error) {
	return nil, nil
}

func (f *V1DataFetcher) FetchPreparedQuery(ctx Context, req *QueryPayload) ([]*Result, error) {
	return nil, nil
}

func convertCheckServiceNodeToResults(checkServiceNode structs.CheckServiceNode, cfg *v1DataFetcherDynamicConfig) *Result {
	name, dc := checkServiceNode.Node.Node, checkServiceNode.Node.Datacenter
	respDomain := getResponseDomain(name, cfg.domain, cfg.altDomain)
	fqdn := name + ".node." + dc + "." + respDomain
	fqdn = dns.Fqdn(strings.ToLower(fqdn))

	// TODO (v2-dns): This is a stub implementation
	return &Result{
		Address:  "",
		Weight:   0,
		Port:     0,
		TTL:      0,
		Metadata: nil,
		Target:   "",
	}
}

// getResponseDomain returns alt-domain if it is configured and request is made with alt-domain,
// respects DNS case insensitivity
func getResponseDomain(questionName, domain, altDomain string) string {
	labels := dns.SplitDomainName(questionName)
	d := domain
	for i := len(labels) - 1; i >= 0; i-- {
		currentSuffix := strings.Join(labels[i:], ".") + "."
		if strings.EqualFold(currentSuffix, domain) || strings.EqualFold(currentSuffix, altDomain) {
			d = currentSuffix
		}
	}
	return d
}

// Craft dns records for a node
// In case of an SRV query the answer will be a IN SRV and additional data will store an IN A to the node IP
// Otherwise it will return a IN A record
func (d *DNSServer) makeRecordFromNode(node *structs.Node, qType uint16, qName string, ttl time.Duration, maxRecursionLevel int) []dns.RR {
	addrTranslate := TranslateAddressAcceptDomain
	if qType == dns.TypeA {
		addrTranslate |= TranslateAddressAcceptIPv4
	} else if qType == dns.TypeAAAA {
		addrTranslate |= TranslateAddressAcceptIPv6
	} else {
		addrTranslate |= TranslateAddressAcceptAny
	}

	addr := d.agent.TranslateAddress(node.Datacenter, node.Address, node.TaggedAddresses, addrTranslate)
	ip := net.ParseIP(addr)

	var res []dns.RR

	if ip == nil {
		res = append(res, &dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   qName,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    uint32(ttl / time.Second),
			},
			Target: dns.Fqdn(node.Address),
		})

		res = append(res,
			d.resolveCNAME(d.config.Load().(*dnsConfig), dns.Fqdn(node.Address), maxRecursionLevel)...,
		)

		return res
	}

	ipRecord := makeARecord(qType, ip, ttl)
	if ipRecord == nil {
		return nil
	}

	ipRecord.Header().Name = qName
	return []dns.RR{ipRecord}
}

func makeARecord(qType uint16, ip net.IP, ttl time.Duration) dns.RR {

	var ipRecord dns.RR
	ipv4 := ip.To4()
	if ipv4 != nil {
		if qType == dns.TypeSRV || qType == dns.TypeA || qType == dns.TypeANY || qType == dns.TypeNS || qType == dns.TypeTXT {
			ipRecord = &dns.A{
				Hdr: dns.RR_Header{
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    uint32(ttl / time.Second),
				},
				A: ipv4,
			}
		}
	} else if qType == dns.TypeSRV || qType == dns.TypeAAAA || qType == dns.TypeANY || qType == dns.TypeNS || qType == dns.TypeTXT {
		ipRecord = &dns.AAAA{
			Hdr: dns.RR_Header{
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    uint32(ttl / time.Second),
			},
			AAAA: ip,
		}
	}
	return ipRecord
}

// resolveCNAME is used to recursively resolve CNAME records
func (d *DNSServer) resolveCNAME(cfg *dnsConfig, name string, maxRecursionLevel int) []dns.RR {
	// If the CNAME record points to a Consul address, resolve it internally
	// Convert query to lowercase because DNS is case insensitive; d.domain and
	// d.altDomain are already converted

	if ln := strings.ToLower(name); strings.HasSuffix(ln, "."+d.domain) || strings.HasSuffix(ln, "."+d.altDomain) {
		if maxRecursionLevel < 1 {
			d.logger.Error("Infinite recursion detected for name, won't perform any CNAME resolution.", "name", name)
			return nil
		}
		req := &dns.Msg{}
		resp := &dns.Msg{}

		req.SetQuestion(name, dns.TypeANY)
		// TODO: handle error response
		d.dispatch(nil, req, resp, maxRecursionLevel-1)

		return resp.Answer
	}

	// Do nothing if we don't have a recursor
	if len(cfg.Recursors) == 0 {
		return nil
	}

	// Ask for any A records
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)

	// Make a DNS lookup request
	c := &dns.Client{Net: "udp", Timeout: cfg.RecursorTimeout}
	var r *dns.Msg
	var rtt time.Duration
	var err error
	for _, idx := range cfg.RecursorStrategy.Indexes(len(cfg.Recursors)) {
		recursor := cfg.Recursors[idx]
		r, rtt, err = c.Exchange(m, recursor)
		if err == nil {
			d.logger.Debug("cname recurse RTT for name",
				"name", name,
				"rtt", rtt,
			)
			return r.Answer
		}
		d.logger.Error("cname recurse failed for name",
			"name", name,
			"error", err,
		)
	}
	d.logger.Error("all resolvers failed for name", "name", name)
	return nil
}
