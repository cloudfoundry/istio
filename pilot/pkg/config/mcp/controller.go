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
package coredatamodel

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	mcpclient "istio.io/istio/galley/pkg/mcp/client"
	"istio.io/istio/pilot/pkg/model"
)

var (
	errNotFound      = errors.New("item not found")
	errAlreadyExists = errors.New("item already exists")
	errUnsupported   = errors.New("this operation is not supported by mcp controller")
)

//go:generate counterfeiter -o fakes/logger.go --fake-name Logger . logger
type logger interface {
	Warnf(template string, args ...interface{})
	Infof(template string, args ...interface{})
}

type CoreDataModel interface {
	model.ConfigStoreCache
	mcpclient.Updater
}

type changeEvent struct {
	metaName       string
	resource       proto.Message
	descriptorType string
}

type Controller struct {
	configStore              map[string]map[string]*sync.Map
	eventCh                  chan func(model.Config, model.Event)
	updateCh                 chan changeEvent
	descriptorsByMessageName map[string]model.ProtoSchema
	logger                   logger
}

func NewController(logger logger) CoreDataModel {
	descriptorsByMessageName := make(map[string]model.ProtoSchema, len(model.IstioConfigTypes))
	for _, config := range model.IstioConfigTypes {
		descriptorsByMessageName[config.MessageName] = config
	}

	configStore := make(map[string]map[string]*sync.Map)
	for _, typ := range model.IstioConfigTypes.Types() {
		configStore[typ] = make(map[string]*sync.Map)
	}
	return &Controller{
		configStore:              configStore,
		eventCh:                  make(chan func(model.Config, model.Event)),
		updateCh:                 make(chan changeEvent),
		descriptorsByMessageName: descriptorsByMessageName,
		logger: logger,
	}
}

func (c *Controller) Run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			if _, ok := <-c.eventCh; ok {
				close(c.eventCh)
			}
			return
		case event, ok := <-c.eventCh:
			if ok {
				// TODO: do we need to call the events with any other args?
				event(model.Config{}, model.EventUpdate)
			}
		case change, ok := <-c.updateCh:
			if ok {
				conf, exists := c.Get(change.descriptorType, change.metaName, "")
				if exists {
					conf.Spec = change.resource
					// handle the error by inserting it in a chan
					c.Update(*conf)
				}
			}
		}
	}
}

func (c *Controller) RegisterEventHandler(typ string, handler func(model.Config, model.Event)) {
	c.eventCh <- handler
}

func (c *Controller) ConfigDescriptor() model.ConfigDescriptor {
	return model.IstioConfigTypes
}

func (c *Controller) Get(typ, name, namespace string) (*model.Config, bool) {
	_, ok := c.configStore[typ]
	if !ok {
		c.logger.Infof("Get: config not found for the type %s", typ)
		return nil, false
	}

	ns, exists := c.configStore[typ][namespace]
	if !exists {
		c.logger.Infof("Get: config not found for the type %s", typ)
		return nil, false
	}

	out, exists := ns.Load(name)
	if !exists {
		return nil, false
	}
	config := out.(model.Config)

	return &config, true
}

func (c *Controller) List(typ, namespace string) ([]model.Config, error) {
	_, ok := c.ConfigDescriptor().GetByType(typ)
	if !ok {
		return nil, errors.New(fmt.Sprintf("List: unknown type %s", typ))
	}
	data, exists := c.configStore[typ]
	if !exists {
		c.logger.Infof("List: config not found for the type %s", typ)
		return nil, nil
	}
	out := make([]model.Config, 0, len(c.configStore[typ]))
	if namespace == "" {
		for _, ns := range data {
			ns.Range(func(key, value interface{}) bool {
				out = append(out, value.(model.Config))
				return true
			})
		}
	} else {
		ns, exists := data[namespace]
		if !exists {
			return nil, nil
		}
		ns.Range(func(key, value interface{}) bool {
			out = append(out, value.(model.Config))
			return true
		})
	}
	return out, nil
}

func (c *Controller) Update(config model.Config) (newRevision string, err error) {
	typ := config.Type
	schema, ok := c.ConfigDescriptor().GetByType(typ)
	if !ok {
		return "", errors.New(fmt.Sprintf("Update: unknown type %s", typ))
	}
	if err := schema.Validate(config.Name, config.Namespace, config.Spec); err != nil {
		return "", err
	}

	ns, exists := c.configStore[typ][config.Namespace]
	if !exists {
		return "", errNotFound
	}

	oldConfig, exists := ns.Load(config.Name)
	if !exists {
		return "", errNotFound
	}

	if config.ResourceVersion != oldConfig.(model.Config).ResourceVersion {
		return "", errors.New("old revision")
	}

	rev := time.Now().String()
	config.ResourceVersion = rev
	ns.Store(config.Name, config)
	return rev, nil
}

func (c *Controller) Create(config model.Config) (revision string, err error) {
	typ := config.Type
	schema, ok := c.ConfigDescriptor().GetByType(typ)
	if !ok {
		return "", errors.New("unknown type")
	}
	if err := schema.Validate(config.Name, config.Namespace, config.Spec); err != nil {
		return "", err
	}
	ns, exists := c.configStore[typ][config.Namespace]
	if !exists {
		ns = new(sync.Map)
		c.configStore[typ][config.Namespace] = ns
	}

	_, exists = ns.Load(config.Name)

	if !exists {
		tnow := time.Now()
		config.ResourceVersion = tnow.String()

		// Set the creation timestamp, if not provided.
		if config.CreationTimestamp.Time.IsZero() {
			config.CreationTimestamp.Time = tnow
		}

		ns.Store(config.Name, config)
		return config.ResourceVersion, nil
	}
	return "", errAlreadyExists
}

func (c *Controller) Apply(change *mcpclient.Change) error {
	for _, obj := range change.Objects {
		descriptor, ok := c.descriptorsByMessageName[change.MessageName]
		if !ok {
			return fmt.Errorf("Apply: type not supported %s", change.MessageName)
		}

		c.updateCh <- changeEvent{
			metaName:       obj.Metadata.Name,
			resource:       obj.Resource,
			descriptorType: descriptor.Type,
		}
	}
	return nil
}

func (c *Controller) HasSynced() bool {
	return true
}

func (c *Controller) Delete(typ, name, namespace string) error {
	return errUnsupported
}
