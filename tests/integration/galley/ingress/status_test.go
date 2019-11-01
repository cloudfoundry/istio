package ingress

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/environment"
	"istio.io/istio/pkg/test/framework/components/environment/kube"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/label"
)

func TestMain(m *testing.M) {
	framework.
		NewSuite("ingress_test", m).
		Label(label.CustomSetup).
		SetupOnEnv(environment.Kube, istio.Setup(nil, setupConfig)).
		Run()
}

const (
	ingressYaml = `
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: galley-ingress
  annotations:
    kubernetes.io/ingress.class: istio
spec:
  rules:
  - host: status.example.com
`
	timeout         = "10s"
	pollingInterval = "1s"
)

func setupConfig(cfg *istio.Config) {
	if cfg == nil {
		return
	}

	cfg.Values["galley.enableServiceDiscovery"] = "true"
}

func TestIngress(t *testing.T) {
	framework.NewTest(t).
		RequiresEnvironment(environment.Kube).
		Run(func(ctx framework.TestContext) {
			g := NewGomegaWithT(t)
			env := ctx.Environment().(*kube.Environment)

			ns := namespace.NewOrFail(t, ctx, namespace.Config{
				Prefix: "galleyingress",
			})

			err := env.ApplyContents(ns.Name(), ingressYaml)
			g.Expect(err).ToNot(HaveOccurred())

			// check ingress resource for lb ip
			gvr := schema.GroupVersionResource{
				Group:    "extensions",
				Version:  "v1beta1",
				Resource: "ingresses",
			}
			name := "galley-ingress"

			getStatusFn := func() string {
				u, err := env.GetUnstructured(gvr, ns.Name(), name)
				if err != nil {
					t.Errorf("Couldn't get status for resource %v", name)
				}
				fmt.Printf("====> object: %+v\n", u.Object)
				status := u.Object["status"].(v1beta1.IngressStatus)
				return status.LoadBalancer.Ingress[0].IP
			}

			g.Eventually(getStatusFn, timeout, pollingInterval).
				Should(ContainSubstring("smething"))
		})
}
