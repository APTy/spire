package defaultnoderesolver

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl"
	"github.com/spiffe/spire/pkg/common/util"
	"github.com/spiffe/spire/proto/common"
	spi "github.com/spiffe/spire/proto/common/plugin"
	"github.com/spiffe/spire/proto/server/noderesolver"
)

const PluginName = "default"

// New returns a new default node resolver.
func New() noderesolver.Plugin {
	return &impl{}
}

// Config is default node resolver behavior.
type Config struct {
	Selectors []string `hcl:"selectors"`
}

type impl struct {
	selectors []string
}

func (i *impl) Configure(ctx context.Context, req *spi.ConfigureRequest) (*spi.ConfigureResponse, error) {
	config := new(Config)
	if err := hcl.Decode(config, req.Configuration); err != nil {
		return nil, fmt.Errorf("unable to decode configuration: %v", err)
	}
	i.selectors = config.Selectors
	return &spi.ConfigureResponse{}, nil
}

func (i *impl) GetPluginInfo(context.Context, *spi.GetPluginInfoRequest) (*spi.GetPluginInfoResponse, error) {
	return &spi.GetPluginInfoResponse{}, nil
}

func (i *impl) Resolve(ctx context.Context, req *noderesolver.ResolveRequest) (*noderesolver.ResolveResponse, error) {
	selectorMap := make(map[string]*common.Selectors)
	for _, spiffeID := range req.BaseSpiffeIdList {
		selectors, err := i.resolveSpiffeID(ctx, spiffeID)
		if err != nil {
			return nil, err
		}
		selectorMap[spiffeID] = selectors
	}
	return &noderesolver.ResolveResponse{
		Map: selectorMap,
	}, nil
}

func (i *impl) resolveSpiffeID(ctx context.Context, spiffeID string) (*common.Selectors, error) {
	selectors := new(common.Selectors)
	for _, selector := range i.selectors {
		selectors.Entries = append(selectors.Entries, &common.Selector{
			Type:  PluginName,
			Value: selector,
		})
	}
	util.SortSelectors(selectors.Entries)
	return selectors, nil
}
