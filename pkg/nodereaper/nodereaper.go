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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/HotelsDotCom/node-reaper/pkg/config"
	"github.com/HotelsDotCom/node-reaper/pkg/drainer"
	"github.com/HotelsDotCom/node-reaper/pkg/log"
	"github.com/HotelsDotCom/node-reaper/pkg/metrics"
)

const (
	zoneLabel = "failure-domain.beta.kubernetes.io/zone"
)

// getNodesFn returns a slice with all nodes in a cluster
type getNodesFn func() ([]corev1.Node, error)

// drainNodeFn drains a node
type drainNodeFn func(*corev1.Node, drainer.Options, log.Logger) error

// NodeReaper is the Node Reaper
type NodeReaper struct {
	cfg           config.Config
	k8scli        kubernetes.Interface
	nodeProviders NodeProviders
	mReg          metrics.Instrumenter
	logger        log.Logger

	getNodesFn  getNodesFn
	drainNodeFn drainNodeFn
}

// newNodeReaper returns an initalised Node Reaper
func newNodeReaper(cfg *config.Config, k8scli kubernetes.Interface, nodeProviders NodeProviders, mReg metrics.Instrumenter, logger log.Logger) *NodeReaper {
	return &NodeReaper{
		cfg:           *cfg,
		k8scli:        k8scli,
		nodeProviders: nodeProviders,
		mReg:          mReg,
		logger:        logger,

		getNodesFn:  newGetNodesFn(k8scli),
		drainNodeFn: newDrainNodeFn(k8scli),
	}
}

// Run assesses the reapability of a node, then reaps it if all conditions are met
func (nr NodeReaper) Run(node *corev1.Node) error {
	nr.logger = nr.logger.WithFields([]log.Field{{Field: node.Name, Width: uint(len(node.Name))}}, "", "   ")
	nr.logger.Info("Node event received")

	// Check if node reapable and all safety checks pass
	shouldReap, err := nr.shouldReap(node)
	if err != nil {
		return err
	}
	if !shouldReap {
		return nil
	}

	// Drain node
	err = nr.drain(node)
	if err != nil {
		return err
	}

	// Reap node
	err = nr.reap(node)
	if err != nil {
		return fmt.Errorf("error reaping: %s", err)
	}

	return nil
}

// shouldReap decides if a node is reapable by checking if it meets reapPredicate and
// running all safety checks. Returns true if reapable
func (nr NodeReaper) shouldReap(node *corev1.Node) (bool, error) {
	// Check if node is reapable by checking if it meets reapPredicate
	reapable, err := nr.isNodeReapable(node)
	if err != nil {
		return false, fmt.Errorf("error checking reaping eligibility: %s", err)
	}

	if !reapable {
		nr.logger.Infof("Node does not meet nodePredicate, won't reap")
		return false, nil
	}

	// Safety check: check if node creation within nr.cfg.NewNodeGracePeriod
	if nr.isNodeCreating(node) {
		nr.logger.Infof("Node creation within grace period, won't reap. Created: %s Age: %ds GracePeriod: %ds", node.CreationTimestamp, time.Since(node.CreationTimestamp.Time)/time.Second, nr.cfg.NewNodeGracePeriod/time.Second)
		return false, nil
	}

	// Safety check: check if too many nodes are unhealthy in the cluster
	nodes, err := nr.getNodesFn()
	if err != nil {
		return false, fmt.Errorf("error getting nodes: %s", err)
	}
	unhealthyPredicate, err := NewEvaluablePredicate(nr.cfg.UnhealthyPredicate)
	if err != nil {
		return false, fmt.Errorf("error evaluating unhealthyPredicate: %s", err)
	}
	clusterHealthy, err := nr.isClusterHealthy(nodes, unhealthyPredicate, nr.cfg.MaxNodesUnhealthy, "")
	if err != nil {
		return false, fmt.Errorf("error assessing cluster health: %s", err)
	}
	if !clusterHealthy {
		nr.logger.Warnf("Too many nodes unhealthy in cluster, won't reap")
		return false, nil
	}
	nr.logger.Info("Enough nodes healthy in cluster")

	// Safety check: check if too many nodes are unhealthy in the same zone in the cluster
	zone, ok := node.Labels[zoneLabel]
	if !ok {
		return false, fmt.Errorf("label %q not found in node object", zoneLabel)
	}
	zoneHealthy, err := nr.isClusterHealthy(nodes, unhealthyPredicate, nr.cfg.MaxNodesUnhealthyInSameZone, zone)
	if err != nil {
		return false, fmt.Errorf("error assessing cluster health: %s", err)
	}
	if !zoneHealthy {
		nr.logger.Warnf("Too many nodes unhealthy in same zone, won't reap")
		return false, nil
	}
	nr.logger.Info("Enough nodes healthy in same zone")

	// Node is reapable and all safety checks passed
	nr.logger.Infof("Node is reapable")
	return true, nil
}

// isNodeReapable returns true if reapPredicate returns true for node
func (nr NodeReaper) isNodeReapable(node *corev1.Node) (bool, error) {
	// Assert node is reapable by checking if reapPredicate returns true
	reapPredicate, err := NewEvaluablePredicate(nr.cfg.ReapPredicate)
	if err != nil {
		return false, fmt.Errorf("error creating new EvaluablePredicate for reapPredicate: %s", err)
	}
	reapable, err := reapPredicate.Eval(node, nr.logger)
	if err != nil {
		return false, fmt.Errorf("error evaluating reapPredicate: %s", err)
	}
	if !reapable {
		nr.logger.Info("Node is not reapable")
		return false, nil
	}
	nr.logger.Info("Node is reapable")
	return true, nil
}

// isNodeCreating returns true if node age < NewNodeGracePeriod
func (nr NodeReaper) isNodeCreating(node *corev1.Node) bool {
	if time.Since(node.CreationTimestamp.Time) < nr.cfg.NewNodeGracePeriod {
		return true
	}
	return false
}

// isClusterHealthy returns true if quantity of nodes returning true for unhealthyPredicate < MaxNodesUnhealthy for a zone, if zone == "" it checks the whole cluster
func (nr NodeReaper) isClusterHealthy(nodes []corev1.Node, unhealthyPredicate NodeEvaluablePredicate, maxNodesUnhealthy config.MaxNodesUnhealthy, zone string) (bool, error) {
	var zoneStr string
	var propStr string
	if zone == "" {
		zoneStr = "cluster"
		propStr = "maxNodesUnhealthy"
	} else {
		zoneStr = "zone"
		propStr = "maxNodesUnhealthyInSameZone"
	}
	nr.logger.Infof("Checking %s health with MaxNodesUnhealthy=%s before reaping", zoneStr, maxNodesUnhealthy)
	totalNodesUnhealthy, err := nr.countUnhealthyNodes(&nodes, unhealthyPredicate, zone)
	if err != nil {
		return false, fmt.Errorf("failed counting unhealthy nodes in %s: %s", zoneStr, err)
	}
	nr.logger.Infof("Nodes unhealthy in %s: Total: %d Unhealhty: %d %s: %s", zoneStr, len(nodes), totalNodesUnhealthy, propStr, maxNodesUnhealthy)
	chc, err := NewClusterHealthCalculator(maxNodesUnhealthy)
	if err != nil {
		return false, fmt.Errorf("failed creating ClusterHealthCalculator: %s", err)
	}
	clusterHealthy := chc(totalNodesUnhealthy, uint(len(nodes)))
	if !clusterHealthy {
		return false, nil
	}
	return true, nil
}

// countUnhealthyNodes counts the number of nodes unhealthy for a zone, if zone == "" it checks the whole cluster
func (nr NodeReaper) countUnhealthyNodes(nodes *[]corev1.Node, nep NodeEvaluablePredicate, zone string) (uint, error) {
	var total uint
	for _, node := range *nodes {
		nu, err := nep.Eval(&node, nr.logger)
		if err != nil {
			return 0, fmt.Errorf("error evaluating node conditions: %s", err)
		}
		if nu {
			if zone == "" {
				nr.logger.Warnf("Unhealthy node detected in cluster: %s", node.ObjectMeta.Name)
				total = total + 1
				continue
			}
			if node.Labels[zoneLabel] == zone {
				nr.logger.Warnf("Unhealthy node detected in same zone: %s", node.ObjectMeta.Name)
				total = total + 1
				continue
			}
		}
	}
	return total, nil
}

// drain drains a node
func (nr NodeReaper) drain(node *corev1.Node) error {
	if nr.cfg.DryRun {
		nr.logger.Info("Would've drained but dryRun == true")
		return nil
	}

	nr.mReg.IncNodeDrainTotal()

	// Drain the node
	opts := drainer.Options{
		Timeout:                nr.cfg.DrainTimeout,
		PodDeletionGracePeriod: nr.cfg.PodDeletionGracePeriod,
		UseEviction:            nr.cfg.UseEviction,
	}
	err := nr.drainNodeFn(node, opts, nr.logger)

	// No error
	if err == nil {
		return nil
	}

	// Error but drainForceRetryOnError == false
	if err != nil && !nr.cfg.DrainForceRetryOnError {
		nr.mReg.IncNodeDrainFail()
		return fmt.Errorf("error draining: %s", err)
	}

	// Error but drainForceRetryOnError == true
	nr.logger.Warnf("Failed draining, attempting force retry with grace period = 0: %s", err)
	forceRetryOpts := drainer.Options{
		Timeout:                nr.cfg.DrainTimeout,
		PodDeletionGracePeriod: 0,
		UseEviction:            false,
	}
	err = nr.drainNodeFn(node, forceRetryOpts, nr.logger)

	// Errored again on retry
	if err != nil {
		nr.mReg.IncNodeDrainFail()
		return fmt.Errorf("error force retry draining: %s", err)
	}

	return nil
}

// reap uses a nodeProvider plugin to reap a node
func (nr NodeReaper) reap(node *corev1.Node) error {
	if nr.cfg.DryRun {
		nr.logger.Info("Would've reaped but dryRun == true")
		return nil
	}
	if nr.cfg.SkipReaping {
		nr.logger.Info("skipReaping set to true, not reaping")
		return nil
	}

	nr.mReg.IncNodeReapTotal()

	nr.logger.Info("Attempting to reap node")
	providerName, err := getProvider(node)
	if err != nil {
		nr.mReg.IncNodeReapFail()
		return fmt.Errorf("error getting node provider: %s", err)
	}
	provider, ok := nr.nodeProviders[providerName]
	if !ok {
		nr.mReg.IncNodeReapFail()
		return fmt.Errorf("plugin for provider %q not found", providerName)
	}
	err = provider.Reap(*node)
	if err != nil {
		nr.mReg.IncNodeReapFail()
		return fmt.Errorf("error reaping with provider %q: %s", providerName, err)
	}
	nr.logger.Info("Node successfully reaped")
	return nil
}

func getProvider(n *corev1.Node) (string, error) {
	provider := strings.Split(n.Spec.ProviderID, ":")[0]
	if provider == "" {
		return "", fmt.Errorf("failed extracting provider from NodeSpec.ProviderID")
	}
	return provider, nil
}

// newGetNodesFn returns a getNodesFn for real world usage
func newGetNodesFn(k8scli kubernetes.Interface) getNodesFn {
	return func() ([]corev1.Node, error) {
		nodes, err := k8scli.CoreV1().Nodes().List(metav1.ListOptions{})
		if err != nil {
			return []corev1.Node{}, fmt.Errorf("error getting nodes: %s", err)
		}
		return nodes.Items, nil
	}
}

// newDrainNodeFn returns a drainNodeFn for real world usage
func newDrainNodeFn(k8scli kubernetes.Interface) drainNodeFn {
	return func(node *corev1.Node, opts drainer.Options, logger log.Logger) error {
		nd := drainer.NewNodeDrainer(k8scli, logger, opts, *node)
		return nd.DrainNode()
	}
}

// SetGetNodesFn sets nr.getNodesFn for mocking
func (nr *NodeReaper) SetGetNodesFn(fn getNodesFn) *NodeReaper {
	nr.getNodesFn = fn
	return nr
}

// SetDrainNodeFn sets nr.drainNodeFn for mocking
func (nr *NodeReaper) SetDrainNodeFn(fn drainNodeFn) *NodeReaper {
	nr.drainNodeFn = fn
	return nr
}
