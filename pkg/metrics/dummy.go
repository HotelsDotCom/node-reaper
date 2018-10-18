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

package metrics

// DummyMetrics implements the Instrumenter for testing
type DummyMetrics struct {
}

// NewDummyMetrics returns a new DummyMetrics object
func NewDummyMetrics() *DummyMetrics {
	d := &DummyMetrics{}
	return d
}

// IncNodeDrainTotal is a fake to implement the Instrumenter interface
func (p *DummyMetrics) IncNodeDrainTotal() {
	return
}

// IncNodeDrainFail is a fake to implement the Instrumenter interface
func (p *DummyMetrics) IncNodeDrainFail() {
	return
}

// IncNodeReapTotal is a fake to implement the Instrumenter interface
func (p *DummyMetrics) IncNodeReapTotal() {
	return
}

// IncNodeReapFail is a fake to implement the Instrumenter interface
func (p *DummyMetrics) IncNodeReapFail() {
	return
}
