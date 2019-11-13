package fakeattestor

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"

	attestor "github.com/spiffe/spire/pkg/agent/attestor/node"
	"github.com/spiffe/spire/pkg/common/bundleutil"
)

type fake struct {
	svid   []*x509.Certificate
	key    *ecdsa.PrivateKey
	bundle *bundleutil.Bundle
}

func New(svid []*x509.Certificate, key *ecdsa.PrivateKey, bundle *bundleutil.Bundle) attestor.Attestor {
	return &fake{
		svid:   svid,
		key:    key,
		bundle: bundle,
	}
}

func (f *fake) Attest(ctx context.Context) (res *attestor.AttestationResult, err error) {
	return &attestor.AttestationResult{
		SVID:   f.svid,
		Key:    f.key,
		Bundle: f.bundle,
	}, nil
}
