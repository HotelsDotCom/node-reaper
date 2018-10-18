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
	"fmt"

	"github.com/HotelsDotCom/node-reaper/pkg/log"

	govaluate "gopkg.in/Knetic/govaluate.v2"
	corev1 "k8s.io/api/core/v1"
)

// NodeEvaluablePredicate holds an evaluableExpression and a node
type NodeEvaluablePredicate struct {
	Expression          string
	EvaluableExpression *govaluate.EvaluableExpression
}

// NewEvaluablePredicate creates an initialised NodeEvaluablePredicate for evaluating Node health
func NewEvaluablePredicate(expression string) (NodeEvaluablePredicate, error) {
	exp, err := govaluate.NewEvaluableExpression(expression)
	if err != nil {
		return NodeEvaluablePredicate{}, err
	}
	return NodeEvaluablePredicate{
		Expression:          expression,
		EvaluableExpression: exp,
	}, nil
}

// Eval populates node conditions into the govaluate expression, sanitises any missing vars, then runs an evaluable expression against a Node
func (nep *NodeEvaluablePredicate) Eval(node *corev1.Node, logger log.Logger) (bool, error) {
	parameters := govaluate.MapParameters{}
	// Load conditions
	for _, condition := range node.Status.Conditions {
		key := fmt.Sprintf("Conditions.%s", string(condition.Type))
		value := string(condition.Status)
		parameters[key] = value
	}
	// Load labels
	for label, value := range node.Labels {
		key := fmt.Sprintf("Labels.%s", string(label))
		parameters[key] = value
	}
	// Load Cordoned
	parameters["Cordoned"] = node.Spec.Unschedulable
	// Blank any user specified parameter that doesn't exist in the Node
	for _, param := range nep.EvaluableExpression.Vars() {
		if _, ok := parameters[param]; !ok {
			logger.Warnf("No value for %q, making it nil", param)
			parameters[param] = nil
		}
	}
	// Log all parameters for debugging
	logger.Debugf("Evaluating node with Predicate: %s", nep.Expression)
	for k, v := range parameters {
		logger.Debugf("Parameter %q = %q", k, v)
	}
	nodeHealthy, err := nep.EvaluableExpression.Eval(parameters)
	if err != nil {
		return false, err
	}
	nodeHealthyBool, ok := nodeHealthy.(bool)
	if !ok {
		return false, fmt.Errorf("invalid predicate, not a boolean expression: %s", nep.Expression)
	}
	return nodeHealthyBool, nil
}
