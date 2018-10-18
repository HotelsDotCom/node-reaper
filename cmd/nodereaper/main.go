/*
Copyright 2018 Expedia Inc.

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

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/HotelsDotCom/node-reaper/pkg/config"
	"github.com/HotelsDotCom/node-reaper/pkg/log"
	"github.com/HotelsDotCom/node-reaper/pkg/metrics"
	"github.com/HotelsDotCom/node-reaper/pkg/nodereaper"
)

// version and gitCommit get injected at build time with ldflags
var version string
var gitCommit string

// nodereaper env vars
const (
	NodeReaperConfigEnvVar = "NODEREAPERCONFIG"
	KubeConfigEnvVar       = "KUBECONFIG"
)

type cmdFlags struct {
	config          string
	kubeConfig      string
	logLevel        string
	listenAddr      string
	version         bool
	providerPlugins providerPlugins
}

type providerPlugins []string

func (p *providerPlugins) String() string {
	return fmt.Sprint(*p)
}

func (p *providerPlugins) Type() string {
	return "providerPlugins"
}

func (p *providerPlugins) Set(value string) error {
	for _, pluginPath := range strings.Split(value, ",") {
		*p = append(*p, pluginPath)
	}
	return nil
}

func (c *cmdFlags) init() {
	flag.StringVar(&c.config, "config", "node-reaper.yaml", "specifies the path to node-reaper's configuration file")
	flag.StringVar(&c.kubeConfig, "kubeconfig", "", "kubernetes configuration path, not to be used in production")
	flag.StringVar(&c.logLevel, "loglevel", "", "log level, one of INFO or DEBUG")
	flag.StringVar(&c.listenAddr, "listenaddr", ":8080", "address to listen to for /metrics endpoint")
	flag.BoolVar(&c.version, "version", false, "show version")
	flag.Var(&c.providerPlugins, "plugin", "node provider plugin file paths to load (.so)")

	flag.Parse()
}

// Main is the main runner.
type Main struct {
	flags  *cmdFlags
	logger log.Logger
	stopC  chan struct{}
}

// New returns a Main object.
func New() Main {
	flags := &cmdFlags{}
	flags.init()

	return Main{
		flags: flags,
	}
}

// Run execs the program.
func (m *Main) Run() error {
	if m.flags.version {
		printVersion()
		return nil
	}

	// Create signal channels
	m.stopC = make(chan struct{})
	errC := make(chan error)

	// Create logger
	ll := log.Level(log.INFO)
	if m.flags.logLevel != "" {
		var err error
		ll, err = log.StringToLogLevel(m.flags.logLevel)
		if err != nil {
			return err
		}
	}
	logger, err := log.Zap{}.New(ll)
	if err != nil {
		return err
	}
	m.setLogger(logger)

	logger.Infof("Starting nodereaper v%s (%s)", version, gitCommit)

	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	k8scli, err := m.createKubernetesClients()
	if err != nil {
		return err
	}

	npp, err := m.loadNodeProviderPlugins()
	if err != nil {
		return fmt.Errorf("error loading node provider plugins: %s", err)
	}
	if len(npp) == 0 {
		m.logger.Warnf("Disabling reaping, will only cordon and drain Nodes: no plugins loaded")
		cfg.SkipReaping = true
	}

	mReg := metrics.NewPrometheusMetrics("/metrics", http.DefaultServeMux)

	// Serve metrics
	go func() {
		logger.Infof("Listening on %s for metrics endpoint", m.flags.listenAddr)
		err := http.ListenAndServe(m.flags.listenAddr, nil)
		if err != nil {
			errC <- fmt.Errorf("error running http listener for metrics endpoint: %s", err)
		}
	}()

	cfg.Log()

	nodeReaperController := nodereaper.New(cfg, k8scli, npp, mReg, m.logger)
	if err != nil {
		return err
	}
	go func() {
		errC <- nodeReaperController.Run(m.stopC)
	}()

	// Await signals
	sigC := m.createSignalCapturer()
	var finalErr error
	select {
	case <-sigC:
		m.logger.Warn("Signal captured, exiting...")
	case err := <-errC:
		m.logger.Errorf("Error received: %q, exiting...", err)
		finalErr = err
	}

	m.stop(m.stopC)
	return finalErr
}

func printVersion() {
	fmt.Println(fmt.Sprintf(`NodeReaper v%s -- %s`,
		version,
		gitCommit,
	))
}

func resolveConfigChain(env string, flag string) string {
	r := ""
	fromEnv := filepath.Join(os.Getenv(env))
	if fromEnv != "" {
		r = fromEnv
	}
	if flag != "" {
		r = flag
	}
	return r
}

func (m *Main) setLogger(l log.Logger) {
	m.logger = l
}

func (m Main) loadConfig() (*config.Config, error) {
	path, _ := filepath.Abs(resolveConfigChain(NodeReaperConfigEnvVar, m.flags.config))
	cfgFile, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("could not load configuration: %s", err)
	}
	cfg, err := cfgFile.Parse(m.logger)
	if err != nil {
		return nil, fmt.Errorf("could not parse configuration: %s", err)
	}
	return cfg, nil
}

// loadKubernetesConfig loads kubernetes configuration based on flags.
func (m *Main) loadKubernetesConfig(kubeConfigPath string) (*rest.Config, error) {
	var cfg *rest.Config
	if kubeConfigPath != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, fmt.Errorf("could not load kubeconfig: %s", err)
		}
		cfg = config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error loading kubernetes configuration inside cluster, is the app running outside kubernetes? Error: %s", err)
		}
		cfg = config
	}
	return cfg, nil
}

func (m Main) loadNodeProviderPluginFile(path string) (nodereaper.NodeProvider, error) {
	p, err := nodereaper.LoadNodeProviderPlugin(path)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (m Main) loadNodeProviderPluginDirectory(path string) (nodereaper.NodeProviders, error) {
	plugins := nodereaper.NodeProviders{}
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nodereaper.NodeProviders{}, fmt.Errorf("error loading plugin directory with path %q: %s", path, err)
	}
	for _, file := range files {
		absPath, err := filepath.Abs(path + "/" + file.Name())
		if err != nil {
			return plugins, fmt.Errorf("error loading plugin directory with path %q: %s", path, err)
		}
		p, err := m.loadNodeProviderPluginFile(absPath)
		if err != nil {
			m.logger.Errorf("Error loading plugin file with path %q: %s", absPath, err)
			continue
		}
		if _, ok := plugins[p.Type()]; ok {
			return plugins, fmt.Errorf("duplicate plugin of type %q found in path %q", p.Type(), absPath)
		}
		plugins[p.Type()] = p
	}
	return plugins, nil
}

func (m Main) loadNodeProviderPlugins() (nodereaper.NodeProviders, error) {
	plugins := nodereaper.NodeProviders{}
	// Try to load plugins from $PWD/plugins if no -plugin flags were passed
	if len(m.flags.providerPlugins) == 0 {
		m.flags.providerPlugins = append(m.flags.providerPlugins, "plugins")
	}
	for _, path := range m.flags.providerPlugins {
		f, err := os.Stat(path)
		if err != nil {
			m.logger.Errorf("Error loading plugin with path %q: %s", path, err)
			continue
		}
		if !f.IsDir() {
			p, err := m.loadNodeProviderPluginFile(path)
			if err != nil {
				m.logger.Error(err)
			}
			if _, ok := plugins[p.Type()]; ok {
				return plugins, fmt.Errorf("duplicate plugin of type %q found in path %q", p.Type(), path)
			}
			plugins[p.Type()] = p
			m.logger.Infof("Successully loaded NodeProvider plugin %q from path %q", p.Type(), path)
			continue
		}
		// It is a directory
		dplugins, err := m.loadNodeProviderPluginDirectory(path)
		if err != nil {
			return plugins, err
		}
		for _, p := range dplugins {
			if _, ok := plugins[p.Type()]; ok {
				return plugins, fmt.Errorf("duplicate plugin of type %q found in path %q", p.Type(), path)
			}
			plugins[p.Type()] = p
			m.logger.Infof("Successully loaded NodeProvider plugin %q from path %q", p.Type(), path)
		}
	}
	return plugins, nil
}

func (m *Main) createKubernetesClients() (kubernetes.Interface, error) {
	kubeConfig, err := m.loadKubernetesConfig(resolveConfigChain(KubeConfigEnvVar, m.flags.kubeConfig))
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func (m *Main) createSignalCapturer() <-chan os.Signal {
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGTERM, syscall.SIGINT)
	return sigC
}

func (m *Main) stop(stopC chan struct{}) {
	m.logger.Warn("Stopping everything...")
	close(stopC)
}

// Run app.
func main() {
	m := New()

	if err := m.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error executing: %s", err)
		os.Exit(1)
	}
	os.Exit(0)
}
