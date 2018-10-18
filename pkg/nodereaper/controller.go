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
	"time"

	"github.com/spotahome/kooper/operator/controller"
	"k8s.io/client-go/kubernetes"

	"github.com/HotelsDotCom/node-reaper/pkg/config"
	"github.com/HotelsDotCom/node-reaper/pkg/log"
	"github.com/HotelsDotCom/node-reaper/pkg/metrics"
)

const (
	resync = 30 * time.Second
)

// New creates a node reaper controller.
func New(cfg *config.Config, k8scli kubernetes.Interface, nodeProviders map[string]NodeProvider, mReg metrics.Instrumenter, logger log.Logger) controller.Controller {
	retr := newNodeRetriever(k8scli)
	nr := newNodeReaper(cfg, k8scli, nodeProviders, mReg, logger)
	hand := newHandler(nr, logger)
	return controller.NewSequential(resync, hand, retr, logger)
}
