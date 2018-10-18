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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/HotelsDotCom/node-reaper/pkg/log"
)

// Handler is the Node Reaper handler
type Handler struct {
	nodeReaper *NodeReaper
	logger     log.Logger
}

func newHandler(nr *NodeReaper, logger log.Logger) *Handler {
	return &Handler{
		nodeReaper: nr,
		logger:     logger,
	}
}

// Add handles Add events
func (h Handler) Add(obj runtime.Object) error {
	node := obj.(*corev1.Node)
	err := h.nodeReaper.Run(node)
	if err != nil {
		h.logger.Errorf("Error processing node %q: %s", node.Name, err)
	}
	return nil
}

// Delete ignores Delete events to implement kooper's handler.Handler interface
func (h Handler) Delete(s string) error {
	return nil
}
