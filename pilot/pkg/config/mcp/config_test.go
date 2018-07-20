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

package mcp_test

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/onsi/gomega"
	mcpapi "istio.io/api/config/mcp/v1alpha1"
	mcpclient "istio.io/istio/galley/pkg/mcp/client"
	mcp "istio.io/istio/pilot/pkg/config/mcp"
)

func TestUpdate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	updater := mcp.Updater{
		// configController mockConfigController
	}

	// 	controller := mcp controller{
	// 		updater: updater
	// 	}

	// 	expect mockcontroller to have received some methods with arguments

	response := mcpapi.MeshConfigResponse{
		VersionInfo: "some-version",
		Envelopes: []mcpapi.Envelope{
			{
				Metadata: &mcpapi.Metadata{},
				Resource: &types.Any{},
			},
		},
	}
	change := &mcpclient.Change{
		MessageName: "some-typeurl",
		Objects:     make([]*mcpclient.Object, 0, len(response.Envelopes)),
	}

	err := updater.Update(change)
	g.Expect(err).To(gomega.BeNil())
	// some expectation on the config controller

}
