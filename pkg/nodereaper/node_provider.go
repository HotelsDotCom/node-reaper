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
	"plugin"

	corev1 "k8s.io/api/core/v1"
)

// NodeProvider must be implemented by plugins that supply a Node Provider
type NodeProvider interface {
	// Reap deletes a node from a backing node provider such as aws, gcloud or azure.
	Reap(corev1.Node) error

	// Type returns the Node Provider Type
	// The string returned must match the prefix of the node's spec.providerID. The prefix is the text before colon, in other words with regex (?:(?!:).)*
	// Example: for the node spec below, it has to match aws.
	// ---
	// spec:
	//   externalID: i-0cd1c351f3c0d11a1
	//   providerID: aws:///us-west-2a/i-0cd1c351f3c0d11a1
	//   unschedulable: true
	Type() string
}

// NodeProviders serves to standardise how a collection of NodeProvider should be represented
type NodeProviders map[string]NodeProvider

// LoadNodeProviderPlugin loads a NodeProvider plugin
func LoadNodeProviderPlugin(path string) (NodeProvider, error) {
	plugin, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error loading plugin in path %q: %s", path, err)
	}
	np, err := plugin.Lookup("NodeProvider")
	if err != nil {
		return nil, fmt.Errorf("error loading plugin in path %q: %s", path, err)
	}
	var concreteNodeProvider NodeProvider
	concreteNodeProvider, ok := np.(NodeProvider)
	if !ok {
		return nil, fmt.Errorf("imported node provider plugin %q does not implement NodeProvider interface", path)
	}
	return concreteNodeProvider, nil
}
