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
	"math"

	"github.com/HotelsDotCom/node-reaper/pkg/config"
)

// ClusterHealthCalculator implements heuristics to decide whether too many Nodes are unhealthy before reaping of a node
type ClusterHealthCalculator func(unhealthyNodes uint, totalNodes uint) bool

// NewAbsoluteClusterHealthCalculator calculates cluster health by absolute number of unhealthy Nodes
func NewAbsoluteClusterHealthCalculator(maxUnhealthyNodes uint) ClusterHealthCalculator {
	return func(unhealthyNodes uint, totalNodes uint) bool {
		if unhealthyNodes > maxUnhealthyNodes {
			return false
		}
		return true
	}
}

// NewPercentualClusterHealthCalculator calculates cluster health by percentage of unhealthy Nodes
func NewPercentualClusterHealthCalculator(maxUnhealthyNodesPercent uint) ClusterHealthCalculator {
	return func(unhealthyNodes uint, totalNodes uint) bool {
		un := float64(unhealthyNodes)
		tn := float64(totalNodes)
		unhealthyNodesPercent := uint(math.Ceil(un * 100 / tn))
		if unhealthyNodesPercent > maxUnhealthyNodesPercent {
			return false
		}
		return true
	}
}

// NewClusterHealthCalculator creates a cluster health calculator of type in pseudo enum noderepaer.MaxNodesUnhealthyType
func NewClusterHealthCalculator(mnu config.MaxNodesUnhealthy) (ClusterHealthCalculator, error) {
	mnuType, err := mnu.Type()
	if err != nil {
		return nil, err
	}
	mnuUint, err := mnu.Uint()
	if err != nil {
		return nil, err
	}
	var chc ClusterHealthCalculator
	if mnuType == config.PERCENTUAL {
		chc = NewPercentualClusterHealthCalculator(mnuUint)
	} else if mnuType == config.ABSOLUTE {
		chc = NewAbsoluteClusterHealthCalculator(mnuUint)
	}
	return chc, nil
}
