// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package flags

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GRPCFlags struct {
	// client api flags
	address       StringValue
	token         StringValue
	tokenFile     StringValue
	caFile        StringValue
	caPath        StringValue
	certFile      StringValue
	keyFile       StringValue
	tlsServerName StringValue

	// server flags
	datacenter StringValue
	stale      BoolValue

	// multi-tenancy flags
	namespace StringValue
	partition StringValue
	peer      StringValue
}

func (f *GRPCFlags) ClientFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Var(&f.address, "grpc-addr",
		"The `address` and port of the Consul HTTP agent. The value can be an IP "+
			"address or DNS address, but it must also include the port. This can "+
			"also be specified via the CONSUL_HTTP_ADDR environment variable. The "+
			"default value is http://127.0.0.1:8500. The scheme can also be set to "+
			"HTTPS by setting the environment variable CONSUL_HTTP_SSL=true.")
	fs.Var(&f.token, "token",
		"ACL token to use in the request. This can also be specified via the "+
			"CONSUL_HTTP_TOKEN environment variable. If unspecified, the query will "+
			"default to the token of the Consul agent at the HTTP address.")
	fs.Var(&f.tokenFile, "token-file",
		"File containing the ACL token to use in the request instead of one specified "+
			"via the -token argument or CONSUL_HTTP_TOKEN environment variable. "+
			"This can also be specified via the CONSUL_HTTP_TOKEN_FILE environment variable.")
	fs.Var(&f.caFile, "ca-file",
		"Path to a CA file to use for TLS when communicating with Consul. This "+
			"can also be specified via the CONSUL_CACERT environment variable.")
	fs.Var(&f.caPath, "ca-path",
		"Path to a directory of CA certificates to use for TLS when communicating "+
			"with Consul. This can also be specified via the CONSUL_CAPATH environment variable.")
	fs.Var(&f.certFile, "client-cert",
		"Path to a client cert file to use for TLS when 'verify_incoming' is enabled. This "+
			"can also be specified via the CONSUL_CLIENT_CERT environment variable.")
	fs.Var(&f.keyFile, "client-key",
		"Path to a client key file to use for TLS when 'verify_incoming' is enabled. This "+
			"can also be specified via the CONSUL_CLIENT_KEY environment variable.")
	fs.Var(&f.tlsServerName, "tls-server-name",
		"The server name to use as the SNI host when connecting via TLS. This "+
			"can also be specified via the CONSUL_TLS_SERVER_NAME environment variable.")
	return fs
}

func (f *GRPCFlags) ServerFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Var(&f.datacenter, "datacenter",
		"Name of the datacenter to query. If unspecified, this will default to "+
			"the datacenter of the queried agent.")
	fs.Var(&f.stale, "stale",
		"Permit any Consul server (non-leader) to respond to this request. This "+
			"allows for lower latency and higher throughput, but can result in "+
			"stale data. This option has no effect on non-read operations. The "+
			"default value is false.")
	return fs
}

func (f *GRPCFlags) MultiTenancyFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Var(&f.namespace, "namespace",
		"Specifies the namespace to query. If not provided, the namespace will be inferred "+
			"from the request's ACL token, or will default to the `default` namespace. "+
			"Namespaces are a Consul Enterprise feature.")
	f.AddPartitionFlag(fs)
	return fs
}

func (f *GRPCFlags) PartitionFlag() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	f.AddPartitionFlag(fs)
	return fs
}
func (f *GRPCFlags) Addr() string {
	return f.address.String()
}

func (f *GRPCFlags) Datacenter() string {
	return f.datacenter.String()
}

func (f *GRPCFlags) Namespace() string {
	return f.namespace.String()
}

func (f *GRPCFlags) Partition() string {
	return f.partition.String()
}

func (f *GRPCFlags) PeerName() string {
	return f.peer.String()
}

func (f *GRPCFlags) Stale() bool {
	if f.stale.v == nil {
		return false
	}
	return *f.stale.v
}

func (f *GRPCFlags) Token() string {
	return f.token.String()
}

func (f *GRPCFlags) SetToken(v string) error {
	return f.token.Set(v)
}

func (f *GRPCFlags) TokenFile() string {
	return f.tokenFile.String()
}

func (f *GRPCFlags) SetTokenFile(v string) error {
	return f.tokenFile.Set(v)
}

func (f *GRPCFlags) ReadTokenFile() (string, error) {
	tokenFile := f.tokenFile.String()
	if tokenFile == "" {
		return "", nil
	}

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func defaultConfig(logger hclog.Logger) *api.Config {
	if logger == nil {
		logger = hclog.New(&hclog.LoggerOptions{
			Name: "consul-grpc",
		})
	}

	config := &api.Config{
		Address:   "127.0.0.1:8502",
		Scheme:    "grpc",
	}

	if addr := os.Getenv(api.GRPCAddrEnvName); addr != "" {
		config.Address = addr
	}

	// if tokenFile := os.Getenv(HTTPTokenFileEnvName); tokenFile != "" {
	// 	config.TokenFile = tokenFile
	// }

	// if token := os.Getenv(HTTPTokenEnvName); token != "" {
	// 	config.Token = token
	// }

	// if auth := os.Getenv(HTTPAuthEnvName); auth != "" {
	// 	var username, password string
	// 	if strings.Contains(auth, ":") {
	// 		split := strings.SplitN(auth, ":", 2)
	// 		username = split[0]
	// 		password = split[1]
	// 	} else {
	// 		username = auth
	// 	}

	// 	config.HttpAuth = &HttpBasicAuth{
	// 		Username: username,
	// 		Password: password,
	// 	}
	// }

	// if ssl := os.Getenv(HTTPSSLEnvName); ssl != "" {
	// 	enabled, err := strconv.ParseBool(ssl)
	// 	if err != nil {
	// 		logger.Warn(fmt.Sprintf("could not parse %s", HTTPSSLEnvName), "error", err)
	// 	}

	// 	if enabled {
	// 		config.Scheme = "https"
	// 	}
	// }

	// if v := os.Getenv(HTTPTLSServerName); v != "" {
	// 	config.TLSConfig.Address = v
	// }
	// if v := os.Getenv(GRPCCAFileEnvName); v != "" {
	// 	config.TLSConfig.CAFile = v
	// }
	// if v := os.Getenv(GRPCCAPathEnvName); v != "" {
	// 	config.TLSConfig.CAPath = v
	// }
	// if v := os.Getenv(HTTPClientCert); v != "" {
	// 	config.TLSConfig.CertFile = v
	// }
	// if v := os.Getenv(HTTPClientKey); v != "" {
	// 	config.TLSConfig.KeyFile = v
	// }
	// if v := os.Getenv(HTTPSSLVerifyEnvName); v != "" {
	// 	doVerify, err := strconv.ParseBool(v)
	// 	if err != nil {
	// 		logger.Warn(fmt.Sprintf("could not parse %s", HTTPSSLVerifyEnvName), "error", err)
	// 	}
	// 	if !doVerify {
	// 		config.TLSConfig.InsecureSkipVerify = true
	// 	}
	// }

	// if v := os.Getenv(HTTPNamespaceEnvName); v != "" {
	// 	config.Namespace = v
	// }

	// if v := os.Getenv(HTTPPartitionEnvName); v != "" {
	// 	config.Partition = v
	// }

	return config
}

func dial(addr string) (*grpc.ClientConn, error) {
	return grpc.Dial(string(addr),
		// TLS is handled in the dialer below.
		grpc.WithTransportCredentials(insecure.NewCredentials()),

		// This dialer negotiates a connection on the multiplexed server port using
		// our type-byte prefix scheme (see Server.handleConn for other side of it).
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", addr)
			if err != nil {
				fmt.Printf("**** in dialer error: %+v", err)
				return nil, err
			}

			// TODO: close connection at some point
			// conn.Close()
			return conn, nil
		}),
	)
}

func NewClient(config *api.Config) (*api.GRPCClient, error) {
	if config.GRPCClient == nil {
		var err error
		conn, err := dial(config.Address)
		if err != nil {
			fmt.Printf("**** error dialing grpc: %+v", err)
			return nil, err
		}
		config.GRPCClient = pbresource.NewResourceServiceClient(conn)
	}

	return &api.GRPCClient{Config: *config}, nil
}

func (f *GRPCFlags) GRPCClient() (*api.GRPCClient, error) {
	c := defaultConfig(nil)

	f.MergeOntoConfig(c)

	return NewClient(c)
}

func (f *GRPCFlags) MergeOntoConfig(c *api.Config) {
	f.address.Merge(&c.Address)
	f.token.Merge(&c.Token)
	f.tokenFile.Merge(&c.TokenFile)
	f.caFile.Merge(&c.TLSConfig.CAFile)
	f.caPath.Merge(&c.TLSConfig.CAPath)
	f.certFile.Merge(&c.TLSConfig.CertFile)
	f.keyFile.Merge(&c.TLSConfig.KeyFile)
	f.tlsServerName.Merge(&c.TLSConfig.Address)
	f.datacenter.Merge(&c.Datacenter)
	f.namespace.Merge(&c.Namespace)
	f.partition.Merge(&c.Partition)
}

func (f *GRPCFlags) AddPartitionFlag(fs *flag.FlagSet) {
	fs.Var(&f.partition, "partition",
		"Specifies the admin partition to query. If not provided, the admin partition will be inferred "+
			"from the request's ACL token, or will default to the `default` admin partition. "+
			"Admin Partitions are a Consul Enterprise feature.")
}

func (f *GRPCFlags) AddPeerName() *flag.FlagSet {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Var(&f.peer, "peer", "Specifies the name of peer to query. By default, it is `local`.")
	return fs
}
