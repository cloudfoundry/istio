// Copyright 2017 Istio Authors
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

package components

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	meshconfigapi "istio.io/api/mesh/v1alpha1"
	"istio.io/istio/galley/pkg/server/settings"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/serviceregistry/kube"
	"istio.io/istio/pkg/config/constants"
	"istio.io/pkg/env"
	"istio.io/pkg/log"
)

const (
	updateInterval    = 60 * time.Second
	ingressElectionID = "istio-ingress-controller-leader"
)

type StatusSyncer struct {
	client kubernetes.Interface

	ingressClass        string
	defaultIngressClass string

	// Name of service (ingressgateway default) to find the IP
	ingressService string

	queue    kube.Queue
	informer cache.SharedIndexInformer
	elector  *leaderelection.LeaderElector
	handler  *kube.ChainHandler
	stopCh   chan struct{}
}

var podNameVar = env.RegisterStringVar("POD_NAME", "", "")

func NewStatusSyncer(args *settings.Args) *StatusSyncer {
	if !args.EnableServiceDiscovery {
		return nil
	}

	i, err := newInterfaces(args.KubeConfig)
	if err != nil {
		panic(err)
	}

	client, err := i.KubeClient()
	if err != nil {
		panic("kubeclient")
	}

	meshConfigCache, err := newMeshConfigCache(args.MeshConfigFile)
	if err != nil {
		panic("meshconfigcache")
	}
	meshConfig := meshConfigCache.Get()

	// TODO: replace istio with the ingressclass from the meshconfig
	mode := meshConfig.GetIngressControllerMode()
	class := meshConfig.GetIngressClass()
	ingressClass, defaultIngressClass := convertIngressControllerMode(mode, class)
	electionID := fmt.Sprintf("%v-%v", ingressElectionID, defaultIngressClass)
	if ingressClass != "" {
		electionID = fmt.Sprintf("%v-%v", ingressElectionID, ingressClass)
	}

	handler := &kube.ChainHandler{}
	// queue requires a time duration for a retry delay after a handler error
	queue := kube.NewQueue(1 * time.Second)

	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(opts metaV1.ListOptions) (runtime.Object, error) {
				return client.ExtensionsV1beta1().Ingresses(args.WatchedNamespace).List(opts)
			},
			WatchFunc: func(opts metaV1.ListOptions) (watch.Interface, error) {
				return client.ExtensionsV1beta1().Ingresses(args.WatchedNamespace).Watch(opts)
			},
		},
		&v1beta1.Ingress{}, args.ResyncPeriod, cache.Indexers{},
	)

	st := StatusSyncer{
		client:              client,
		informer:            informer,
		queue:               queue,
		ingressClass:        ingressClass,
		defaultIngressClass: defaultIngressClass,
		ingressService:      meshConfig.IngressService,
		handler:             handler,
	}

	callbacks := leaderelection.LeaderCallbacks{
		OnStartedLeading: func(ctx context.Context) {
			log.Infof("I am the new status update leader")
			go st.queue.Run(ctx.Done())
			err := wait.PollUntil(updateInterval, func() (bool, error) {
				st.queue.Push(kube.NewTask(st.handler.Apply, "Start leading", model.EventUpdate))
				return false, nil
			}, ctx.Done())

			if err != nil {
				log.Errorf("Stop requested")
			}
		},
		OnStoppedLeading: func() {
			log.Infof("I am not status update leader anymore")
		},
		OnNewLeader: func(identity string) {
			log.Infof("New leader elected: %v", identity)
		},
	}

	broadcaster := record.NewBroadcaster()
	hostname, _ := os.Hostname()

	recorder := broadcaster.NewRecorder(scheme.Scheme, coreV1.EventSource{
		Component: "ingress-leader-elector",
		Host:      hostname,
	})
	podName := podNameVar.Get()

	lock := resourcelock.ConfigMapLock{
		ConfigMapMeta: metaV1.ObjectMeta{Namespace: "default", Name: electionID},
		Client:        client.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      podName,
			EventRecorder: recorder,
		},
	}

	ttl := 30 * time.Second
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          &lock,
		LeaseDuration: ttl,
		RenewDeadline: ttl / 2,
		RetryPeriod:   ttl / 4,
		Callbacks:     callbacks,
	})

	if err != nil {
		log.Errorf("unexpected error starting leader election: %v", err)
	}

	st.elector = le

	// Register handler at the beginning
	ingressNamespace := constants.IstioIngressNamespace
	handler.Append(func(obj interface{}, event model.Event) error {
		addrs, err := st.runningAddresses(ingressNamespace)
		if err != nil {
			return err
		}

		return st.updateStatus(sliceToStatus(addrs))
	})

	return &st
}

func (s *StatusSyncer) Start() (err error) {
	s.stopCh = make(chan struct{})
	go s.informer.Run(s.stopCh)
	go s.elector.Run(context.Background())
	return nil
}

func (s *StatusSyncer) Stop() {
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
}

// convertIngressControllerMode converts Ingress controller mode into k8s ingress status syncer ingress class and
// default ingress class. Ingress class and default ingress class are used by the syncer to determine whether or not to
// update the IP of a ingress resource.
func convertIngressControllerMode(mode meshconfigapi.MeshConfig_IngressControllerMode,
	class string) (string, string) {
	var ingressClass, defaultIngressClass string
	switch mode {
	case meshconfigapi.MeshConfig_DEFAULT:
		defaultIngressClass = class
		ingressClass = class
	case meshconfigapi.MeshConfig_STRICT:
		ingressClass = class
	}
	return ingressClass, defaultIngressClass
}

// runningAddresses returns a list of IP addresses and/or FQDN where the
// ingress controller is currently running
func (s *StatusSyncer) runningAddresses(ingressNs string) ([]string, error) {
	addrs := make([]string, 0)

	if s.ingressService != "" {
		svc, err := s.client.CoreV1().Services(ingressNs).Get(s.ingressService, metaV1.GetOptions{})
		if err != nil {
			return nil, err
		}

		if svc.Spec.Type == coreV1.ServiceTypeExternalName {
			addrs = append(addrs, svc.Spec.ExternalName)
			return addrs, nil
		}

		for _, ip := range svc.Status.LoadBalancer.Ingress {
			if ip.IP == "" {
				addrs = append(addrs, ip.Hostname)
			} else {
				addrs = append(addrs, ip.IP)
			}
		}

		addrs = append(addrs, svc.Spec.ExternalIPs...)
		return addrs, nil
	}

	// get information about all the pods running the ingress controller (gateway)
	pods, err := s.client.CoreV1().Pods(ingressNs).List(metaV1.ListOptions{
		// TODO: make it a const or maybe setting ( unless we remove k8s ingress support first)
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": "ingressgateway"}).String(),
	})
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		// only Running pods are valid
		if pod.Status.Phase != coreV1.PodRunning {
			continue
		}

		// Find node external IP
		node, err := s.client.CoreV1().Nodes().Get(pod.Spec.NodeName, metaV1.GetOptions{})
		if err != nil {
			continue
		}

		for _, address := range node.Status.Addresses {
			if address.Type == coreV1.NodeExternalIP {
				if address.Address != "" && !addressInSlice(address.Address, addrs) {
					addrs = append(addrs, address.Address)
				}
			}
		}
	}

	return addrs, nil
}

// updateStatus updates ingress status with the list of IP
func (s *StatusSyncer) updateStatus(status []coreV1.LoadBalancerIngress) error {
	ingressStore := s.informer.GetStore()
	for _, obj := range ingressStore.List() {
		currIng := obj.(*v1beta1.Ingress)
		if !classIsValid(currIng, s.ingressClass, s.defaultIngressClass) {
			continue
		}

		curIPs := currIng.Status.LoadBalancer.Ingress
		sort.SliceStable(status, lessLoadBalancerIngress(status))
		sort.SliceStable(curIPs, lessLoadBalancerIngress(curIPs))

		if ingressSliceEqual(status, curIPs) {
			log.Infof("skipping update of Ingress %v/%v (no change)", currIng.Namespace, currIng.Name)
			return nil
		}

		currIng.Status.LoadBalancer.Ingress = status

		ingClient := s.client.ExtensionsV1beta1().Ingresses(currIng.Namespace)
		_, err := ingClient.UpdateStatus(currIng)
		if err != nil {
			log.Warnf("error updating ingress status: %v", err)
		}
	}

	return nil
}

func addressInSlice(addr string, list []string) bool {
	for _, v := range list {
		if v == addr {
			return true
		}
	}

	return false
}

// classIsValid returns true if the given Ingress either doesn't specify
// the ingress.class annotation, or it's set to the configured in the
// ingress controller.
func classIsValid(ing *v1beta1.Ingress, controller, defClass string) bool {
	// ingress fetched through annotation.
	var ingress string
	if ing != nil && len(ing.GetAnnotations()) != 0 {
		ingress = ing.GetAnnotations()[kube.IngressClassAnnotation]
	}

	// we have 2 valid combinations
	// 1 - ingress with default class | blank annotation on ingress
	// 2 - ingress with specific class | same annotation on ingress
	//
	// and 2 invalid combinations
	// 3 - ingress with default class | fixed annotation on ingress
	// 4 - ingress with specific class | different annotation on ingress
	if ingress == "" && controller == defClass {
		return true
	}

	return ingress == controller
}
func lessLoadBalancerIngress(addrs []coreV1.LoadBalancerIngress) func(int, int) bool {

	return func(a, b int) bool {
		switch strings.Compare(addrs[a].Hostname, addrs[b].Hostname) {
		case -1:
			return true
		case 1:
			return false
		}
		return addrs[a].IP < addrs[b].IP
	}
}

func ingressSliceEqual(lhs, rhs []coreV1.LoadBalancerIngress) bool {
	if len(lhs) != len(rhs) {
		return false
	}

	for i := range lhs {
		if lhs[i].IP != rhs[i].IP {
			return false
		}
		if lhs[i].Hostname != rhs[i].Hostname {
			return false
		}
	}
	return true
}

// sliceToStatus converts a slice of IP and/or hostnames to LoadBalancerIngress
func sliceToStatus(endpoints []string) []coreV1.LoadBalancerIngress {
	lbi := make([]coreV1.LoadBalancerIngress, 0)
	for _, ep := range endpoints {
		if net.ParseIP(ep) == nil {
			lbi = append(lbi, coreV1.LoadBalancerIngress{Hostname: ep})
		} else {
			lbi = append(lbi, coreV1.LoadBalancerIngress{IP: ep})
		}
	}

	return lbi
}
