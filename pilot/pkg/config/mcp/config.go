// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mcp

import (
	"fmt"
	"sync"

	mcpclient "istio.io/istio/galley/pkg/mcp/client"
	"istio.io/istio/pilot/pkg/model"
)

type Updater struct{}

type store struct {
	descriptor model.ConfigDescriptor
	data       map[string]map[string]*sync.Map
}

// Make creates an in-memory config store from a config descriptor
func Make(descriptor model.ConfigDescriptor) model.ConfigStore {
	out := store{
		descriptor: descriptor,
		data:       make(map[string]map[string]*sync.Map),
	}
	for _, typ := range descriptor.Types() {
		out.data[typ] = make(map[string]*sync.Map)
	}
	return &out
}

func (cr *store) ConfigDescriptor() model.ConfigDescriptor {
	return cr.descriptor
}

func (Updater) Update(change *mcpclient.Change) error {
	fmt.Printf("==>\n%v\n", change)
	return nil
}

func (Updater) Create(change *mcpclient.Change) error {
	fmt.Printf("==>\n%v\n", change)
	return nil
}
