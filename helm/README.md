# node-reaper

node-reaper reaps unhealthy Kubernetes nodes based on metadata then drains and optionally deletes backing virtual servers.

## Introduction

This chart installs a node-reaper deployment on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites
  - Kubernetes 1.4+ with Beta APIs enabled
  - [Helm Registry plugin](https://github.com/app-registry/helm-plugin)

## Configuration

The following tables lists the configurable parameters of the node-reaper chart and their default values.

Parameter | Description | Default
--- | --- | ---
`awsRegion` | (REQUIRED) AWS region in which this ingress controller will operate | `us-west-1`
`clusterName` | (REQUIRED) Resources created by the ALB Ingress controller will be prefixed with this string | `k8s`
`controller.image.repository` | controller container image repository | `quay.io/coreos/alb-ingress-controller`
`controller.image.tag` | controller container image tag | `0.8`
`controller.image.pullPolicy` | controller container image pull policy | `IfNotPresent`
`controller.extraEnv` | map of environment variables to be injected into the controller pod | `{}`
`controller.nodeSelector` | node labels for controller pod assignment | `{}`
`controller.tolerations` | controller pod toleration for taints | `{}`
`controller.podAnnotations` | annotations to be added to controller pod | `{}`
`controller.resources` | controller pod resource requests & limits | `{}`
`controller.service.annotations` | annotations to be added to controller service | `{}`
`defaultBackend.image.repository` | default backend container image repository | `gcr.io/google_containers/defaultbackend`
`defaultBackend.image.tag` | default backend container image tag | `1.2`
`defaultBackend.image.pullPolicy` | default backend container image pull policy | `IfNotPresent`
`defaultBackend.nodeSelector` | node labels for default backend pod assignment | `{}`
`defaultBackend.podAnnotations` | annotations to be added to default backend pod | `{}`
`defaultBackend.replicaCount` | desired number of default backend pods | `1`
`defaultBackend.resources` | default backend pod resource requests & limits | `{}`
`defaultBackend.service.annotations` | annotations to be added to default backend service | `{}`
`rbac.create` | If true, create & use RBAC resources | `true`
`rbac.serviceAccountName` | ServiceAccount ALB ingress controller will use (ignored if rbac.create=true) | `default`
`scope.ingressClass` | If provided, the ALB ingress controller will only act on Ingress resources annotated with this class | `alb`
`scope.singleNamespace` | If true, the ALB ingress controller will only act on Ingress resources in a single namespace | `false` (watch all namespaces)
`scope.watchNamespace` | If scope.singleNamespace=true, the ALB ingress controller will only act on Ingress resources in this namespace | `""` (namespace of the ALB ingress controller)

> **Tip**: You can use the default [values.yaml](values.yaml)
