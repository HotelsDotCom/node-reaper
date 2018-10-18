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

package drainer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"

	"github.com/HotelsDotCom/node-reaper/pkg/log"
)

func TestDeletePod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		description               string
		nodeName                  string
		createPodNames            []string
		deletePodNames            []string
		expectedRemainingPodNames []string
		expectedErr               error
	}{
		{
			description:               "Test delete no pods",
			nodeName:                  "testnode",
			createPodNames:            []string{"testpod1", "testpod2"},
			deletePodNames:            []string{},
			expectedRemainingPodNames: []string{"testpod1", "testpod2"},
		},
		{
			description:               "Test delete one pod",
			nodeName:                  "testnode",
			createPodNames:            []string{"testpod1", "testpod2"},
			deletePodNames:            []string{"testpod2"},
			expectedRemainingPodNames: []string{"testpod1"},
		},
		{
			description:               "Test delete all pods",
			nodeName:                  "testnode",
			createPodNames:            []string{"testpod1", "testpod2"},
			deletePodNames:            []string{"testpod1", "testpod2"},
			expectedRemainingPodNames: []string{},
		},
	}

	for _, test := range tests {
		k8scli := testclient.NewSimpleClientset()

		// Create node
		testNode := newNode(test.nodeName)
		createNode(k8scli, testNode)

		// Create pods
		var pods []corev1.Pod
		for _, podName := range test.createPodNames {
			p := newPod(testPodSpec{Name: podName, NodeName: test.nodeName})
			createPod(k8scli, p)
			pods = append(pods, *p)
		}

		// Create NodeDrainer
		logger, _ := log.Dummy{}.New(log.INFO)
		nd := NewNodeDrainer(
			k8scli,
			logger,
			Options{},
			*testNode,
		)

		// Delete pods
		for _, pod := range pods {
			for _, deletePodName := range test.deletePodNames {
				if pod.Name == deletePodName {
					err := nd.deletePodFn(pod, 0)
					if err != nil {
						panic(err)
					}
				}
			}
		}

		// Get pods
		listOptions := metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", test.nodeName),
		}
		actualPods, err := k8scli.CoreV1().Pods("").List(listOptions)
		if err != nil {
			panic(err)
		}
		actualPodNames := []string{}
		for _, pod := range actualPods.Items {
			actualPodNames = append(actualPodNames, pod.Name)
		}

		// Assert expected behaviour
		if !reflect.DeepEqual(actualPodNames, test.expectedRemainingPodNames) {
			t.Fatalf("Pod list returned doesn't match expected list while testing `%s`.\nGot: %v\nExpected: %v", test.description, prettify(actualPodNames), prettify(test.expectedRemainingPodNames))
		}
	}
}

func TestFilterDaemonSets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		description  string
		inputPods    []testPodSpec
		expectedPods []testPodSpec
	}{
		{
			description:  "No daemonset pods to filter",
			inputPods:    []testPodSpec{{Name: "pod1", Kind: "ReplicaSet"}},
			expectedPods: []testPodSpec{{Name: "pod1", Kind: "ReplicaSet"}},
		},
		{
			description: "Only daemonset pods to filter",
			inputPods: []testPodSpec{
				{Name: "pod1", Kind: "DaemonSet"},
				{Name: "pod2", Kind: "DaemonSet"},
			},
			expectedPods: []testPodSpec{},
		},
		{
			description: "One daemonset pod to filter",
			inputPods: []testPodSpec{
				{Name: "pod1", Kind: "DaemonSet"},
				{Name: "pod2", Kind: "ReplicaSet"},
			},
			expectedPods: []testPodSpec{{Name: "pod2", Kind: "ReplicaSet"}},
		},
	}

	for _, test := range tests {
		inputPods := []corev1.Pod{}
		for _, inputPod := range test.inputPods {
			p := newPod(inputPod)
			inputPods = append(inputPods, *p)
		}
		expectedPods := []corev1.Pod{}
		for _, expectedPod := range test.expectedPods {
			p := newPod(expectedPod)
			expectedPods = append(expectedPods, *p)
		}
		outputPods := filterDaemonSets(inputPods)
		if !reflect.DeepEqual(outputPods, expectedPods) {
			t.Fatalf("Unexpected list of filtered pods while testing `%s`.\nGot: %v\nExpected: %v", test.description, prettify(outputPods), prettify(test.expectedPods))
		}
	}
}

type testState struct {
	pods []corev1.Pod
	mux  sync.Mutex
}

func (ts *testState) getPod(name string) corev1.Pod {
	ts.mux.Lock()
	defer ts.mux.Unlock()
	for _, p := range ts.pods {
		if p.Name == name {
			return p
		}
	}
	return corev1.Pod{}
}

func (ts *testState) getPods() []corev1.Pod {
	ts.mux.Lock()
	defer ts.mux.Unlock()
	return ts.pods
}

func (ts *testState) deletePod(name string) {
	ts.mux.Lock()
	defer ts.mux.Unlock()
	for i, p := range ts.pods {
		if p.Name == name {
			ts.pods = append(ts.pods[:i], ts.pods[i+1:]...)
		}
	}
}

func (ts *testState) podExists(name string) bool {
	ts.mux.Lock()
	defer ts.mux.Unlock()
	for _, p := range ts.pods {
		if p.Name == name {
			return true
		}
	}
	return false
}

func TestDrain(t *testing.T) {
	t.Parallel()

	type testFnOptions struct {
		freeze  bool
		latency time.Duration
	}

	type test struct {
		description  string
		inputPods    []testPodSpec
		expectedPods []testPodSpec
		expectErr    bool
		expectedErr  string
		options      Options

		cordonFn                 func(*testState, testFnOptions) cordonFn
		cordonFnOptions          testFnOptions
		supportEvictionFn        func(*testState, testFnOptions) supportEvictionFn
		supportEvictionFnOptions testFnOptions
		getPodFn                 func(*testState, testFnOptions) getPodFn
		getPodFnOptions          testFnOptions
		getPodsFn                func(*testState, testFnOptions) getPodsFn
		getPodsFnOptions         testFnOptions
		evictPodFn               func(*testState, testFnOptions) evictPodFn
		evictPodFnOptions        testFnOptions
		deletePodFn              func(*testState, testFnOptions) deletePodFn
		deletePodFnOptions       testFnOptions
	}

	defaultCordonFn := func(ts *testState, opts testFnOptions) cordonFn {
		return func(node corev1.Node) error {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			return nil
		}
	}

	defaultSupportEvictionFn := func(ts *testState, opts testFnOptions) supportEvictionFn {
		return func() (string, error) {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			return "policy/v1beta1", nil
		}
	}

	defaultGetPodFn := func(ts *testState, opts testFnOptions) getPodFn {
		return func(namespace, name string) (*corev1.Pod, error) {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			if !ts.podExists(name) {
				return &corev1.Pod{}, apierrors.NewNotFound(schema.GroupResource{Group: "pods"}, name)
			}
			pod := ts.getPod(name)
			return &pod, nil
		}
	}

	defaultGetPodsFn := func(ts *testState, opts testFnOptions) getPodsFn {
		return func(nodeName string) ([]corev1.Pod, error) {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			return ts.getPods(), nil
		}
	}

	defaultEvictPodFn := func(ts *testState, opts testFnOptions) evictPodFn {
		return func(pod corev1.Pod, policyGroupVersion string, gracePeriod int64) error {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			if ts.podExists(pod.Name) {
				ts.deletePod(pod.Name)
			}
			return nil
		}
	}

	defaultDeletePodFn := func(ts *testState, opts testFnOptions) deletePodFn {
		return func(pod corev1.Pod, gracePeriod int64) error {
			if opts.freeze {
				select {}
			}
			time.After(opts.latency)
			if ts.podExists(pod.Name) {
				ts.deletePod(pod.Name)
			}
			return nil
		}
	}

	tests := []test{
		{
			description:  "Test no pods to evict/delete",
			inputPods:    []testPodSpec{},
			expectedPods: []testPodSpec{},
			expectErr:    false,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			deletePodFn:       defaultDeletePodFn,
		},
		{
			description: "Test successfully evict pods",
			inputPods: []testPodSpec{
				{Name: "pod1"},
				{Name: "pod2"},
			},
			expectedPods: []testPodSpec{},
			expectErr:    false,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			deletePodFn:       defaultDeletePodFn,
		},
		{
			description: "Test successfully delete pods",
			inputPods: []testPodSpec{
				{Name: "pod1"},
				{Name: "pod2"},
			},
			expectedPods: []testPodSpec{},
			expectErr:    false,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			deletePodFn:       defaultDeletePodFn,
		},
		{
			description:  "Test error evicting pods",
			inputPods:    []testPodSpec{{Name: "pod1"}},
			expectedPods: []testPodSpec{{Name: "pod1"}},
			expectErr:    true,
			expectedErr:  `failed draining after 0 seconds: error attempting to evict pod "pod1": bogus error`,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn: func(ts *testState, opts testFnOptions) evictPodFn {
				return func(pod corev1.Pod, policyGroupVersion string, gracePeriod int64) error {
					return fmt.Errorf("bogus error")
				}
			},
			deletePodFn: defaultDeletePodFn,
		},
		{
			description:  "Test error deleting pods",
			inputPods:    []testPodSpec{{Name: "pod1"}},
			expectedPods: []testPodSpec{{Name: "pod1"}},
			expectErr:    true,
			expectedErr:  `failed draining after 0 seconds: error attempting to delete pod "pod1": bogus error`,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			deletePodFn: func(ts *testState, opts testFnOptions) deletePodFn {
				return func(pod corev1.Pod, gracePeriod int64) error {
					return fmt.Errorf("bogus error")
				}
			},
		},
		{
			description:  "Test error getting pod using eviction",
			inputPods:    []testPodSpec{{Name: "pod1"}},
			expectedPods: []testPodSpec{},
			expectErr:    true,
			expectedErr:  `failed draining after 0 seconds: error waiting for pod "pod1" to terminate: error evicting pod: bogus error`,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn: func(ts *testState, opts testFnOptions) getPodFn {
				return func(namespace, name string) (*corev1.Pod, error) {
					return &corev1.Pod{}, fmt.Errorf("bogus error")
				}
			},
			getPodsFn:   defaultGetPodsFn,
			evictPodFn:  defaultEvictPodFn,
			deletePodFn: defaultDeletePodFn,
		},
		{
			description:  "Test error getting pod not using eviction",
			inputPods:    []testPodSpec{{Name: "pod1"}},
			expectedPods: []testPodSpec{},
			expectErr:    true,
			expectedErr:  `failed draining after 0 seconds: error waiting for pod "pod1" to terminate: error deleting pod: bogus error`,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn: func(ts *testState, opts testFnOptions) getPodFn {
				return func(namespace, name string) (*corev1.Pod, error) {
					return &corev1.Pod{}, fmt.Errorf("bogus error")
				}
			},
			getPodsFn:   defaultGetPodsFn,
			evictPodFn:  defaultEvictPodFn,
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test error not found when evicting pod",
			inputPods: []testPodSpec{
				{Name: "pod1"},
				{Name: "pod2"},
				{Name: "pod3"},
				{Name: "pod4"},
			},
			expectedPods: []testPodSpec{},
			expectErr:    false,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn: func(ts *testState, opts testFnOptions) evictPodFn {
				return func(pod corev1.Pod, policyGroupVersion string, gracePeriod int64) error {
					ts.deletePod(pod.Name)
					return apierrors.NewNotFound(schema.GroupResource{Group: "pods"}, pod.Name)
				}
			},
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test error not found when deleting pod",
			inputPods: []testPodSpec{
				{Name: "pod1"},
				{Name: "pod2"},
				{Name: "pod3"},
				{Name: "pod4"},
			},
			expectedPods: []testPodSpec{},
			expectErr:    false,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			deletePodFn: func(ts *testState, opts testFnOptions) deletePodFn {
				return func(pod corev1.Pod, gracePeriod int64) error {
					ts.deletePod(pod.Name)
					return apierrors.NewNotFound(schema.GroupResource{Group: "pods"}, pod.Name)
				}
			},
		},
		{
			description: "Test error not found when getting pod and using eviction",
			inputPods: []testPodSpec{
				{Name: "pod1"},
				{Name: "pod2"},
				{Name: "pod3"},
				{Name: "pod4"},
			},
			expectedPods: []testPodSpec{},
			expectErr:    false,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn: func(ts *testState, opts testFnOptions) getPodFn {
				return func(namespace, name string) (*corev1.Pod, error) {
					return &corev1.Pod{}, apierrors.NewNotFound(schema.GroupResource{Group: "pods"}, name)
				}
			},
			getPodsFn:   defaultGetPodsFn,
			evictPodFn:  defaultEvictPodFn,
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test pod returned with different UID when getting pod",
			inputPods: []testPodSpec{
				{Name: "pod1"},
				{Name: "pod2"},
				{Name: "pod3"},
				{Name: "pod4"},
			},
			expectedPods: []testPodSpec{},
			expectErr:    false,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn: func(ts *testState, opts testFnOptions) getPodFn {
				return func(namespace, name string) (*corev1.Pod, error) {
					return &corev1.Pod{
						metav1.TypeMeta{},
						metav1.ObjectMeta{
							Name: name,
							UID:  "asd",
						},
						corev1.PodSpec{},
						corev1.PodStatus{},
					}, nil
				}
			},
			getPodsFn:   defaultGetPodsFn,
			evictPodFn:  defaultEvictPodFn,
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test pod never deleted when using eviction",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `failed draining after 0 seconds: drain timed out`,
			options: Options{
				Timeout:                10 * time.Microsecond,
				PodDeletionGracePeriod: 10 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn: func(ts *testState, opts testFnOptions) getPodFn {
				return func(namespace, name string) (*corev1.Pod, error) {
					return &corev1.Pod{
						metav1.TypeMeta{},
						metav1.ObjectMeta{
							Name: name,
						},
						corev1.PodSpec{},
						corev1.PodStatus{},
					}, nil
				}
			},
			getPodsFn: defaultGetPodsFn,
			evictPodFn: func(ts *testState, opts testFnOptions) evictPodFn {
				return func(pod corev1.Pod, policyGroupVersion string, gracePeriod int64) error {
					return nil
				}
			},
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test pod never deleted when not using eviction",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `failed draining after 0 seconds: drain timed out`,
			options: Options{
				Timeout:                10 * time.Microsecond,
				PodDeletionGracePeriod: 10 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn: func(ts *testState, opts testFnOptions) getPodFn {
				return func(namespace, name string) (*corev1.Pod, error) {
					return &corev1.Pod{
						metav1.TypeMeta{},
						metav1.ObjectMeta{
							Name: name,
						},
						corev1.PodSpec{},
						corev1.PodStatus{},
					}, nil
				}
			},
			getPodsFn:  defaultGetPodsFn,
			evictPodFn: defaultEvictPodFn,
			deletePodFn: func(ts *testState, opts testFnOptions) deletePodFn {
				return func(pod corev1.Pod, gracePeriod int64) error {
					return nil
				}
			},
		},
		{
			description: "Test getPodFn hangs when using eviction",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `failed draining after 0 seconds: drain timed out`,
			options: Options{
				Timeout:                10 * time.Microsecond,
				PodDeletionGracePeriod: 10 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodFnOptions:   testFnOptions{latency: 30 * time.Second},
			getPodsFn:         defaultGetPodsFn,
			evictPodFn: func(ts *testState, opts testFnOptions) evictPodFn {
				return func(pod corev1.Pod, policyGroupVersion string, gracePeriod int64) error {
					return nil
				}
			},
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test getPodFn hangs when not using eviction",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `failed draining after 0 seconds: drain timed out`,
			options: Options{
				Timeout:                10 * time.Microsecond,
				PodDeletionGracePeriod: 10 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodFnOptions:   testFnOptions{latency: 30 * time.Second},
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			deletePodFn: func(ts *testState, opts testFnOptions) deletePodFn {
				return func(pod corev1.Pod, gracePeriod int64) error {
					return nil
				}
			},
		},
		{
			description: "Test evictPodFn hangs",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `failed draining after 0 seconds: drain timed out`,
			options: Options{
				Timeout:                10 * time.Microsecond,
				PodDeletionGracePeriod: 10 * time.Millisecond,
				UseEviction:            true,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			evictPodFnOptions: testFnOptions{freeze: true},
			deletePodFn:       defaultDeletePodFn,
		},
		{
			description: "Test deletePodFn hangs",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `failed draining after 0 seconds: drain timed out`,
			options: Options{
				Timeout:                10 * time.Microsecond,
				PodDeletionGracePeriod: 10 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:           defaultCordonFn,
			supportEvictionFn:  defaultSupportEvictionFn,
			getPodFn:           defaultGetPodFn,
			getPodsFn:          defaultGetPodsFn,
			evictPodFn:         defaultEvictPodFn,
			deletePodFn:        defaultDeletePodFn,
			deletePodFnOptions: testFnOptions{freeze: true},
		},
		{
			description: "Test error cordoning",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `error cordoning node: bogus error`,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn: func(ts *testState, opts testFnOptions) cordonFn {
				return func(node corev1.Node) error {
					return fmt.Errorf("bogus error")
				}
			},
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn:         defaultGetPodsFn,
			evictPodFn:        defaultEvictPodFn,
			deletePodFn:       defaultDeletePodFn,
		},
		{
			description: "Test error running supportEvictionFn",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `error getting policy group version: bogus error`,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn: defaultCordonFn,
			supportEvictionFn: func(ts *testState, opts testFnOptions) supportEvictionFn {
				return func() (string, error) {
					return "", fmt.Errorf("bogus error")
				}
			},
			getPodFn:    defaultGetPodFn,
			getPodsFn:   defaultGetPodsFn,
			evictPodFn:  defaultEvictPodFn,
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test error getting pods",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectErr:   true,
			expectedErr: `error getting pods: bogus error`,
			options: Options{
				Timeout:                1000 * time.Millisecond,
				PodDeletionGracePeriod: 100 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn:          defaultGetPodFn,
			getPodsFn: func(ts *testState, opts testFnOptions) getPodsFn {
				return func(nodeName string) ([]corev1.Pod, error) {
					return []corev1.Pod{}, fmt.Errorf("bogus error")
				}
			},
			evictPodFn:  defaultEvictPodFn,
			deletePodFn: defaultDeletePodFn,
		},
		{
			description: "Test getPodFn returns nil",
			inputPods: []testPodSpec{
				{Name: "pod1"},
			},
			expectedPods: []testPodSpec{},
			expectErr:    true,
			expectedErr:  `failed draining after 0 seconds: drain timed out`,
			options: Options{
				Timeout:                10 * time.Microsecond,
				PodDeletionGracePeriod: 10 * time.Millisecond,
				UseEviction:            false,
			},
			cordonFn:          defaultCordonFn,
			supportEvictionFn: defaultSupportEvictionFn,
			getPodFn: func(ts *testState, opts testFnOptions) getPodFn {
				return func(namespace, name string) (*corev1.Pod, error) {
					return nil, nil
				}
			},
			getPodsFn:   defaultGetPodsFn,
			evictPodFn:  defaultEvictPodFn,
			deletePodFn: defaultDeletePodFn,
		},
	}

	for _, test := range tests {
		// Initialise testState
		state := testState{}
		// Initialise NodeDrainer
		k8scli := testclient.NewSimpleClientset()
		testNode := newNode("node")
		createNode(k8scli, testNode)
		logger, _ := log.Dummy{}.New(log.DEBUG)
		nd := NewNodeDrainer(
			k8scli,
			logger,
			test.options,
			*testNode,
		).
			SetCordonFn(test.cordonFn(&state, test.getPodFnOptions)).
			SetSupportEvictionFn(test.supportEvictionFn(&state, test.getPodFnOptions)).
			SetGetPodFn(test.getPodFn(&state, test.getPodFnOptions)).
			SetGetPodsFn(test.getPodsFn(&state, test.getPodsFnOptions)).
			SetEvictPodFn(test.evictPodFn(&state, test.evictPodFnOptions)).
			SetDeletePodFn(test.deletePodFn(&state, test.deletePodFnOptions))

		// Create pods and initialise testState
		inputPods := []corev1.Pod{}
		for _, inputPod := range test.inputPods {
			p := newPod(inputPod)
			inputPods = append(inputPods, *p)
		}
		expectedPods := []corev1.Pod{}
		for _, expectedPod := range test.expectedPods {
			p := newPod(expectedPod)
			expectedPods = append(expectedPods, *p)
		}
		state.pods = inputPods

		err := nd.DrainNode()

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
		if !reflect.DeepEqual(expectedPods, state.pods) {
			t.Fatalf("Remaining pod list doesn't match expected list while testing `%s`.\nGot: %v\nExpected: %v", test.description, prettify(state.pods), prettify(expectedPods))
		}
	}
}

func newNode(name string) *corev1.Node {
	return &corev1.Node{
		metav1.TypeMeta{},
		metav1.ObjectMeta{Name: name},
		corev1.NodeSpec{},
		corev1.NodeStatus{},
	}
}

func createNode(k8scli kubernetes.Interface, n *corev1.Node) {
	k8scli.Core().Nodes().Create(n)
}

type testPodSpec struct {
	Name     string
	NodeName string
	Kind     string
}

func newPod(tps testPodSpec) *corev1.Pod {
	return &corev1.Pod{
		metav1.TypeMeta{},
		metav1.ObjectMeta{
			Name:            tps.Name,
			OwnerReferences: []metav1.OwnerReference{{Kind: tps.Kind}},
		},
		corev1.PodSpec{
			NodeName: tps.NodeName,
		},
		corev1.PodStatus{},
	}
}

func createPod(k8scli kubernetes.Interface, p *corev1.Pod) {
	k8scli.Core().Pods("").Create(p)
}

func prettify(d interface{}) string {
	output, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		panic(err)
	}
	return string(output)
}
