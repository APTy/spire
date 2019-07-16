package main

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"sync"

	spiffe_tls "github.com/spiffe/go-spiffe/tls"
	"github.com/spiffe/spire/pkg/common/idutil"
	"github.com/spiffe/spire/proto/spire/api/registration"
	"github.com/spiffe/spire/proto/spire/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type registrationClient interface {
	CreateEntry(context.Context, *common.RegistrationEntry, ...grpc.CallOption) (*registration.RegistrationEntryID, error)
	DeleteEntry(context.Context, *registration.RegistrationEntryID, ...grpc.CallOption) (*common.RegistrationEntry, error)
	FetchBundle(context.Context, *common.Empty, ...grpc.CallOption) (*registration.Bundle, error)
}

type multiClient struct {
	sync.Mutex

	cfg     config
	i       int
	clients []registration.RegistrationClient
}

func (c *multiClient) inc() {
	if c.i < len(c.clients)-1 {
		c.i += 1
	} else {
		c.i = 0
	}
}

func (c *multiClient) FetchBundle(ctx context.Context, in *common.Empty, opts ...grpc.CallOption) (*registration.Bundle, error) {
	c.Lock()
	client := c.clients[c.i]
	c.Unlock()
	return client.FetchBundle(ctx, in, opts...)
}

func (c *multiClient) CreateEntry(ctx context.Context, in *common.RegistrationEntry, opts ...grpc.CallOption) (*registration.RegistrationEntryID, error) {
	c.Lock()
	client := c.clients[c.i]
	c.inc()
	c.Unlock()
	return client.CreateEntry(ctx, in, opts...)
}
func (c *multiClient) DeleteEntry(ctx context.Context, in *registration.RegistrationEntryID, opts ...grpc.CallOption) (*common.RegistrationEntry, error) {
	c.Lock()
	client := c.clients[c.i]
	c.inc()
	c.Unlock()
	return client.DeleteEntry(ctx, in, opts...)
}

func newRegistrationClient(ctx context.Context, cfg config) (registrationClient, error) {
	svid, err := fetchSVID(ctx, cfg.AgentAddr)
	if err != nil {
		return nil, err
	}
	transportCreds := creds(cfg.TrustDomain, svid.PrivateKey, svid.Certificates, svid.TrustBundlePool)

	hostport := strings.Split(cfg.ServerAddr, ":")
	if len(hostport) != 2 {
		return nil, fmt.Errorf("address %q missing port", hostport)
	}
	host, port := hostport[0], hostport[1]

	// check for a dns name with multiple entries
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	// return a simple client if there is only one address
	if len(addrs) == 1 {
		conn, err := grpc.DialContext(ctx, cfg.ServerAddr, grpc.WithTransportCredentials(transportCreds))
		if err != nil {
			return nil, err
		}
		return registration.NewRegistrationClient(conn), nil
	}

	// return a round-robin muilti client if there are multiple addresses
	c := new(multiClient)
	for _, addr := range addrs {
		conn, err := grpc.DialContext(ctx, addr.String()+":"+port, grpc.WithTransportCredentials(transportCreds))
		if err != nil {
			return nil, err
		}
		c.clients = append(c.clients, registration.NewRegistrationClient(conn))
	}
	return c, nil
}

func creds(trustDomain string, key crypto.Signer, certs []*x509.Certificate, trustBundle *x509.CertPool) credentials.TransportCredentials {
	spiffePeer := &spiffe_tls.TLSPeer{
		SpiffeIDs:  []string{idutil.ServerID(trustDomain)},
		TrustRoots: trustBundle,
	}
	tlsCert := tls.Certificate{PrivateKey: key}
	for _, cert := range certs {
		tlsCert.Certificate = append(tlsCert.Certificate, cert.Raw)
	}
	return credentials.NewTLS(spiffePeer.NewTLSConfig([]tls.Certificate{tlsCert}))
}
