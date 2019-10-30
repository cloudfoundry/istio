package components

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	. "github.com/onsi/gomega"
	meshconfigapi "istio.io/api/mesh/v1alpha1"
	"istio.io/istio/galley/pkg/config/source/kube"
	meshconfig "istio.io/istio/galley/pkg/meshconfig"
	"istio.io/istio/galley/pkg/server/settings"
	"istio.io/istio/galley/pkg/testing/mock"
)

func TestStatus(t *testing.T) {
	g := NewWithT(t)
	resetPatchTable()
	defer resetPatchTable()

	mk := mock.NewKube()
	cl := fake.NewSimpleDynamicClient(k8sRuntime.NewScheme())
	mk.AddResponse(cl, nil)

	args := defaultSyncArgs(*g)

	newInterfaces = func(string) (kube.Interfaces, error) {
		return mk, nil
	}

	s := NewStatusSyncer(args)
	g.Expect(s).ToNot(BeNil())

	err := s.Start()
	g.Expect(err).ToNot(HaveOccurred())

	s.Stop()
}

func TestNewStatusSyncerWithErrors(t *testing.T) {
	g := NewWithT(t)
	resetPatchTable()
	defer resetPatchTable()

	mk := mock.NewKube()
	cl := fake.NewSimpleDynamicClient(k8sRuntime.NewScheme())
	mk.AddResponse(cl, nil)

	newInterfaces = func(string) (kube.Interfaces, error) {
		return mk, nil
	}

	cases := []struct {
		description     string
		newInterfaces   func(string) (kube.Interfaces, error)
		meshConfigCache func(string) (meshconfig.Cache, error)
	}{
		{
			description: "no kube interface",
			newInterfaces: func(string) (kube.Interfaces, error) {
				return nil, fmt.Errorf("error getting kube interface")
			},
		},
		{
			description: "mesh config cache error",
			meshConfigCache: func(string) (meshconfig.Cache, error) {
				return nil, fmt.Errorf("error getting mesh config")
			},
		},
	}

	for _, c := range cases {
		args := defaultSyncArgs(*g)

		if c.newInterfaces != nil {
			newInterfaces = c.newInterfaces
		}

		if c.meshConfigCache != nil {
			newMeshConfigCache = c.meshConfigCache
		}

		s := NewStatusSyncer(args)
		g.Expect(s).To(BeNil())
	}

}

func defaultSyncArgs(g GomegaWithT) *settings.Args {
	args := settings.DefaultArgs()
	args.EnableServiceDiscovery = true

	tmpDir, _ := ioutil.TempDir(os.TempDir(), "status-syncer")
	meshCfgFile := path.Join(tmpDir, "meshcfg.yaml")
	_, err := os.Create(meshCfgFile)
	g.Expect(err).ToNot(HaveOccurred())

	args.MeshConfigFile = meshCfgFile

	return args
}

func TestShouldUpdateStatus(t *testing.T) {
	g := NewWithT(t)

	cases := []struct {
		mode                   meshconfigapi.MeshConfig_IngressControllerMode
		annotation             string
		meshConfigIngressClass string
		update                 bool
	}{
		{
			mode:                   meshconfigapi.MeshConfig_DEFAULT,
			annotation:             "",
			meshConfigIngressClass: "some-class",
			update:                 true,
		},
		{
			mode:                   meshconfigapi.MeshConfig_DEFAULT,
			annotation:             "some-annotation",
			meshConfigIngressClass: "some-class",
			update:                 false,
		},
		{
			mode:                   meshconfigapi.MeshConfig_STRICT,
			annotation:             "some-specific-class",
			meshConfigIngressClass: "some-specific-class",
			update:                 true,
		},
		{
			mode:                   meshconfigapi.MeshConfig_STRICT,
			annotation:             "some-annotation",
			meshConfigIngressClass: "some-class",
			update:                 false,
		},
	}

	for _, c := range cases {
		result := shouldUpdateStatus(c.mode, c.annotation, c.meshConfigIngressClass)
		g.Expect(result).To(Equal(c.update))
	}
}
