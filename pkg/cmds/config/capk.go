/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
	"sigs.k8s.io/yaml"
)

func NewCmdCAPK() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "capk",
		Short:             "Configure CAPK config",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}

			var out bytes.Buffer

			err = parser.ProcessResources(in, func(ri parser.ResourceInfo) error {
				if ri.Object.GetAPIVersion() == "infrastructure.cluster.x-k8s.io/v1alpha1" &&
					ri.Object.GetKind() == "KubevirtCluster" {
					if err := setControlPlaneServiceTemplate(ri); err != nil {
						return err
					}
				} else if ri.Object.GetAPIVersion() == "infrastructure.cluster.x-k8s.io/v1alpha1" &&
					ri.Object.GetKind() == "KubevirtMachineTemplate" {

					if err := setBootstrapCheckStrategy(ri); err != nil {
						return err
					}

					if strings.HasSuffix(ri.Object.GetName(), "control-plane") {
						if err := setControlPlaneCpuMemory(ri); err != nil {
							return err
						}
					} else {
						if err := setWorkerMachineCpuMemory(ri); err != nil {
							return err
						}
					}
				}

				data, err := yaml.Marshal(ri.Object)
				if err != nil {
					return err
				}
				if out.Len() > 0 {
					out.WriteString("---\n")
				}
				_, err = out.Write(data)
				return err
			})
			if err != nil {
				return err
			}

			_, err = os.Stdout.Write(out.Bytes())
			return err
		},
	}

	return cmd
}

func setBootstrapCheckStrategy(ri parser.ResourceInfo) error {
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), "none", "spec", "template", "spec", "virtualMachineBootstrapCheck", "checkStrategy"); err != nil {
		return err
	}
	return nil
}

func setControlPlaneServiceTemplate(ri parser.ResourceInfo) error {
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), "0.0.0.0", "spec", "controlPlaneServiceTemplate", "metadata", "annotations", "kube-vip.io/loadbalancerIPs"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), "${CLUSTER_NAME}", "spec", "controlPlaneServiceTemplate", "metadata", "generateName"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), "LoadBalancer", "spec", "controlPlaneServiceTemplate", "spec", "type"); err != nil {
		return err
	}
	return nil
}

func setControlPlaneCpuMemory(ri parser.ResourceInfo) error {
	cpu := map[string]any{
		"cores":   "${CONTROL_PLANE_MACHINE_CPU}",
		"sockets": "${SOCKETS}",
		"threads": "${THREADS}",
	}
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), cpu, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "cpu"); err != nil {
		return err
	}
	unstructured.RemoveNestedField(ri.Object.UnstructuredContent(), "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "memory")

	resources := map[string]any{
		"cpu":    "${CONTROL_PLANE_MACHINE_CPU}",
		"memory": "${CONTROL_PLANE_MACHINE_MEMORY}",
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "limits"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "requests"); err != nil {
		return err
	}
	return nil
}

func setWorkerMachineCpuMemory(ri parser.ResourceInfo) error {
	cpu := map[string]any{
		"cores":   "${WORKER_MACHINE_CPU}",
		"sockets": "${SOCKETS}",
		"threads": "${THREADS}",
	}
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), cpu, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "cpu"); err != nil {
		return err
	}
	unstructured.RemoveNestedField(ri.Object.UnstructuredContent(), "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "memory")

	resources := map[string]any{
		"cpu":    "${WORKER_MACHINE_CPU}",
		"memory": "${WORKER_MACHINE_MEMORY}",
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "limits"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "requests"); err != nil {
		return err
	}
	return nil
}
