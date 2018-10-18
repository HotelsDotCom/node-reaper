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

package main

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	corev1 "k8s.io/api/core/v1"
)

type awsNodeProvider struct {
	ec2Client *ec2.EC2
}

// NodeProvider is the symbol resolved by the plugin package (https://golang.org/pkg/plugin/) when loading nodeReaper
// plugins. It provides a concrete implementation of an aws plugin.
var NodeProvider awsNodeProvider

// Type returns the Node Provider Type
// The string returned must match the prefix of the node's spec.providerID. The prefix is the text before colon, in other words with regex (?:(?!:).)*
// Example: for the node spec below, it has to match aws.
// ---
// spec:
//   externalID: i-0cd1c351f3c0d11a1
//   providerID: aws:///us-west-2a/i-0cd1c351f3c0d11a1
//   unschedulable: true
func (anp *awsNodeProvider) Type() string {
	return "aws"
}

// Reap Reaps a node
func (anp *awsNodeProvider) Reap(node corev1.Node) error {
	anp.ec2Client = newEc2Client()
	return anp.terminateInstance(node.Spec.ExternalID)
}

func (anp *awsNodeProvider) terminateInstance(id string) error {
	dryRun := false
	instanceID := id
	_, err := anp.ec2Client.TerminateInstances(
		&ec2.TerminateInstancesInput{
			DryRun:      &dryRun,
			InstanceIds: []*string{&instanceID},
		},
	)
	if err != nil {
		return err
	}
	return nil
}

func newEc2Client() *ec2.EC2 {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	return ec2.New(sess)
}
