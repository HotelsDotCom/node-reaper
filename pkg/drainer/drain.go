/*
Copyright 2015-2018 The Kubernetes Authors and Expedia Inc.

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

/*
Parts of the code in this file were inspired by the draining logic
originally implmented by the drain command in kubectl:

https://github.com/kubernetes/kubernetes/blob/master/pkg/kubectl/cmd/drain.go
*/

package drainer

import (
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/HotelsDotCom/node-reaper/pkg/log"
)

const (
	evictionKind          = "Eviction"
	evictionSubresource   = "pods/eviction"
	waitForDeleteInterval = time.Second * 1
)

// cordonFn cordons a node
type cordonFn func(node corev1.Node) error

// supportEvictionFn uses Discovery API to find out if the server support eviction subresource
// If supported returns its groupVersion, otherwise returns ""
type supportEvictionFn func() (string, error)

// getPodFn gets a pod based on a namespace and pod name
type getPodFn func(namespace, name string) (*corev1.Pod, error)

// getPodsFn gets all pods for a given node name
type getPodsFn func(nodeName string) ([]corev1.Pod, error)

// evictPodFn runs an pod eviction API call
type evictPodFn func(pod corev1.Pod, policyGroupVersion string, gracePeriod int64) error

// deletePodFn runs a pod deletion API call
type deletePodFn func(pod corev1.Pod, gracePeriod int64) error

// Options define the options for NodeDrainer
type Options struct {
	Timeout                time.Duration
	PodDeletionGracePeriod time.Duration
	UseEviction            bool
}

// NodeDrainer drains a kubernetes node
type NodeDrainer struct {
	Node              corev1.Node
	k8scli            kubernetes.Interface
	logger            log.Logger
	opts              Options
	cordonFn          cordonFn
	supportEvictionFn supportEvictionFn
	getPodFn          getPodFn
	getPodsFn         getPodsFn
	evictPodFn        evictPodFn
	deletePodFn       deletePodFn
}

// NewNodeDrainer initialises a new NodeDrainer
func NewNodeDrainer(k8scli kubernetes.Interface, logger log.Logger, opts Options, node corev1.Node) *NodeDrainer {
	return &NodeDrainer{
		k8scli:            k8scli,
		logger:            logger,
		opts:              opts,
		Node:              node,
		cordonFn:          newCordonFn(k8scli, logger),
		supportEvictionFn: newSupportEvictionFn(k8scli),
		getPodFn:          newGetPodFn(k8scli),
		getPodsFn:         newGetPodsFn(k8scli),
		evictPodFn:        newEvictPodFn(k8scli),
		deletePodFn:       newDeletePodFn(k8scli),
	}
}

// waitForDelete waits forever for the deletion of a pod, and returns nil if the pod is deleted
func (nd *NodeDrainer) waitForDelete(pod corev1.Pod, interval time.Duration, useEviction bool) error {
	// opType is used for logging whether the operation was of type eviction or deletion
	var opType string
	if useEviction {
		opType = "evicting"
	} else {
		opType = "deleting"
	}

	err := wait.PollImmediate(interval, math.MaxInt64, func() (bool, error) {
		p, err := nd.getPodFn(pod.Namespace, pod.Name)
		if apierrors.IsNotFound(err) || (p != nil && p.ObjectMeta.UID != pod.ObjectMeta.UID) {
			nd.logger.Infof("Success %s pod: %q", opType, pod.Name)
			return true, nil
		} else if apierrors.IsTimeout(err) {
			return false, nil
		} else if err != nil {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("error %s pod: %s", opType, err)
	}
	return nil
}

type evictOrDeletePodsOptions struct {
	PolicyGroupVersion string

	// GracePeriod to set in the evict/delete call
	GracePeriod time.Duration

	// If true use eviction API instead of deleting pods
	UseEviction bool
}

func (nd *NodeDrainer) evictOrDeletePods(pods []corev1.Pod, options evictOrDeletePodsOptions) (func() time.Duration, error) {
	startTime := time.Now()
	elapsed := func() time.Duration {
		return time.Since(startTime)
	}

	if len(pods) == 0 {
		return elapsed, nil
	}

	// opType is used for logging whether the operation was of type eviction or deletion
	var opType string
	if options.UseEviction {
		opType = "evict"
	} else {
		opType = "delete"
	}

	doneCh := make(chan bool, len(pods))
	errCh := make(chan error, 1)

	for _, pod := range pods {
		nd.logger.Infof("Attempting to %s pod %q", opType, pod.Name)
		go func(pod corev1.Pod, doneCh chan bool, errCh chan error) {
			var err error
			for {
				if options.UseEviction {
					err = nd.evictPodFn(pod, options.PolicyGroupVersion, int64(options.GracePeriod/time.Second))
				} else {
					err = nd.deletePodFn(pod, int64(options.GracePeriod/time.Second))
				}
				if err == nil {
					break
				} else if apierrors.IsNotFound(err) {
					doneCh <- true
					return
				} else if apierrors.IsTooManyRequests(err) {
					time.Sleep(time.Second)
				} else {
					errCh <- fmt.Errorf("error attempting to %s pod %q: %s", opType, pod.Name, err)
					return
				}
			}
			err = nd.waitForDelete(
				pod,
				waitForDeleteInterval,
				options.UseEviction,
			)
			if err == nil {
				doneCh <- true
			} else {
				errCh <- fmt.Errorf("error waiting for pod %q to terminate: %s", pod.Name, err)
			}
		}(pod, doneCh, errCh)
	}

	doneCount := 0
	for {
		select {
		case err := <-errCh:
			return elapsed, err
		case <-doneCh:
			doneCount++
			if doneCount == len(pods) {
				return elapsed, nil
			}
		case <-time.After(nd.opts.Timeout):
			return elapsed, fmt.Errorf("drain timed out")
		}
	}
}

// filterDaemonSets filters out any pods owned by DaemonSets
func filterDaemonSets(pods []corev1.Pod) []corev1.Pod {
	filteredPods := []corev1.Pod{}
	for _, pod := range pods {
		for _, or := range pod.OwnerReferences {
			if or.Kind != "DaemonSet" {
				filteredPods = append(filteredPods, pod)
			}
		}
	}
	return filteredPods
}

// DrainNode drains a node in a cluster. Its process is as similar as reasonable to the one implemented by kubectl drain.
func (nd *NodeDrainer) DrainNode() error {
	nd.logger.Info("Draining node")
	if err := nd.cordonFn(nd.Node); err != nil {
		return fmt.Errorf("error cordoning node: %s", err)
	}

	policyGroupVersion, err := nd.supportEvictionFn()
	if err != nil {
		return fmt.Errorf("error getting policy group version: %s", err)
	}

	podArray, err := nd.getPodsFn(nd.Node.Name)
	if err != nil {
		return fmt.Errorf("error getting pods: %s", err)
	}

	pods := filterDaemonSets(podArray)

	elapsed, err := nd.evictOrDeletePods(
		pods,
		evictOrDeletePodsOptions{
			PolicyGroupVersion: policyGroupVersion,
			GracePeriod:        nd.opts.PodDeletionGracePeriod,
			UseEviction:        nd.opts.UseEviction,
		},
	)

	if err != nil {
		return fmt.Errorf("failed draining after %d seconds: %s", elapsed()/time.Second, err)
	}

	nd.logger.Infof("Node successfully drained in %d seconds", elapsed()/time.Second)
	return nil
}

// newCordonFn returns a cordonFn for real world usage
func newCordonFn(k8scli kubernetes.Interface, logger log.Logger) cordonFn {
	return func(node corev1.Node) error {
		if node.Spec.Unschedulable == true {
			logger.Info("Node already cordoned")
			return nil
		}
		k8srestcli := k8scli.Core().RESTClient()
		err := k8srestcli.
			Patch(types.StrategicMergePatchType).
			Resource("nodes").
			Name(node.Name).
			Body([]byte(`{"spec":{"unschedulable":true}}`)).
			Do().
			Error()
		if err != nil {
			return err
		}
		logger.Info("Node successfully cordoned")
		return nil
	}
}

// newSupportEvictionFn returns a supportEvictionFn for real world usage
func newSupportEvictionFn(k8scli kubernetes.Interface) supportEvictionFn {
	return func() (string, error) {
		discoveryClient := k8scli.Discovery()
		groupList, err := discoveryClient.ServerGroups()
		if err != nil {
			return "", err
		}
		foundPolicyGroup := false
		var policyGroupVersion string
		for _, group := range groupList.Groups {
			if group.Name == "policy" {
				foundPolicyGroup = true
				policyGroupVersion = group.PreferredVersion.GroupVersion
				break
			}
		}
		if !foundPolicyGroup {
			return "", nil
		}
		resourceList, err := discoveryClient.ServerResourcesForGroupVersion("v1")
		if err != nil {
			return "", err
		}
		for _, resource := range resourceList.APIResources {
			if resource.Name == evictionSubresource && resource.Kind == evictionKind {
				return policyGroupVersion, nil
			}
		}
		return "", nil
	}
}

// newGetPodFn returns a getPodFn for real world usage
func newGetPodFn(k8scli kubernetes.Interface) getPodFn {
	return func(namespace, name string) (*corev1.Pod, error) {
		return k8scli.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
	}
}

// newGetPodsFn returns a getPodsFn for real world usage
func newGetPodsFn(k8scli kubernetes.Interface) getPodsFn {
	return func(nodeName string) ([]corev1.Pod, error) {
		listOptions := metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
		}
		// Pods("") returns pods for all namespaces
		podList, err := k8scli.CoreV1().Pods("").List(listOptions)
		if err != nil {
			return []corev1.Pod{}, err
		}
		return podList.Items, nil
	}
}

// newEvictPodFn returns a evictPodFn for real world usage
func newEvictPodFn(k8scli kubernetes.Interface) evictPodFn {
	return func(pod corev1.Pod, policyGroupVersion string, gracePeriod int64) error {
		deleteOptions := &metav1.DeleteOptions{}
		if gracePeriod >= 0 {
			deleteOptions.GracePeriodSeconds = &gracePeriod
		}
		eviction := &policyv1beta1.Eviction{
			TypeMeta: metav1.TypeMeta{
				APIVersion: policyGroupVersion,
				Kind:       evictionKind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
			DeleteOptions: deleteOptions,
		}
		return k8scli.PolicyV1beta1().Evictions(eviction.Namespace).Evict(eviction)
	}
}

// newDeletePodFn returns a deletePodFn for real world usage
func newDeletePodFn(k8scli kubernetes.Interface) deletePodFn {
	return func(pod corev1.Pod, gracePeriod int64) error {
		uid := pod.UID
		preconditions := metav1.Preconditions{UID: &uid}
		policy := metav1.DeletePropagationBackground
		deleteOptions := metav1.DeleteOptions{
			Preconditions:     &preconditions,
			PropagationPolicy: &policy,
		}
		if gracePeriod >= 0 {
			deleteOptions.GracePeriodSeconds = &gracePeriod
		}
		err := k8scli.Core().Pods(pod.Namespace).Delete(pod.Name, &deleteOptions)
		if err != nil {
			return err
		}
		return nil
	}
}

// SetCordonFn sets nd.supportEvictionFn for mocking
func (nd *NodeDrainer) SetCordonFn(fn cordonFn) *NodeDrainer {
	nd.cordonFn = fn
	return nd
}

// SetSupportEvictionFn sets nd.supportEvictionFn for mocking
func (nd *NodeDrainer) SetSupportEvictionFn(fn supportEvictionFn) *NodeDrainer {
	nd.supportEvictionFn = fn
	return nd
}

// SetGetPodFn sets nd.getPodFn for mocking
func (nd *NodeDrainer) SetGetPodFn(fn getPodFn) *NodeDrainer {
	nd.getPodFn = fn
	return nd
}

// SetGetPodsFn sets nd.getPodsFn for mocking
func (nd *NodeDrainer) SetGetPodsFn(fn getPodsFn) *NodeDrainer {
	nd.getPodsFn = fn
	return nd
}

// SetEvictPodFn sets nd.evictPodFn for mocking
func (nd *NodeDrainer) SetEvictPodFn(fn evictPodFn) *NodeDrainer {
	nd.evictPodFn = fn
	return nd
}

// SetDeletePodFn sets nd.deletePodFn for mocking
func (nd *NodeDrainer) SetDeletePodFn(fn deletePodFn) *NodeDrainer {
	nd.deletePodFn = fn
	return nd
}
