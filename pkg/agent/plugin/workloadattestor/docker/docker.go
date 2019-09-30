package docker

import (
	"context"
	"fmt"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/hcl"
	"github.com/spiffe/spire/pkg/agent/common/cgroups"
	"github.com/spiffe/spire/pkg/common/catalog"
	"github.com/spiffe/spire/pkg/common/plugin/docker"
	"github.com/spiffe/spire/proto/spire/agent/workloadattestor"
	"github.com/spiffe/spire/proto/spire/common"
	spi "github.com/spiffe/spire/proto/spire/common/plugin"
)

const (
	pluginName          = "docker"
	subselectorLabel    = "label"
	subselectorImageID  = "image_id"
	defaultCgroupPrefix = "/docker"
)

var defaultContainerIndex = 1

func BuiltIn() catalog.Plugin {
	return builtin(New())
}

func builtin(p *DockerPlugin) catalog.Plugin {
	return catalog.MakePlugin(pluginName, workloadattestor.PluginServer(p))
}

// DockerClient is a subset of the docker client functionality, useful for mocking.
type DockerClient interface {
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
}

type DockerPlugin struct {
	log                  hclog.Logger
	docker               DockerClient
	cgroupContainerIndex int
	fs                   cgroups.FileSystem
	mtx                  *sync.RWMutex
	retryer              *retryer
	containerIDFinder    docker.ContainerIDFinder
}

func New() *DockerPlugin {
	return &DockerPlugin{
		mtx:     &sync.RWMutex{},
		fs:      cgroups.OSFileSystem{},
		retryer: newRetryer(),
	}
}

type dockerPluginConfig struct {
	// DockerSocketPath is the location of the docker daemon socket (default: "unix:///var/run/docker.sock" on unix).
	DockerSocketPath string `hcl:"docker_socket_path"`
	// DockerVersion is the API version of the docker daemon (default: "1.40").
	DockerVersion string `hcl:"docker_version"`
	// CgroupPrefix is the cgroup prefix to look for in the cgroup entries (default: "/docker").
	CgroupPrefix string `hcl:"cgroup_prefix"`
	// CgroupContainerIndex is the index within the cgroup path where the container ID should be found (default: 1).
	// This is a *int to allow differentiation between the default int value (0) and the absence of the field.
	CgroupContainerIndex *int `hcl:"cgroup_container_index"`
	// ContainerIDCGroupMatchers
	ContainerIDCGroupMatchers []string `hcl:"container_id_cgroup_matchers"`
}

func (p *DockerPlugin) SetLogger(log hclog.Logger) {
	p.log = log
}

func (p *DockerPlugin) Attest(ctx context.Context, req *workloadattestor.AttestRequest) (*workloadattestor.AttestResponse, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	cgroupList, err := cgroups.GetCgroups(req.Pid, p.fs)
	if err != nil {
		return nil, err
	}

	var containerID string
	var hasDockerEntries bool
	for _, cgroup := range cgroupList {
		// We are only interested in cgroup entries that match our desired pattern. Example entry:
		// "10:perf_event:/docker/2235ebefd9babe0dde4df4e7c49708e24fb31fb851edea55c0ee29a18273cdf4"
		id, ok := p.containerIDFinder.FindContainerID(cgroup.GroupPath)
		if !ok {
			continue
		}
		hasDockerEntries = true
		containerID = id
		break
	}

	// Not a docker workload. Since it is expected that non-docker workloads will call the
	// workload API, it is fine to return a response without any selectors.
	if !hasDockerEntries {
		return &workloadattestor.AttestResponse{}, nil
	}
	if containerID == "" {
		return nil, fmt.Errorf("workloadattestor/docker: a pattern matched, but no container id was found")
	}

	var container types.ContainerJSON
	p.retryer.Retry(ctx, func() error {
		container, err = p.docker.ContainerInspect(ctx, containerID)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &workloadattestor.AttestResponse{
		Selectors: getSelectorsFromConfig(container.Config),
	}, nil
}

func getSelectorsFromConfig(cfg *container.Config) []*common.Selector {
	var selectors []*common.Selector
	for label, value := range cfg.Labels {
		selectors = append(selectors, &common.Selector{
			Type:  pluginName,
			Value: fmt.Sprintf("%s:%s:%s", subselectorLabel, label, value),
		})
	}
	if cfg.Image != "" {
		selectors = append(selectors, &common.Selector{
			Type:  pluginName,
			Value: fmt.Sprintf("%s:%s", subselectorImageID, cfg.Image),
		})
	}
	return selectors
}

func (p *DockerPlugin) Configure(ctx context.Context, req *spi.ConfigureRequest) (*spi.ConfigureResponse, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	var err error
	config := &dockerPluginConfig{}
	if err = hcl.Decode(config, req.Configuration); err != nil {
		return nil, err
	}

	var opts []func(*dockerclient.Client) error
	if config.DockerSocketPath != "" {
		opts = append(opts, dockerclient.WithHost(config.DockerSocketPath))
	}
	if config.DockerVersion != "" {
		opts = append(opts, dockerclient.WithVersion(config.DockerVersion))
	}
	p.docker, err = dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	if config.CgroupPrefix == "" {
		config.CgroupPrefix = defaultCgroupPrefix
	}
	if config.CgroupContainerIndex == nil {
		config.CgroupContainerIndex = &defaultContainerIndex
	}

	p.containerIDFinder, err = docker.NewContainerIDFinders(config.ContainerIDCGroupMatchers)
	if err != nil {
		return nil, err
	}

	return &spi.ConfigureResponse{}, nil
}

func (*DockerPlugin) GetPluginInfo(context.Context, *spi.GetPluginInfoRequest) (*spi.GetPluginInfoResponse, error) {
	return &spi.GetPluginInfoResponse{}, nil
}
