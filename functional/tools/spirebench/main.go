package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	uuid "github.com/gofrs/uuid"
	"github.com/spiffe/spire/api/workload/v2"
	"github.com/spiffe/spire/proto/spire/common"
)

type config struct {
	AgentAddr   string
	ServerAddr  string
	TrustDomain string
	RPS         int
	Duration    time.Duration
}

func newConfigFromFlags() config {
	var cfg config
	flag.StringVar(&cfg.AgentAddr, "agent-addr", workload.DefaultAgentAddress, "Location of SPIRE Agent Unix Listener.")
	flag.StringVar(&cfg.ServerAddr, "server-addr", "127.0.0.1:8081", "Location of SPIRE Server TCP Listener.")
	flag.StringVar(&cfg.TrustDomain, "trust-domain", "example.org", "Trust domain of SPIRE Server.")
	flag.IntVar(&cfg.RPS, "rps", 100, "Target requests per second.")
	flag.DurationVar(&cfg.Duration, "duration", 10*time.Second, "Benchmark duration.")
	flag.Parse()
	return cfg
}

func main() {
	res, err := run(newConfigFromFlags())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res)
}

func run(cfg config) (string, error) {
	ctx := context.Background()
	client, err := newRegistrationClient(ctx, cfg)
	if err != nil {
		return "", err
	}
	if err := basicTest(ctx, client, cfg.TrustDomain); err != nil {
		return "", err
	}

	fmt.Println("Starting \"create\" benchmark.")
	doBenchmark(cfg.RPS, cfg.Duration, func() error {
		id, err := uuid.NewV4()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if _, err := client.CreateEntry(ctx, &common.RegistrationEntry{
			Selectors: []*common.Selector{
				{
					Type:  "test",
					Value: "foo:bar",
				},
				{
					Type:  "test",
					Value: "baz:qux",
				},
			},
			ParentId: fmt.Sprintf("spiffe://%s/test/parent/%s", cfg.TrustDomain, id),
			SpiffeId: fmt.Sprintf("spiffe://%s/test/workload/%s", cfg.TrustDomain, id),
		}); err != nil {
			return err
		}
		return nil
	})
	//	DELETE FROM registered_entries WHERE parent_id LIKE '%/test/parent/%'; DELETE FROM selectors WHERE type = 'test';

	return "Success.", nil
}

func basicTest(ctx context.Context, client registrationClient, trustDomain string) error {
	id, err := uuid.NewV4()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reqID, err := client.CreateEntry(ctx, &common.RegistrationEntry{
		Selectors: []*common.Selector{
			{
				Type:  "test",
				Value: "foo:bar",
			},
		},
		ParentId: fmt.Sprintf("spiffe://%s/test/parent/%s", trustDomain, id),
		SpiffeId: fmt.Sprintf("spiffe://%s/test/workload/%s", trustDomain, id),
	})
	if err != nil {
		return err
	}
	if _, err := client.DeleteEntry(ctx, reqID); err != nil {
		return err
	}
	return nil
}
