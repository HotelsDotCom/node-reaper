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

package nodereaper

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"

	"github.com/HotelsDotCom/node-reaper/pkg/config"
	"github.com/HotelsDotCom/node-reaper/pkg/drainer"
	"github.com/HotelsDotCom/node-reaper/pkg/log"
	"github.com/HotelsDotCom/node-reaper/pkg/metrics"
)

type testState struct {
	nodes []corev1.Node
}

func (ts *testState) getNode() corev1.Node {
	return ts.nodes[0]
}

func (ts *testState) getNodes() []corev1.Node {
	return ts.nodes
}

func (ts *testState) deleteNode(name string) {
	for i, n := range ts.nodes {
		if n.Name == name {
			ts.nodes = append(ts.nodes[:i], ts.nodes[i+1:]...)
			return
		}
	}
}

func (ts *testState) setNodes(nodes []corev1.Node) {
	ts.nodes = nodes
}

type mockProvider struct {
	ReapFn func(node corev1.Node) error
	TypeFn func() string
}

func (p mockProvider) Reap(node corev1.Node) error {
	return p.ReapFn(node)
}

func (p mockProvider) Type() string {
	return p.TypeFn()
}

func TestRun(t *testing.T) {
	t.Parallel()

	type testFnOptions struct {
		freeze  bool
		latency time.Duration
	}

	type test struct {
		description string
		// The first node in the slice is passed to NodeReaper
		inputNodes    []testNodeSpec
		expectedNodes []testNodeSpec
		provider      NodeProvider
		expectErr     bool
		expectedErr   string
		config        config.Config

		providerReapFn        func(*testState, testFnOptions) func(corev1.Node) error
		providerReapFnOptions testFnOptions
		providerTypeFn        func() func() string
		getNodesFn            func(*testState, testFnOptions) getNodesFn
		getNodesFnOptions     testFnOptions
		drainNodeFn           func(*testState, testFnOptions) drainNodeFn
		drainNodeFnOptions    testFnOptions
	}

	defaultGetNodesFn := func(ts *testState, opts testFnOptions) getNodesFn {
		return func() ([]corev1.Node, error) {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			return ts.getNodes(), nil
		}
	}

	defaultDrainNodeFn := func(ts *testState, opts testFnOptions) drainNodeFn {
		return func(*corev1.Node, drainer.Options, log.Logger) error {
			nodes := ts.getNodes()
			nodes[0].Spec.Unschedulable = true
			ts.setNodes(nodes)
			return nil
		}
	}

	defaultReapFn := func(ts *testState, opts testFnOptions) func(corev1.Node) error {
		return func(node corev1.Node) error {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			ts.deleteNode(node.Name)
			return nil
		}
	}

	defaultTypeFn := func() func() string {
		return func() string {
			return "bogusType"
		}
	}

	defaultTime := time.Now()

	tests := []test{
		{
			description: "Test reapPredicate returns integer",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{},
				},
			},
			expectErr:   true,
			expectedErr: `error checking reaping eligibility: error evaluating reapPredicate: invalid predicate, not a boolean expression: 2 + 2`,
			config: config.Config{
				ReapPredicate:               `2 + 2`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test unhealthyPredicate returns integer",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error assessing cluster health: failed counting unhealthy nodes in cluster: error evaluating node conditions: invalid predicate, not a boolean expression: 2 + 2`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `2 + 2`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test invalid reapPredicate",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{},
				},
			},
			expectErr:   true,
			expectedErr: `error checking reaping eligibility: error creating new EvaluablePredicate for reapPredicate: Invalid token: '^^^'`,
			config: config.Config{
				ReapPredicate:               `^^^`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test invalid unhealthyPredicate",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error evaluating unhealthyPredicate: Invalid token: '^^^'`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `^^^`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test node not reapable",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{},
				},
			},
			expectErr: false,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test node reapable but cluster unhealthy",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr: false,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("0"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test node reapable but cluster zone unhealthy",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr: false,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("0"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test node reapable but cluster zone unhealthy",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr: false,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("0"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test node creation within grace period",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
					creation:   defaultTime,
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
					creation:   defaultTime,
				},
			},
			expectErr: false,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Invalid MaxNodesUnhealthy",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error assessing cluster health: failed creating ClusterHealthCalculator: can't parse MaxNodesUnhealthy. It should be a number or a percentage. Actual value: abc`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("abc"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Invalid MaxNodesUnhealthyInSameZone",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error assessing cluster health: failed creating ClusterHealthCalculator: can't parse MaxNodesUnhealthy. It should be a number or a percentage. Actual value: abc`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("abc"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn:    defaultDrainNodeFn,
		},
		{
			description: "Test error getting nodes",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error getting nodes: bogus error`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn: func(ts *testState, opts testFnOptions) getNodesFn {
				return func() ([]corev1.Node, error) {
					return []corev1.Node{}, fmt.Errorf("bogus error")
				}
			},
			drainNodeFn: defaultDrainNodeFn,
		},
		{
			description: "Test error draining with DrainForceRetryOnError == true",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error force retry draining: bogus error`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
				DrainForceRetryOnError:      true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return fmt.Errorf("bogus error")
				}
			},
		},
		{
			description: "Test error draining",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error draining: bogus error`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return fmt.Errorf("bogus error")
				}
			},
		},
		{
			description: "Test provider not found",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error reaping: plugin for provider "aws" not found`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 false,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return nil
				}
			},
		},
		{
			description: "Test error reaping",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error reaping: error reaping with provider "aws": bogus error`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 false,
			},
			providerReapFn: func(ts *testState, opts testFnOptions) func(corev1.Node) error {
				return func(corev1.Node) error {
					return fmt.Errorf("bogus error")
				}
			},
			providerTypeFn: func() func() string {
				return func() string {
					return "aws"
				}
			},
			getNodesFn: defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return nil
				}
			},
		},
		{
			description: "Test success draining and reaping",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{},
			expectErr:     false,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 false,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: func() func() string {
				return func() string {
					return "aws"
				}
			},
			getNodesFn: defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return nil
				}
			},
		},
		{
			description: "Test success draining but skip reaping",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr: false,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 true,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return nil
				}
			},
		},
		{
			description: "Test missing zone label",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "aws://foobar",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `label "failure-domain.beta.kubernetes.io/zone" not found in node object`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 false,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: func() func() string {
				return func() string {
					return "aws"
				}
			},
			getNodesFn: defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return nil
				}
			},
		},
		{
			description: "Test fail getting node ProviderID",
			inputNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectedNodes: []testNodeSpec{
				{
					name:       "node",
					providerID: "://foobar",
					zone:       "zone-a",
					tcs:        []testConditionSpec{{Type: "BogusCondition", Status: "Unknown"}},
				},
			},
			expectErr:   true,
			expectedErr: `error reaping: error getting node provider: failed extracting provider from NodeSpec.ProviderID`,
			config: config.Config{
				ReapPredicate:               `[Conditions.BogusCondition] == "Unknown"`,
				UnhealthyPredicate:          `[Conditions.BogusCondition] == "Unknown"`,
				MaxNodesUnhealthy:           config.MaxNodesUnhealthy("100%"),
				MaxNodesUnhealthyInSameZone: config.MaxNodesUnhealthy("100%"),
				NewNodeGracePeriod:          30 * time.Second,
				SkipReaping:                 false,
			},
			providerReapFn: defaultReapFn,
			providerTypeFn: defaultTypeFn,
			getNodesFn:     defaultGetNodesFn,
			drainNodeFn: func(ts *testState, opts testFnOptions) drainNodeFn {
				return func(*corev1.Node, drainer.Options, log.Logger) error {
					return nil
				}
			},
		},
	}

	for _, test := range tests {
		// Initialise testState
		state := testState{
			nodes: newNodes(test.inputNodes),
		}

		// Initialise NodeReaper
		k8scli := testclient.NewSimpleClientset()
		mReg := metrics.NewDummyMetrics()
		logger, _ := log.Dummy{}.New(log.INFO)
		provider := mockProvider{
			ReapFn: test.providerReapFn(&state, test.providerReapFnOptions),
			TypeFn: test.providerTypeFn(),
		}
		nr := newNodeReaper(
			&test.config,
			k8scli,
			NodeProviders{provider.Type(): provider},
			mReg,
			logger,
		).
			SetGetNodesFn(test.getNodesFn(&state, test.getNodesFnOptions)).
			SetDrainNodeFn(test.drainNodeFn(&state, test.drainNodeFnOptions))

		// Create expectedNodes
		expectedNodes := newNodes(test.expectedNodes)

		// Run NodeReaper
		err := nr.Run(&state.nodes[0])

		// Assert expectations
		if err == nil && test.expectErr {
			t.Fatalf("Error expected but not returned while testing `%s`", test.description)
		}
		if err != nil {
			if !test.expectErr {
				t.Fatalf("Error returned but not expected while testing `%s`:\n Got: %s", test.description, err)
			}
			if err.Error() != test.expectedErr {
				t.Fatalf("Unexpected error returned while testing `%s`:\nGot: %v\nExpected: %v", test.description, err, test.expectedErr)
			}
		}
		if !reflect.DeepEqual(expectedNodes, state.nodes) {
			t.Fatalf("Remaining node list doesn't match expected list while testing `%s`.\nGot: %v\nExpected: %v", test.description, prettify(state.nodes), prettify(expectedNodes))
		}
	}
}

type testNodeSpec struct {
	name       string
	providerID string
	zone       string
	tcs        []testConditionSpec
	creation   time.Time
}

type testConditionSpec struct {
	Type   string
	Status string
}

func newNodes(specs []testNodeSpec) []corev1.Node {
	nodes := []corev1.Node{}
	for _, spec := range specs {
		nodes = append(nodes, newNode(spec))
	}
	return nodes
}

func newNode(tns testNodeSpec) corev1.Node {
	objectMeta := metav1.ObjectMeta{
		Name:              tns.name,
		CreationTimestamp: metav1.NewTime(tns.creation),
	}
	if tns.zone != "" {
		objectMeta.Labels = map[string]string{
			"failure-domain.beta.kubernetes.io/zone": tns.zone,
		}
	}
	return corev1.Node{
		metav1.TypeMeta{},
		objectMeta,
		corev1.NodeSpec{ProviderID: tns.providerID},
		corev1.NodeStatus{
			Conditions: newConditions(tns.tcs),
		},
	}
}

func newConditions(specs []testConditionSpec) []corev1.NodeCondition {
	conditions := []corev1.NodeCondition{}
	for _, spec := range specs {
		conditions = append(conditions, newCondition(spec))
	}
	return conditions
}

func newCondition(tcs testConditionSpec) corev1.NodeCondition {
	return corev1.NodeCondition{
		Type:   corev1.NodeConditionType(tcs.Type),
		Status: corev1.ConditionStatus(tcs.Status),
	}
}

func prettify(d interface{}) string {
	output, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		panic(err)
	}
	return string(output)
}
