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

package config

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
	"github.com/HotelsDotCom/node-reaper/pkg/log"
)

// MaxNodesUnhealthy is the MaxNodesUnhealthy setting. It has its own type declaration so that it can have methods.
type MaxNodesUnhealthy string

// MaxNodesUnhealthyType in a pseudo enum to set accepted types of the MaxNodeUnheathy property
type MaxNodesUnhealthyType int

// Sets the accepted MaxNodesUnhealthy configuration types
const (
	ABSOLUTE = iota + 1
	PERCENTUAL
)

var maxNodesUnhealthyType = [...]string{
	"ABSOLUTE",
	"PERCENTUAL",
}

func (t MaxNodesUnhealthyType) String() string {
	return maxNodesUnhealthyType[t]
}

// File is the configuration file for node reaper.
type File struct {
	ReapPredicate                 string            `yaml:"reapPredicate"`
	UnhealthyPredicate            string            `yaml:"unhealthyPredicate"`
	MaxNodesUnhealthy             MaxNodesUnhealthy `yaml:"maxNodesUnhealthy"`
	MaxNodesUnhealthyInSameZone   MaxNodesUnhealthy `yaml:"maxNodesUnhealthyInSameZone"`
	NewNodeGracePeriodSeconds     uint              `yaml:"newNodeGracePeriodSeconds"`
	DrainTimeoutSeconds           uint              `yaml:"drainTimeoutSeconds"`
	DrainForceRetryOnError        bool              `yaml:"drainForceRetryOnError"`
	PodDeletionGracePeriodSeconds uint              `yaml:"podDeletionGracePeriodSeconds"`
	UseEviction                   bool              `yaml:"useEviction"`
	SkipReaping                   bool              `yaml:"skipReaping"`
	DryRun                        bool              `yaml:"dryRun"`
}

// Config is the configuration for node reaper.
type Config struct {
	// Defines a govaluate EvaluableExpression which is used to evaluate whether a node is reapable
	ReapPredicate string

	// Defines a govaluate EvaluableExpression which is used to evaluate whether a node is unhealthy
	UnhealthyPredicate string

	// Defines an absolute number or a percentage which serve as a limit to how many nodes may be unhealthy in a cluster before reaping can proceed
	MaxNodesUnhealthy MaxNodesUnhealthy

	// Defines an absolute number or a percentage which serve as a limit to how many nodes may be unhealthy in the same zone as the reapable node before reaping can proceed
	MaxNodesUnhealthyInSameZone MaxNodesUnhealthy

	// Grace Period when new nodes aren't reaped
	NewNodeGracePeriod time.Duration

	// Timeout for draining
	DrainTimeout time.Duration

	// Defines whether to retry draining forcefully if draining fails
	DrainForceRetryOnError bool

	// Grace period to be used in evict/delete call to kubernetes
	PodDeletionGracePeriod time.Duration

	// If true use eviction instead of directly deleting pods
	UseEviction bool

	// Defines whether to reap a node after draining
	SkipReaping bool

	// If true runs with no side effects, instead logging when it would've drained or reaped
	DryRun bool

	logger log.Logger
}

var defaultConfig = File{
	ReapPredicate: `
		Cordoned                       == false
		&& [Labels.kubernetes.io/role] != 'master'
		&& [Conditions.Ready]          != 'True'`,
	UnhealthyPredicate: `
		Cordoned                       == true
		|| [Conditions.OutOfDisk]      != 'False'
		|| [Conditions.MemoryPressure] != 'False'
		|| [Conditions.DiskPressure]   != 'False'
		|| [Conditions.Ready]          != 'True'`,
	MaxNodesUnhealthy:             MaxNodesUnhealthy("1"),
	MaxNodesUnhealthyInSameZone:   MaxNodesUnhealthy("1"),
	NewNodeGracePeriodSeconds:     uint(300),
	DrainTimeoutSeconds:           uint(310),
	DrainForceRetryOnError:        true,
	PodDeletionGracePeriodSeconds: uint(300),
	UseEviction:                   true,
	SkipReaping:                   false,
	DryRun:                        true,
}

// Load loads configuration from a file, if a property is not specified the default is used
func Load(path string) (*File, error) {
	cfgYaml, err := ioutil.ReadFile(path)
	if err != nil {
		return &File{}, err
	}

	// Load defaults
	cfg := defaultConfig

	// Override with provided configuration file
	err = yaml.UnmarshalStrict(cfgYaml, &cfg)
	if err != nil {
		return &File{}, err
	}

	// If no property was found in the config file, use default
	zeroedConfig := File{}
	if cfg == zeroedConfig {
		cfg = defaultConfig
	}

	return &cfg, nil
}

// Parse parses all configuration parameters, validating them
func (cf *File) Parse(logger log.Logger) (*Config, error) {
	c := Config{
		logger: logger,
	}
	for _, err := range []error{
		c.setReapPredicate(*cf),
		c.setUnhealthyPredicate(*cf),
		c.setMaxNodesUnhealthy(*cf),
		c.setMaxNodesUnhealthyInSameZone(*cf),
		c.setNewNodeGracePeriod(*cf),
		c.setDrainTimeout(*cf),
		c.setDrainForceRetryOnError(*cf),
		c.setPodDeletionGracePeriod(*cf),
		c.setUseEviction(*cf),
		c.setSkipReaping(*cf),
		c.setDryRun(*cf),
	} {
		if err != nil {
			return &Config{}, err
		}
	}
	return &c, nil
}

// Log logs the configuration properties
func (c *Config) Log() {
	c.logger.Infof(fmt.Sprintf("%-45s %s", "ReapPredicate set to:", c.ReapPredicate))
	c.logger.Infof(fmt.Sprintf("%-45s %s", "UnhealthyPredicate set to:", c.UnhealthyPredicate))
	c.logger.Infof(fmt.Sprintf("%-45s %s", "MaxNodesUnhealthy set to:", c.MaxNodesUnhealthy))
	c.logger.Infof(fmt.Sprintf("%-45s %s", "MaxNodesUnhealthyInSameZone set to:", c.MaxNodesUnhealthyInSameZone))
	c.logger.Infof(fmt.Sprintf("%-45s %d", "NewNodeGracePeriod set to:", c.NewNodeGracePeriod/time.Second))
	c.logger.Infof(fmt.Sprintf("%-45s %d", "DrainTimeout set to:", c.DrainTimeout/time.Second))
	c.logger.Infof(fmt.Sprintf("%-45s %t", "DrainForceRetryOnError set to:", c.DrainForceRetryOnError))
	c.logger.Infof(fmt.Sprintf("%-45s %d", "PodDeletionGracePeriod set to:", c.PodDeletionGracePeriod/time.Second))
	c.logger.Infof(fmt.Sprintf("%-45s %t", "UseEviction set to:", c.UseEviction))
	c.logger.Infof(fmt.Sprintf("%-45s %t", "SkipReaping set to:", c.SkipReaping))
	c.logger.Infof(fmt.Sprintf("%-45s %t", "DryRun set to:", c.DryRun))
}

func (c *Config) setReapPredicate(cf File) error {
	if cf.ReapPredicate == "" {
		d := `Cordoned == false && [Labels.kubernetes.io/role] != 'master' && [Conditions.Ready] != 'True'`
		c.logger.Warnf(`reapPredicate not specified, setting it to the default of %q`, d)
		c.ReapPredicate = d
		return nil
	}
	c.ReapPredicate = cf.ReapPredicate
	return nil
}

func (c *Config) setUnhealthyPredicate(cf File) error {
	c.UnhealthyPredicate = cf.UnhealthyPredicate
	return nil
}

func (c *Config) setMaxNodesUnhealthy(cf File) error {
	_, err := cf.MaxNodesUnhealthy.Type()
	if err != nil {
		return err
	}
	c.MaxNodesUnhealthy = cf.MaxNodesUnhealthy
	return nil
}

func (c *Config) setMaxNodesUnhealthyInSameZone(cf File) error {
	_, err := cf.MaxNodesUnhealthyInSameZone.Type()
	if err != nil {
		return err
	}
	c.MaxNodesUnhealthyInSameZone = cf.MaxNodesUnhealthyInSameZone
	return nil
}

func (c *Config) setNewNodeGracePeriod(cf File) error {
	if cf.NewNodeGracePeriodSeconds == 0 {
		c.logger.Warn("NewNodeGracePeriod set to 0, new nodes may not be given enough time to start")
	}
	c.NewNodeGracePeriod = time.Duration(cf.NewNodeGracePeriodSeconds) * time.Second
	return nil
}

func (c *Config) setDrainTimeout(cf File) error {
	if cf.DrainTimeoutSeconds < 30 {
		c.logger.Warn("Overriding DrainTimeout to 30. DrainTimeout cannot be lower than 30 seconds")
		c.DrainTimeout = time.Duration(30) * time.Second
		return nil
	}
	c.DrainTimeout = time.Duration(cf.DrainTimeoutSeconds) * time.Second
	return nil
}

func (c *Config) setDrainForceRetryOnError(cf File) error {
	c.DrainForceRetryOnError = cf.DrainForceRetryOnError
	return nil
}

func (c *Config) setPodDeletionGracePeriod(cf File) error {
	c.PodDeletionGracePeriod = time.Duration(cf.PodDeletionGracePeriodSeconds) * time.Second
	if cf.PodDeletionGracePeriodSeconds == 0 {
		c.logger.Warn("PodDeletionGracePeriod set to 0, pods may not be given enough time to gracefully shutdown")
	}
	return nil
}

func (c *Config) setUseEviction(cf File) error {
	c.UseEviction = cf.UseEviction
	return nil
}

func (c *Config) setSkipReaping(cf File) error {
	c.SkipReaping = cf.SkipReaping
	return nil
}

func (c *Config) setDryRun(cf File) error {
	c.DryRun = cf.DryRun
	return nil
}

// Type returns the type of the MaxNodesUnhealthy setting. One of {ABSOLUTE, PERCENTUAL}.
// It's ABSOLUTE when it's solely numbers, and PERCENTUAL when it's between 1 and 3 digits and ends in a percent sign.
func (mnu MaxNodesUnhealthy) Type() (MaxNodesUnhealthyType, error) {
	var mt MaxNodesUnhealthyType
	if m, err := regexp.MatchString(`^\d+$`, mnu.String()); m == true && err == nil {
		mt = ABSOLUTE
		return mt, nil
	}
	if m, err := regexp.MatchString(`^\d{1,3}%$`, mnu.String()); m == true && err == nil {
		val, err := mnu.Uint()
		if err != nil {
			return 0, fmt.Errorf(
				`can't parse MaxNodesUnhealthy. It should be a number or a percentage. Actual value: %s`,
				mnu)
		}
		if val > 100 {
			return 0, fmt.Errorf(
				`invalid MaxNodesUnhealthy. It cannot be a percentage larger than 100%%. Actual value: %s`,
				mnu)
		}
		mt = PERCENTUAL
		return mt, nil
	}
	return 0, fmt.Errorf(
		`can't parse MaxNodesUnhealthy. It should be a number or a percentage. Actual value: %s`,
		mnu)
}

// String returns MaxNodesUnhealthy value cast to string
func (mnu MaxNodesUnhealthy) String() string {
	return string(mnu)
}

// Uint returns MaxNodesUnhealthy value cast to uint, and returns an error if the cast fails
func (mnu MaxNodesUnhealthy) Uint() (uint, error) {
	u, err := strconv.ParseUint(strings.TrimSuffix(mnu.String(), `%`), 10, 0)
	if err != nil {
		return 0, err
	}
	return uint(u), nil
}
