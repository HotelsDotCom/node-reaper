# Kube Node Reaper

Kube Node Reaper reaps unhealthy Kubernetes nodes based on [conditions](https://kubernetes.io/docs/concepts/architecture/nodes/#condition) and [labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/) then drains and optionally deletes the backing virtual server.

### Why

As a Kubernetes cluster grows, the likelihood of nodes becoming unhealthy grows along with it. Kubernetes detects certain node problems automatically and flags the problems as [conditions](https://kubernetes.io/docs/concepts/architecture/nodes/#condition) to node metadata. It's possible to add custom conditions to nodes by writing problem detectors, such as done by [node-problem-detector](https://github.com/kubernetes/node-problem-detector).

Kube Node Reaper subscribes to the philosophy of treating servers as cattle as opposed to pets. Why fix a node if you can decommission it and let kubernetes reschedule the pods?

By using node reaper, you can:

 * Detect and alert on unhealthy nodes using native or extended conditions such as the ones provided by [node-problem-detector](https://github.com/kubernetes/node-problem-detector).
 * Quarantine unhealthy nodes by cordoning and draining while leaving the backing virtual server running for troubleshooting.
 * Remove unhealthy nodes before they cause too much damage.

Node reaper was inspired from experience with node failures in the wild, to name but a few:

 * Docker or kubelet freezing.
 * Malfunctioning cloud instance.
 * Extremely noisy neighbour in the cloud.

### How it works

A Reapable Node is a node that has been detected as a candidate for reaping. The process of detection and reaping of a Reapable Node is described below:

**[1]**: The controller watches for [nodes](https://kubernetes.io/docs/concepts/architecture/nodes/) from the API server. When it finds a new node, it assesses whether it is Reapable using a [govaluate](https://github.com/Knetic/govaluate) expression (reapPredicate).

**[2]**: Cluster health checks are carried out to ensure nodes are only reaped when it's an isolated and not a wider problem. To that end the health of nodes in the cluster is assessed using a different [govaluate](https://github.com/Knetic/govaluate) expression (unhealthyPredicate). If too many nodes in the cluster or same zone are found to be unhealthy node reaper will log and stop at this step.

**[3]**: The node is cordoned and drained. If draining fails an optional force retry is attempted.

**[4]**: The cloud instance backing the node is reaped regardless of whether draining succeeded (unless skipReaping is set to true).

### Draining

Node reaper's draining implementation follows [`kubectl drain`](https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#drain) as closely as possible:

**[1]**: Cordon the node.

**[2]**: Evict or delete all pods concurrently, ignoring DaemonSets.

The configuration file defines whether the pods are evicted or deleted, timeouts and grace periods.

If draining fails and drainForceRetryOnError is set to true, node reaper will retry draining using the deletion api and with pod deletiong grace period set to 0.

### Reaping

Reaping simply deletes the backing virtual machine the node was running on. Currently this is a non-blocking operation, so node reaper won't wait until the backing virtual machine is actually removed.

### Command line options

- **-config**: Defines the node reaper configuration file path. Defaults to $PWD/node-reaper.yaml.

- **-kubeconfig**: Defines the kubernetes client configuration path, only used when running from outside of a kubernetes cluster.

- **-listenaddr**: Address to listen to for prometheus endpoint (/metrics). Defaults to ":8080".

- **-loglevel**: Defines the log level, must be INFO or DEBUG.

- **-plugin**: Node provider plugin files (.so) or lookup directories to load. Non recursive.

- **-version**: Display version and exit.

### Configuration

If any property is not provided, node reaper's defaults are used.

```yaml
---
# reapPredicate defines a govaluate expression which decides if a node is a candidate for reaping.
reapPredicate: |-
  Cordoned                       == false
  && [Labels.kubernetes.io/role] != 'master'
  && [Conditions.Ready]          != 'True'

# unhealthyPredicate defines a govaluate expression which decides if nodes in the cluster are unhealthy. Used for cluster health checks before reaping.
unhealthyPredicate: |-
  Cordoned                       == true
  || [Conditions.OutOfDisk]      != 'False'
  || [Conditions.MemoryPressure] != 'False'
  || [Conditions.DiskPressure]   != 'False'
  || [Conditions.Ready]          != 'True'

# maxNodesUnhealthy defines how many nodes can be unhealthy in the cluster, if more nodes than maxNodesUnhealthy are unhealthy reaping will be cancelled.
# Must be a positive integer or a percentage between 0% and 100%.
maxNodesUnhealthy: 1

# maxNodesUnhealthyInSameZone defines how many nodes can be unhealthy in the same zone of the cluster as the reap candidate, if more nodes than maxNodesUnhealthy are unhealthy reaping will be cancelled.
# Must be a positive integer or a percentage between 0% and 100%.
maxNodesUnhealthyInSameZone: 1

# newNodeGracePeriodSeconds defines the time after a node's creation when it can be reaped. Nodes whose creation timestamp fall within the grace period won't be reaped.
newNodeGracePeriodSeconds: 300

# drainTimeoutSeconds defines the time after which node reaper will fail an unfinished drain attempt.
drainTimeoutSeconds: 310

# drainForceRetryOnError instructs node reaper to retry draining once with podDeletionGracePeriodSeconds set to 0 and useEviction set to false.
drainForceRetryOnError: true

# podDeletionGracePeriodSeconds defines the GracePeriodSeconds on the eviction or deletion call to the kubernetes api.
podDeletionGracePeriodSeconds: 300

# useEviction instructs node reaper to use the pod eviction api when draining as opposed to the deletion api.
useEviction: true

# skipReaping instructs node reaper to never reap the underlying node, stopping after draining.
skipReaping: false

# dryRun instructs node reaper to never drain or reap nodes, logging instead.
dryRun: true
```

### Node Providers

Node Provider plugins implement the apis for deletion of virtual machines from specific virtual machine providers such as aws, gcloud, azure, vmware, openstack, etc. Node reaper comes with an aws plugin out of the box.

Node Providers plugins are loaded using the [plugin](https://golang.org/pkg/plugin/) package which allows writing your own.

#### Loading a Node Provider

Node reaper loads and logs all plugins it finds in $PWD/plugins if no -plugin flags were passed. You can pass plugins or plugin lookup directories as -plugin flags.

#### Implementing a Node Provider

Node providers are loaded with golang's stdlib [plugin](https://golang.org/pkg/plugin/) package. Node provider plugins must declare a variable called NodeProvider which must implement the interface below:

```go
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
```

# Legal
This project is available under the [Apache 2.0 License](http://www.apache.org/licenses/LICENSE-2.0.html).

Copyright 2018 Expedia Inc.
