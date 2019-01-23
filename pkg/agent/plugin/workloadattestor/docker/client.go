package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

const (
	defaultDockerSocketPath    = "/var/run/docker.sock"
	defaultDockerClientVersion = "1.26"
)

type dockerClient struct {
	socketPath string
	version    string
	httpClient *http.Client
}

// Returns a default http transport for the unix socket path address provided.
// Most of the default values are taken from net/http.
func unixTransportForAddr(address string) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext(ctx, "unix", address)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func newDockerClient(socketPath, version string) DockerClient {
	if socketPath == "" {
		socketPath = defaultDockerSocketPath
	}
	if version == "" {
		version = defaultDockerClientVersion
	}
	return &dockerClient{
		socketPath: socketPath,
		version:    version,
		httpClient: &http.Client{
			Transport: unixTransportForAddr(socketPath),
		},
	}
}

type ContainerInfo struct {
	Config ContainerConfig `json:"Config"`
}

type ContainerConfig struct {
	Labels map[string]string `json:"Labels"`
	Image  string            `json:"Image"`
}

func (c *dockerClient) ContainerInspect(ctx context.Context, containerID string) (*ContainerInfo, error) {
	// example: "/v1.26/containers/<id>/json"
	url := fmt.Sprintf("http://%s/v%s/containers/%s/json", c.socketPath, c.version, containerID)
	req, err := http.NewRequest(http.MethodGet, url, nil /* body */)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode > 299 {
		return nil, fmt.Errorf("unexpected error from docker daemon: %q", body)
	}
	info := new(ContainerInfo)
	if err := json.Unmarshal(body, info); err != nil {
		return nil, err
	}
	return info, nil
}
