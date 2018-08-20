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
package coredatamodel_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	google_protobuf "github.com/gogo/protobuf/types"
	"github.com/onsi/gomega"
	mcp "istio.io/api/mcp/v1alpha1"
	networking "istio.io/api/networking/v1alpha3"
	coredatamodel "istio.io/istio/pilot/pkg/config/mcp"
	"istio.io/istio/pilot/pkg/model"
	mcpclient "istio.io/istio/pkg/mcp/client"
)

func TestHasSynced(t *testing.T) {
	t.Skip("Pending: https://github.com/istio/istio/issues/7947")
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	g.Expect(controller.HasSynced()).To(gomega.BeFalse())
}

func TestConfigDescriptor(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	descriptors := controller.ConfigDescriptor()
	g.Expect(descriptors).To(gomega.Equal(model.IstioConfigTypes))
}

func TestListInvalidType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	c, err := controller.List("non-existent", "some-phony-name-space.com")
	g.Expect(c).To(gomega.BeNil())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("List: unknown type"))
}

func TestListValidType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	gateway1 := &networking.Gateway{
		Servers: []*networking.Server{
			{
				Port: &networking.Port{
					Number:   80,
					Name:     "http",
					Protocol: "HTTP",
				},
				Hosts: []string{"*.example.com"},
			},
		},
	}
	gateway2 := &networking.Gateway{
		Servers: []*networking.Server{
			{
				Port: &networking.Port{
					Number:   443,
					Name:     "https",
					Protocol: "HTTP",
				},
				Hosts: []string{"*.secure.example.com"},
			},
		},
	}

	controller := coredatamodel.NewController()
	marshaledGateway1, err := proto.Marshal(gateway1)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message1, err := makeMessage(marshaledGateway1, model.Gateway.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	marshaledGateway2, err := proto.Marshal(gateway2)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message2, err := makeMessage(marshaledGateway2, model.Gateway.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	change := convertToChange(
		[]proto.Message{message1, message2},
		[]string{"some-gateway1", "some-gateway2"},
		model.Gateway.MessageName)

	err = controller.Apply(change)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	// Get gateways in all ("") namespaces
	c, err := controller.List("gateway", "")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(len(c)).To(gomega.Equal(2))
	for _, conf := range c {
		g.Expect(conf.Type).To(gomega.Equal(model.Gateway.Type))
		if conf.Name == "some-gateway1" {
			g.Expect(conf.Spec).To(gomega.Equal(message1))
		} else {
			g.Expect(conf.Name).To(gomega.Equal("some-gateway2"))
			g.Expect(conf.Spec).To(gomega.Equal(message2))
		}
	}
}

func TestApplyInvalidType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	gateway := &networking.Gateway{
		Servers: []*networking.Server{
			&networking.Server{
				Port: &networking.Port{
					Name:     "https",
					Number:   443,
					Protocol: "HTTP",
				},
				Hosts: []string{
					"*",
				},
			},
		},
	}

	marshaledGateway, err := proto.Marshal(gateway)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message, err := makeMessage(marshaledGateway, model.Gateway.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	change := convertToChange([]proto.Message{message}, []string{"some-gateway"}, "bad-type")

	err = controller.Apply(change)
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestApplyValidType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	originalGateway := &networking.Gateway{
		Servers: []*networking.Server{
			{
				Port: &networking.Port{
					Number:   80,
					Name:     "http",
					Protocol: "HTTP",
				},
				Hosts: []string{"*.example.com"},
			},
		},
	}
	marshaledGateway, err := proto.Marshal(originalGateway)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message, err := makeMessage(marshaledGateway, model.Gateway.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	change := convertToChange([]proto.Message{message}, []string{"some-gateway"}, model.Gateway.MessageName)
	err = controller.Apply(change)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	c, err := controller.List("gateway", "")
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(len(c)).To(gomega.Equal(1))
	g.Expect(c[0].Name).To(gomega.Equal("some-gateway"))
	g.Expect(c[0].Type).To(gomega.Equal(model.Gateway.Type))
	g.Expect(c[0].Spec).To(gomega.Equal(message))
}

func TestGetService(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	hostname := "something.example.com"
	serviceEntry := &networking.ServiceEntry{
		Hosts:     []string{hostname},
		Addresses: []string{"172.217.0.0/32"},
		Ports: []*networking.Port{
			{Number: 444, Name: "tcp-444", Protocol: "tcp"},
		},
		Location:   networking.ServiceEntry_MESH_EXTERNAL,
		Resolution: networking.ServiceEntry_NONE,
	}

	marshaledServiceEntry, err := proto.Marshal(serviceEntry)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message, err := makeMessage(marshaledServiceEntry, model.ServiceEntry.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	change := convertToChange([]proto.Message{message}, []string{"some-service-entry"}, model.ServiceEntry.MessageName)

	err = controller.Apply(change)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	service, err := controller.GetService(model.Hostname(hostname))
	g.Expect(err).ToNot(gomega.HaveOccurred())

	g.Expect(service.Hostname).To(gomega.Equal(model.Hostname(serviceEntry.Hosts[0])))
	g.Expect(service.Address).To(gomega.Equal("172.217.0.0"))
	g.Expect(service.Ports[0]).To(gomega.Equal(convertPort(serviceEntry.Ports[0])))
	g.Expect(service.Resolution).To(gomega.Equal(model.Passthrough))
	g.Expect(service.MeshExternal).To(gomega.BeTrue())
	g.Expect(service.Attributes.Name).To(gomega.Equal(serviceEntry.Hosts[0]))
	g.Expect(service.Attributes.Namespace).To(gomega.Equal(""))
}

func TestServices(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	serviceEntry := &networking.ServiceEntry{
		Hosts:     []string{"something.example.com"},
		Addresses: []string{"172.217.0.0/32"},
		Ports: []*networking.Port{
			{Number: 444, Name: "tcp-444", Protocol: "tcp"},
		},
		Location:   networking.ServiceEntry_MESH_EXTERNAL,
		Resolution: networking.ServiceEntry_NONE,
	}

	marshaledServiceEntry, err := proto.Marshal(serviceEntry)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message, err := makeMessage(marshaledServiceEntry, model.ServiceEntry.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	change := convertToChange([]proto.Message{message}, []string{"some-service-entry"}, model.ServiceEntry.MessageName)

	err = controller.Apply(change)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	services, err := controller.Services()
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(len(services)).To(gomega.Equal(1))
	service := services[0]
	g.Expect(service.Hostname).To(gomega.Equal(model.Hostname(serviceEntry.Hosts[0])))
	g.Expect(service.Address).To(gomega.Equal("172.217.0.0"))
	g.Expect(service.Ports[0]).To(gomega.Equal(convertPort(serviceEntry.Ports[0])))
	g.Expect(service.Resolution).To(gomega.Equal(model.Passthrough))
	g.Expect(service.MeshExternal).To(gomega.BeTrue())
	g.Expect(service.Attributes.Name).To(gomega.Equal(serviceEntry.Hosts[0]))
	g.Expect(service.Attributes.Namespace).To(gomega.Equal(""))
}

func TestInstances(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()

	hostname := "something.example.com"
	serviceEntry := &networking.ServiceEntry{
		Hosts:     []string{hostname},
		Addresses: []string{"172.217.0.0"},
		Ports: []*networking.Port{
			{Number: 444, Name: "tcp-444", Protocol: "tcp"},
		},
		Location:   networking.ServiceEntry_MESH_EXTERNAL,
		Resolution: networking.ServiceEntry_DNS,
	}

	marshaledServiceEntry, err := proto.Marshal(serviceEntry)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message, err := makeMessage(marshaledServiceEntry, model.ServiceEntry.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	change := convertToChange([]proto.Message{message}, []string{"some-service-entry"}, model.ServiceEntry.MessageName)

	err = controller.Apply(change)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	serviceInstances, err := controller.Instances(model.Hostname(hostname), []string{"444"}, nil)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(len(serviceInstances)).To(gomega.Equal(1))
	instance := serviceInstances[0]

	desiredEndpoint := model.NetworkEndpoint{
		Address:     hostname,
		Port:        444,
		ServicePort: convertPort(serviceEntry.Ports[0]),
	}

	g.Expect(instance.Endpoint).To(gomega.Equal(desiredEndpoint))
	g.Expect(instance.Labels).To(gomega.BeNil())
	g.Expect(instance.Service.Hostname).To(gomega.Equal(model.Hostname(serviceEntry.Hosts[0])))
	g.Expect(instance.Service.Address).To(gomega.Equal("172.217.0.0"))
	g.Expect(instance.Service.Ports[0]).To(gomega.Equal(convertPort(serviceEntry.Ports[0])))
	g.Expect(instance.Service.Resolution).To(gomega.Equal(model.DNSLB))
	g.Expect(instance.Service.MeshExternal).To(gomega.BeTrue())
	g.Expect(instance.Service.Attributes.Name).To(gomega.Equal(serviceEntry.Hosts[0]))
	g.Expect(instance.Service.Attributes.Namespace).To(gomega.Equal(""))
}

func TestInstancesWithEndpoints(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	controller := coredatamodel.NewController()
	serviceEntry := tcpDNS

	marshaledServiceEntry, err := proto.Marshal(serviceEntry)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	message, err := makeMessage(marshaledServiceEntry, model.ServiceEntry.MessageName)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	change := convertToChange([]proto.Message{message}, []string{"some-service-entry"}, model.ServiceEntry.MessageName)

	err = controller.Apply(change)
	g.Expect(err).ToNot(gomega.HaveOccurred())

	actualServiceInstances, err := controller.Instances("tcpdns.com", []string{"444"}, nil)
	g.Expect(err).ToNot(gomega.HaveOccurred())
	g.Expect(len(actualServiceInstances)).To(gomega.Equal(2))

	port := &networking.Port{
		Number:   444,
		Name:     "tcp-444",
		Protocol: "tcp",
	}
	instanceOne := makeInstance(serviceEntry, "lon.google.com", 444, port, nil, time.Now())
	instanceTwo := makeInstance(serviceEntry, "in.google.com", 444, port, nil, time.Now())
	expectedServiceInstances := []*model.ServiceInstance{instanceOne, instanceTwo}

	sortServiceInstances(actualServiceInstances)
	sortServiceInstances(expectedServiceInstances)
	for i, actualInstance := range actualServiceInstances {
		g.Expect(actualInstance.Endpoint).To(gomega.Equal(expectedServiceInstances[i].Endpoint))
		g.Expect(actualInstance.Labels).To(gomega.BeNil())
		g.Expect(actualInstance.Service.Hostname).To(gomega.Equal(model.Hostname(serviceEntry.Hosts[0])))
		g.Expect(actualInstance.Service.Ports[0]).To(gomega.Equal(convertPort(serviceEntry.Ports[0])))
		g.Expect(actualInstance.Service.Resolution).To(gomega.Equal(model.DNSLB))
		g.Expect(actualInstance.Service.MeshExternal).To(gomega.BeTrue())
		g.Expect(actualInstance.Service.Attributes.Name).To(gomega.Equal(serviceEntry.Hosts[0]))
		g.Expect(actualInstance.Service.Attributes.Namespace).To(gomega.Equal(""))
		g.Expect(actualInstance.Service.Address).To(gomega.Equal(expectedServiceInstances[i].Service.Address))
	}
}

func convertPort(port *networking.Port) *model.Port {
	return &model.Port{
		Name:     port.Name,
		Port:     int(port.Number),
		Protocol: model.ParseProtocol(port.Protocol),
	}
}

func makeMessage(value []byte, responseMessageName string) (proto.Message, error) {
	resource := &google_protobuf.Any{
		TypeUrl: fmt.Sprintf("type.googleapis.com/%s", responseMessageName),
		Value:   value,
	}

	var dynamicAny google_protobuf.DynamicAny
	err := google_protobuf.UnmarshalAny(resource, &dynamicAny)
	if err == nil {
		return dynamicAny.Message, nil
	}

	return nil, err
}

func convertToChange(resources []proto.Message, names []string, responseMessageName string) *mcpclient.Change {
	out := new(mcpclient.Change)
	out.MessageName = responseMessageName
	for i, res := range resources {
		out.Objects = append(out.Objects,
			&mcpclient.Object{
				MessageName: responseMessageName,
				Metadata: &mcp.Metadata{
					Name: names[i],
				},
				Resource: res,
			},
		)
	}
	return out
}

// Test Cases:
// httpStatic
// tcpStatic
// httpDNS
// httpDNSNoEndpoints
// tcpDNS
// httpNoneInternal
// tcpNoneInternal
// multiAddrInternal
// udsLocal
