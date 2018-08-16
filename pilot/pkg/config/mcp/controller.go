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

	"istio.io/fortio/log"
	"istio.io/istio/pilot/pkg/model"
	mcpclient "istio.io/istio/pkg/mcp/client"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	errNotFound      = errors.New("item not found")
	errAlreadyExists = errors.New("item already exists")
	errUnsupported   = errors.New("this operation is not supported by mcp controller")
)

type CoreDataModel interface {
	model.ConfigStoreCache
	model.ServiceDiscovery
	mcpclient.Updater
}

type Controller struct {
	//	serviceEntryStore map[string]
	configStore              map[string]map[string]*sync.Map
	eventCh                  chan func(model.Config, model.Event)
	descriptorsByMessageName map[string]model.ProtoSchema
}

func NewController() CoreDataModel {
	descriptorsByMessageName := make(map[string]model.ProtoSchema, len(model.IstioConfigTypes))
	for _, config := range model.IstioConfigTypes {
		descriptorsByMessageName[config.MessageName] = config
	}

	// Remove this when https://github.com/istio/istio/issues/7947 is done
	configStore := make(map[string]map[string]*sync.Map)
	for _, typ := range model.IstioConfigTypes.Types() {
		configStore[typ] = make(map[string]*sync.Map)
	}
	return &Controller{
		configStore:              configStore,
		eventCh:                  make(chan func(model.Config, model.Event)),
		descriptorsByMessageName: descriptorsByMessageName,
	}
}

func (c *Controller) ConfigDescriptor() model.ConfigDescriptor {
	return model.IstioConfigTypes
}

func (c *Controller) List(typ, namespace string) ([]model.Config, error) {
	_, ok := c.ConfigDescriptor().GetByType(typ)
	if !ok {
		return nil, errors.New(fmt.Sprintf("List: unknown type %s", typ))
	}
	data, exists := c.configStore[typ]
	if !exists {
		log.Infof("List: config not found for the type %s", typ)
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

// TODO: add to the configstore if it does not exist
func (c *Controller) Apply(change *mcpclient.Change) error {
	for _, obj := range change.Objects {
		descriptor, ok := c.descriptorsByMessageName[change.MessageName]
		if !ok {
			return fmt.Errorf("Apply: type not supported %s", change.MessageName)
		}

		conf := model.Config{
			ConfigMeta: model.ConfigMeta{
				Type:              descriptor.Type,
				Group:             descriptor.Group,
				Version:           descriptor.Version,
				Name:              obj.Metadata.Name,
				CreationTimestamp: meta_v1.Time{},
			},
			Spec: obj.Resource,
		}
		_, err := c.create(conf)
		if err != nil {
			return err
		}
	}
	return nil
}

// TODO: pending https://github.com/istio/istio/issues/7947
func (c *Controller) HasSynced() bool {
	// TODO:The configStore already populated with all the keys to avoid nil map issue
	if len(c.configStore) == 0 {
		return false
	}
	for _, descriptor := range c.ConfigDescriptor() {
		if _, ok := c.configStore[descriptor.Type]; !ok {
			return false
		}

	}
	return true
}

func (c *Controller) Run(stop <-chan struct{}) {
	log.Warnf("Run: %s", errUnsupported)
}

func (c *Controller) RegisterEventHandler(typ string, handler func(model.Config, model.Event)) {
	log.Warnf("RegisterEventHandler: %s", errUnsupported)
}

func (c *Controller) Get(typ, name, namespace string) (*model.Config, bool) {
	log.Warnf("Get: %s", errUnsupported)
	return nil, false
}

func (c *Controller) Update(config model.Config) (newRevision string, err error) {
	log.Warnf("Update: %s", errUnsupported)
	return "", errUnsupported
}

func (c *Controller) Create(config model.Config) (revision string, err error) {
	log.Warnf("Create: %s", errUnsupported)
	return "", errUnsupported
}
func (c *Controller) Delete(typ, name, namespace string) error {
	return errUnsupported
}

func (c *Controller) get(typ, name, namespace string) (*model.Config, bool) {
	fmt.Println("type:", typ, "name:", name, "namespace:", namespace)
	_, ok := c.configStore[typ]
	if !ok {
		log.Infof("Get: config not found for the type %s", typ)
		return nil, false
	}

	ns, exists := c.configStore[typ][namespace]
	if !exists {
		log.Infof("Get: config not found for the type %s", typ)
		return nil, false
	}

	out, exists := ns.Load(name)
	if !exists {
		return nil, false
	}
	config := out.(model.Config)

	return &config, true
}

func (c *Controller) create(config model.Config) (revision string, err error) {
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
