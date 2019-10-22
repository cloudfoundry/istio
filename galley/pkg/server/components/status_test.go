package components

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	. "github.com/onsi/gomega"
	"istio.io/istio/galley/pkg/config/source/kube"
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

	args := settings.DefaultArgs()
	args.EnableServiceDiscovery = true

	newInterfaces = func(string) (kube.Interfaces, error) {
		return mk, nil
	}
	tmpDir, err := ioutil.TempDir(os.TempDir(), t.Name())
	g.Expect(err).To(BeNil())

	meshCfgFile := path.Join(tmpDir, "meshcfg.yaml")
	_, err = os.Create(meshCfgFile)
	g.Expect(err).To(BeNil())
	args.MeshConfigFile = meshCfgFile

	s := NewStatusSyncer(args)
	g.Expect(s).ToNot(BeNil())

	err = s.Start()
	g.Expect(err).To(BeNil())
}

func TestStatusWithDisabledServiceDiscovery(t *testing.T) {
	g := NewWithT(t)
	resetPatchTable()
	defer resetPatchTable()

	args := settings.DefaultArgs()
	args.EnableServiceDiscovery = false

	s := NewStatusSyncer(args)
	g.Expect(s).To(BeNil())
}
