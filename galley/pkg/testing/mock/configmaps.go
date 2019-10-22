// Copyright 2019 Istio Authors
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

package mock

import (
	"fmt"
	"sync"

	apicorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

var _ corev1.ConfigMapInterface = &configMapImpl{}

type configMapImpl struct {
	mux        sync.Mutex
	configMaps map[string]*apicorev1.ConfigMap
	watches    Watches
}

func newConfigMapInterface() corev1.ConfigMapInterface {
	return &configMapImpl{
		configMaps: make(map[string]*apicorev1.ConfigMap),
	}
}

func (n *configMapImpl) Create(obj *apicorev1.ConfigMap) (*apicorev1.ConfigMap, error) {
	n.mux.Lock()
	defer n.mux.Unlock()

	n.configMaps[obj.Name] = obj

	n.watches.Send(watch.Event{
		Type:   watch.Added,
		Object: obj,
	})
	return obj, nil
}

func (n *configMapImpl) Update(obj *apicorev1.ConfigMap) (*apicorev1.ConfigMap, error) {
	n.mux.Lock()
	defer n.mux.Unlock()

	n.configMaps[obj.Name] = obj

	n.watches.Send(watch.Event{
		Type:   watch.Modified,
		Object: obj,
	})
	return obj, nil
}

func (n *configMapImpl) Delete(name string, options *metav1.DeleteOptions) error {
	n.mux.Lock()
	defer n.mux.Unlock()

	obj := n.configMaps[name]
	if obj == nil {
		return fmt.Errorf("unable to delete configMap %s", name)
	}

	delete(n.configMaps, name)

	n.watches.Send(watch.Event{
		Type:   watch.Deleted,
		Object: obj,
	})
	return nil
}

func (n *configMapImpl) List(opts metav1.ListOptions) (*apicorev1.ConfigMapList, error) {
	n.mux.Lock()
	defer n.mux.Unlock()

	out := &apicorev1.ConfigMapList{}

	for _, v := range n.configMaps {
		out.Items = append(out.Items, *v)
	}

	return out, nil
}

func (n *configMapImpl) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	n.mux.Lock()
	defer n.mux.Unlock()

	w := NewWatch()
	n.watches = append(n.watches, w)

	// Send add events for all current resources.
	for _, configMap := range n.configMaps {
		w.Send(watch.Event{
			Type:   watch.Added,
			Object: configMap,
		})
	}

	return w, nil
}

func (n *configMapImpl) UpdateStatus(*apicorev1.ConfigMap) (*apicorev1.ConfigMap, error) {
	panic("not implemented")
}

func (n *configMapImpl) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	panic("not implemented")
}

func (n *configMapImpl) Get(name string, options metav1.GetOptions) (*apicorev1.ConfigMap, error) {
	n.mux.Lock()
	defer n.mux.Unlock()

	return n.configMaps[name], nil
}

func (n *configMapImpl) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *apicorev1.ConfigMap, err error) {
	panic("not implemented")
}

func (n *configMapImpl) PatchStatus(configMapName string, data []byte) (*apicorev1.ConfigMap, error) {
	panic("not implemented")
}

func (n *configMapImpl) Finalize(item *apicorev1.ConfigMap) (*apicorev1.ConfigMap, error) {
	panic("not implemented")
}
