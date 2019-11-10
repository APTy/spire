package run

import (
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/hcl"
	"github.com/imdario/mergo"
	"github.com/spiffe/spire/internal/dns"
	"github.com/spiffe/spire/internal/grpcrand"
	"github.com/spiffe/spire/pkg/agent"
	"github.com/spiffe/spire/pkg/common/catalog"
	"github.com/spiffe/spire/pkg/common/cli"
	"github.com/spiffe/spire/pkg/common/health"
	"github.com/spiffe/spire/pkg/common/idutil"
	"github.com/spiffe/spire/pkg/common/log"
	"github.com/spiffe/spire/pkg/common/pemutil"
	"github.com/spiffe/spire/pkg/common/telemetry"
	"github.com/spiffe/spire/pkg/common/util"
	"google.golang.org/grpc/resolver"
)

const (
	defaultConfigPath = "conf/agent/agent.conf"
	defaultSocketPath = "./spire_api"

	// TODO: Make my defaults sane
	defaultDataDir  = "."
	defaultLogLevel = "INFO"
)

func init() {
	// custom resolver with 30-60 second polling interval
	resolver.Register(dns.NewBuilder(
		dns.WithScheme("dns+poll60s"),
		dns.WithPollingInterval(time.Duration(30+grpcrand.Intn(30))*time.Second),
	))
}

// config contains all available configurables, arranged by section
type config struct {
	Agent        *agentConfig                `hcl:"agent"`
	Plugins      *catalog.HCLPluginConfigMap `hcl:"plugins"`
	Telemetry    telemetry.FileConfig        `hcl:"telemetry"`
	HealthChecks health.Config               `hcl:"health_checks"`
}

type agentConfig struct {
	DataDir           string `hcl:"data_dir"`
	EnableSDS         bool   `hcl:"enable_sds"`
	InsecureBootstrap bool   `hcl:"insecure_bootstrap"`
	JoinToken         string `hcl:"join_token"`
	LogFile           string `hcl:"log_file"`
	LogFormat         string `hcl:"log_format"`
	LogLevel          string `hcl:"log_level"`
	ServerAddress     string `hcl:"server_address"`
	ServerPort        int    `hcl:"server_port"`
	SocketPath        string `hcl:"socket_path"`
	TrustBundlePath   string `hcl:"trust_bundle_path"`
	TrustDomain       string `hcl:"trust_domain"`

	ConfigPath string

	// Undocumented configurables
	ProfilingEnabled bool     `hcl:"profiling_enabled"`
	ProfilingPort    int      `hcl:"profiling_port"`
	ProfilingFreq    int      `hcl:"profiling_freq"`
	ProfilingNames   []string `hcl:"profiling_names"`
}

type RunCLI struct {
}

func (*RunCLI) Help() string {
	_, err := parseFlags([]string{"-h"})
	return err.Error()
}

func (*RunCLI) Run(args []string) int {
	cliInput, err := parseFlags(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fileInput, err := parseFile(cliInput.ConfigPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	input, err := mergeInput(fileInput, cliInput)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	c, err := newAgentConfig(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// Create uds dir and parents if not exists
	dir := filepath.Dir(c.BindAddress.String())
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		c.Log.WithField("dir", dir).Infof("Creating spire agent UDS directory")
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	// Set umask before starting up the agent
	cli.SetUmask(c.Log)

	a := agent.New(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	util.SignalListener(ctx, cancel)

	err = a.Run(ctx)
	if err != nil {
		c.Log.WithError(err).Error("agent crashed")
		return 1
	}

	c.Log.Info("Agent stopped gracefully")
	return 0
}

func (*RunCLI) Synopsis() string {
	return "Runs the agent"
}

func parseFile(path string) (*config, error) {
	c := &config{}

	if path == "" {
		path = defaultConfigPath
	}

	// Return a friendly error if the file is missing
	data, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			msg := "could not determine CWD; config file not found at %s: use -config"
			return nil, fmt.Errorf(msg, path)
		}

		msg := "could not find config file %s: please use the -config flag"
		return nil, fmt.Errorf(msg, absPath)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to read configuration at %q: %v", path, err)
	}

	if err := hcl.Decode(&c, string(data)); err != nil {
		return nil, fmt.Errorf("unable to decode configuration at %q: %v", path, err)
	}

	return c, nil
}

func parseFlags(args []string) (*agentConfig, error) {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	c := &agentConfig{}

	flags.StringVar(&c.ConfigPath, "config", defaultConfigPath, "Path to a SPIRE config file")
	flags.StringVar(&c.DataDir, "dataDir", "", "A directory the agent can use for its runtime data")
	flags.StringVar(&c.JoinToken, "joinToken", "", "An optional token which has been generated by the SPIRE server")
	flags.StringVar(&c.LogFile, "logFile", "", "File to write logs to")
	flags.StringVar(&c.LogFormat, "logFormat", "", "'text' or 'json'")
	flags.StringVar(&c.LogLevel, "logLevel", "", "'debug', 'info', 'warn', or 'error'")
	flags.StringVar(&c.ServerAddress, "serverAddress", "", "IP address or DNS name of the SPIRE server")
	flags.IntVar(&c.ServerPort, "serverPort", 0, "Port number of the SPIRE server")
	flags.StringVar(&c.SocketPath, "socketPath", "", "Location to bind the workload API socket")
	flags.StringVar(&c.TrustDomain, "trustDomain", "", "The trust domain that this agent belongs to")
	flags.StringVar(&c.TrustBundlePath, "trustBundle", "", "Path to the SPIRE server CA bundle")
	flags.BoolVar(&c.InsecureBootstrap, "insecureBootstrap", false, "If true, the agent bootstraps without verifying the server's identity")

	err := flags.Parse(args)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func mergeInput(fileInput *config, cliInput *agentConfig) (*config, error) {
	c := &config{Agent: &agentConfig{}}

	// Highest precedence first
	err := mergo.Merge(c.Agent, cliInput)
	if err != nil {
		return nil, err
	}

	err = mergo.Merge(c, fileInput)
	if err != nil {
		return nil, err
	}

	err = mergo.Merge(c, defaultConfig())
	if err != nil {
		return nil, err
	}

	return c, nil
}

func newAgentConfig(c *config) (*agent.Config, error) {
	ac := &agent.Config{}

	if err := validateConfig(c); err != nil {
		return nil, err
	}

	serverHostPort := net.JoinHostPort(c.Agent.ServerAddress, strconv.Itoa(c.Agent.ServerPort))
	// TODO: move entire grpc target to config?
	ac.ServerAddress = fmt.Sprintf("dns+poll60s:///%s", serverHostPort)

	td, err := idutil.ParseSpiffeID("spiffe://"+c.Agent.TrustDomain, idutil.AllowAnyTrustDomain())
	if err != nil {
		return nil, fmt.Errorf("could not parse trust_domain %q: %v", c.Agent.TrustDomain, err)
	}
	ac.TrustDomain = *td

	// Parse trust bundle
	ac.InsecureBootstrap = c.Agent.InsecureBootstrap
	if c.Agent.TrustBundlePath != "" {
		bundle, err := parseTrustBundle(c.Agent.TrustBundlePath)
		if err != nil {
			return nil, fmt.Errorf("could not parse trust bundle: %s", err)
		}
		ac.TrustBundle = bundle
	}

	ac.BindAddress = &net.UnixAddr{
		Name: c.Agent.SocketPath,
		Net:  "unix",
	}

	ac.JoinToken = c.Agent.JoinToken
	ac.DataDir = c.Agent.DataDir
	ac.EnableSDS = c.Agent.EnableSDS

	ll := strings.ToUpper(c.Agent.LogLevel)
	lf := strings.ToUpper(c.Agent.LogFormat)
	logger, err := log.NewLogger(ll, lf, c.Agent.LogFile)
	if err != nil {
		return nil, fmt.Errorf("could not start logger: %s", err)
	}
	ac.Log = logger

	ac.ProfilingEnabled = c.Agent.ProfilingEnabled
	ac.ProfilingPort = c.Agent.ProfilingPort
	ac.ProfilingFreq = c.Agent.ProfilingFreq
	ac.ProfilingNames = c.Agent.ProfilingNames

	ac.PluginConfigs = *c.Plugins
	ac.Telemetry = c.Telemetry
	ac.HealthChecks = c.HealthChecks

	return ac, nil
}

func validateConfig(c *config) error {
	if c.Agent == nil {
		return errors.New("agent section must be configured")
	}

	if c.Agent.ServerAddress == "" {
		return errors.New("server_address must be configured")
	}

	if c.Agent.ServerPort == 0 {
		return errors.New("server_port must be configured")
	}

	if c.Agent.TrustDomain == "" {
		return errors.New("trust_domain must be configured")
	}

	if c.Agent.TrustBundlePath == "" && !c.Agent.InsecureBootstrap {
		return errors.New("trust_bundle_path must be configured unless insecure_bootstrap is set")
	}

	if c.Plugins == nil {
		return errors.New("plugins section must be configured")
	}

	return nil
}

func defaultConfig() *config {
	return &config{
		Agent: &agentConfig{
			DataDir:    defaultDataDir,
			LogLevel:   defaultLogLevel,
			LogFormat:  log.DefaultFormat,
			SocketPath: defaultSocketPath,
		},
	}
}

func parseTrustBundle(path string) ([]*x509.Certificate, error) {
	bundle, err := pemutil.LoadCertificates(path)
	if err != nil {
		return nil, err
	}

	if len(bundle) == 0 {
		return nil, errors.New("no certificates found in trust bundle")
	}

	return bundle, nil
}
