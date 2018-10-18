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
	"testing"

	"github.com/HotelsDotCom/node-reaper/pkg/config"
)

func TestAbsoluteClusterHealthCalculator(t *testing.T) {
	t.Parallel()

	type testCase struct {
		description       string
		maxUnhealthyNodes uint
		nodesUnhealthy    uint
		expectedReturn    bool
	}

	testCases := []testCase{
		{
			description:       "Zero nodes unhealthy",
			maxUnhealthyNodes: 1,
			nodesUnhealthy:    0,
			expectedReturn:    true,
		},
		{
			description:       "Nodes unhealthy within limit",
			maxUnhealthyNodes: 1,
			nodesUnhealthy:    1,
			expectedReturn:    true,
		},
		{
			description:       "Nodes unhealthy above limit",
			maxUnhealthyNodes: 1,
			nodesUnhealthy:    2,
			expectedReturn:    false,
		},
	}

	for _, tc := range testCases {
		absoluteClusterHealthCalculator := NewAbsoluteClusterHealthCalculator(tc.maxUnhealthyNodes)
		returned := absoluteClusterHealthCalculator(tc.nodesUnhealthy, 0)
		if returned != tc.expectedReturn {
			t.Errorf("Expected: %v Got: %v", tc.expectedReturn, returned)
		}
	}
}

func TestPercentualClusterHealthCalculator(t *testing.T) {
	t.Parallel()

	type testCase struct {
		description              string
		maxUnhealthyNodesPercent uint
		nodesUnhealthy           uint
		totalNodes               uint
		expectedReturn           bool
	}

	testCases := []testCase{
		{
			description:              "Zero nodes unhealthy",
			maxUnhealthyNodesPercent: 1,
			nodesUnhealthy:           0,
			totalNodes:               100,
			expectedReturn:           true,
		},
		{
			description:              "Nodes unhealthy within limit",
			maxUnhealthyNodesPercent: 1,
			nodesUnhealthy:           1,
			totalNodes:               100,
			expectedReturn:           true,
		},
		{
			description:              "Nodes unhealthy above limit",
			maxUnhealthyNodesPercent: 1,
			nodesUnhealthy:           2,
			totalNodes:               100,
			expectedReturn:           false,
		},
		{
			description:              "Nodes unhealthy as non integer percentage just within limit",
			maxUnhealthyNodesPercent: 14,
			nodesUnhealthy:           10,
			totalNodes:               73,
			expectedReturn:           true,
		},
		{
			description:              "Nodes unhealthy as non integer percentage just above limit",
			maxUnhealthyNodesPercent: 13,
			nodesUnhealthy:           10,
			totalNodes:               73,
			expectedReturn:           false,
		},
		{
			description:              "Nodes unhealthy as non integer percentage just within limit",
			maxUnhealthyNodesPercent: 23,
			nodesUnhealthy:           7,
			totalNodes:               31,
			expectedReturn:           true,
		},
		{
			description:              "Nodes unhealthy as non integer percentage just above limit",
			maxUnhealthyNodesPercent: 22,
			nodesUnhealthy:           7,
			totalNodes:               31,
			expectedReturn:           false,
		},
	}

	for _, tc := range testCases {
		absoluteClusterHealthCalculator := NewPercentualClusterHealthCalculator(tc.maxUnhealthyNodesPercent)
		returned := absoluteClusterHealthCalculator(tc.nodesUnhealthy, tc.totalNodes)
		if returned != tc.expectedReturn {
			t.Errorf(`Expected: "%v" Got: "%v"`, tc.expectedReturn, returned)
		}
	}
}

func TestNewClusterHealthCalculator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		description       string
		maxNodesUnhealthy config.MaxNodesUnhealthy
		chcUnhealthyNodes uint
		chcTotalNodes     uint
		chcExpectedOutput bool
		expectErr         bool
		expectedErr       string
	}{
		{
			description:       "Test new percentual calculator with cluster healthy",
			maxNodesUnhealthy: config.MaxNodesUnhealthy("10%"),
			chcUnhealthyNodes: 0,
			chcTotalNodes:     1,
			chcExpectedOutput: true,
			expectErr:         false,
		},
		{
			description:       "Test new absolute calculator with cluster healthy",
			maxNodesUnhealthy: config.MaxNodesUnhealthy("1"),
			chcUnhealthyNodes: 0,
			chcTotalNodes:     1,
			chcExpectedOutput: true,
			expectErr:         false,
		},
		{
			description:       "Test new percentual calculator with cluster unhealthy",
			maxNodesUnhealthy: config.MaxNodesUnhealthy("10%"),
			chcUnhealthyNodes: 2,
			chcTotalNodes:     10,
			chcExpectedOutput: false,
			expectErr:         false,
		},
		{
			description:       "Test new absolute calculator with cluster unhealthy",
			maxNodesUnhealthy: config.MaxNodesUnhealthy("1"),
			chcUnhealthyNodes: 2,
			chcTotalNodes:     10,
			chcExpectedOutput: false,
			expectErr:         false,
		},
		{
			description:       "Test invalid maxNodesUnhealthy",
			maxNodesUnhealthy: config.MaxNodesUnhealthy("a"),
			chcUnhealthyNodes: 2,
			chcTotalNodes:     10,
			expectErr:         true,
			expectedErr:       `can't parse MaxNodesUnhealthy. It should be a number or a percentage. Actual value: a`,
		},
		{
			description:       "Test maxNodesUnhealthy out of range",
			maxNodesUnhealthy: config.MaxNodesUnhealthy("18446744073709551616"),
			chcUnhealthyNodes: 0,
			chcTotalNodes:     0,
			expectErr:         true,
			expectedErr:       `strconv.ParseUint: parsing "18446744073709551616": value out of range`,
		},
	}

	for _, test := range tests {
		chc, err := NewClusterHealthCalculator(test.maxNodesUnhealthy)
		if err == nil && test.expectErr {
			t.Fatalf("Error expected but not returned while testing `%s`", test.description)
		}
		if err == nil {
			output := chc(test.chcUnhealthyNodes, test.chcTotalNodes)

			if output != test.chcExpectedOutput {
				t.Fatalf("Unexpected Cluster Health Calculator output while testing `%s`:\nGot: %v\nExpected: %v", test.description, err, test.expectedErr)
			}
		}
		if err != nil {
			if !test.expectErr {
				t.Fatalf("Error returned but not expected while testing `%s`:\n Got: %s", test.description, err)
			}
			if err.Error() != test.expectedErr {
				t.Fatalf("Unexpected error returned while testing `%s`:\nGot: %v\nExpected: %v", test.description, err, test.expectedErr)
			}
		}
	}
}
