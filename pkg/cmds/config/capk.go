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
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
	"sigs.k8s.io/yaml"
)

type machineSpecs struct {
	cpu, socket, threads int64
	memory               string
}

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
			vmImage := os.Getenv("NODE_VM_IMAGE_TEMPLATE")
			clusterName := os.Getenv("CLUSTER_NAME")
			cpCPU, err := strconv.ParseInt(os.Getenv("CONTROL_PLANE_MACHINE_CPU"), 10, 64)
			if err != nil {
				return err
			}
			cpMemory := os.Getenv("CONTROL_PLANE_MACHINE_MEMORY") + "Gi"
			wmCPU, err := strconv.ParseInt(os.Getenv("WORKER_MACHINE_CPU"), 10, 64)
			if err != nil {
				return err
			}
			wmMemory := os.Getenv("WORKER_MACHINE_MEMORY") + "Gi"

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

					if err := addInterfaces(ri); err != nil {
						return err
					}

					if err := addNetworks(ri); err != nil {
						return err
					}

					if strings.HasSuffix(ri.Object.GetName(), "control-plane") {
						if err := setControlPlaneCpuMemory(ri, &machineSpecs{
							cpu:     cpCPU,
							memory:  cpMemory,
							socket:  1,
							threads: 1,
						}); err != nil {
							return err
						}
					} else {
						if err := setWorkerMachineCpuMemory(ri, &machineSpecs{
							cpu:     wmCPU,
							memory:  wmMemory,
							socket:  1,
							threads: 1,
						}); err != nil {
							return err
						}

						if err := replaceVolumes(ri, clusterName); err != nil {
							return err
						}

						if err := addDataVolumeTemplates(ri, clusterName, vmImage); err != nil {
							return err
						}

						if err := replaceDisks(ri); err != nil {
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

func addInterfaces(ri parser.ResourceInfo) error {
	interfaces := []interface{}{
		map[string]interface{}{
			"bridge": map[string]interface{}{},
			"model":  "virtio",
			"name":   "default",
		},
		map[string]interface{}{
			"bridge": map[string]interface{}{},
			"model":  "virtio",
			"name":   "secondary",
		},
	}

	if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), interfaces, "spec", "template", "spec", "virtualMachineTemplate", "spec",
		"template", "spec", "domain", "devices", "interfaces"); err != nil {
		return err
	}
	return nil
}

func addNetworks(ri parser.ResourceInfo) error {
	networks := []interface{}{
		map[string]interface{}{
			"pod":  map[string]interface{}{},
			"name": "default",
		},
		map[string]interface{}{
			"multus": map[string]interface{}{
				"networkName": "default/vmnet",
			},
			"name": "secondary",
		},
	}

	if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), networks, "spec", "template", "spec", "virtualMachineTemplate", "spec",
		"template", "spec", "networks"); err != nil {
		return err
	}

	return nil
}

func addDataVolumeTemplates(ri parser.ResourceInfo, clusterName string, vmImage string) error {
	dataVolumeTemplates := []interface{}{
		map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": clusterName + "-md-0-boot-volume",
			},
			"spec": map[string]interface{}{
				"pvc": map[string]interface{}{
					"accessModes": []interface{}{"ReadWriteOnce"},
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"storage": "50Gi",
						},
					},
					"storageClassName": "hvl",
				},
				"source": map[string]interface{}{
					"registry": map[string]interface{}{
						"url": "docker://" + vmImage,
					},
				},
			},
		},
	}

	if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), dataVolumeTemplates, "spec", "template", "spec",
		"virtualMachineTemplate", "spec", "dataVolumeTemplates"); err != nil {
		return err
	}

	return nil
}

func replaceDisks(ri parser.ResourceInfo) error {
	disks := []interface{}{
		map[string]interface{}{
			"disk": map[string]interface{}{
				"bus": "virtio",
			},
			"name": "dv-volume",
		},
	}

	if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), disks, "spec", "template", "spec", "virtualMachineTemplate", "spec",
		"template", "spec", "domain", "devices", "disks"); err != nil {
		return err
	}

	return nil
}

func replaceVolumes(ri parser.ResourceInfo, clusterName string) error {
	volumes := []interface{}{
		map[string]interface{}{
			"dataVolume": map[string]interface{}{
				"name": clusterName + "-md-0-boot-volume",
			},
			"name": "dv-volume",
		},
	}

	if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), volumes, "spec", "template", "spec", "virtualMachineTemplate", "spec",
		"template", "spec", "volumes"); err != nil {
		return err
	}

	return nil
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

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), "LoadBalancer", "spec", "controlPlaneServiceTemplate", "spec", "type"); err != nil {
		return err
	}
	return nil
}

func setControlPlaneCpuMemory(ri parser.ResourceInfo, specs *machineSpecs) error {
	cpu := map[string]any{
		"cores":   specs.cpu,
		"sockets": specs.socket,
		"threads": specs.threads,
	}
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), cpu, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "cpu"); err != nil {
		return err
	}

	unstructured.RemoveNestedField(ri.Object.UnstructuredContent(), "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "memory")

	resources := map[string]any{
		"cpu":    specs.cpu,
		"memory": specs.memory,
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "limits"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "requests"); err != nil {
		return err
	}
	return nil
}

func setWorkerMachineCpuMemory(ri parser.ResourceInfo, specs *machineSpecs) error {
	cpu := map[string]any{
		"cores":   specs.cpu,
		"sockets": specs.socket,
		"threads": specs.threads,
	}
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), cpu, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "cpu"); err != nil {
		return err
	}
	unstructured.RemoveNestedField(ri.Object.UnstructuredContent(), "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "memory")

	resources := map[string]any{
		"cpu":    specs.cpu,
		"memory": specs.memory,
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "limits"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), resources, "spec", "template", "spec", "virtualMachineTemplate", "spec", "template", "spec", "domain", "resources", "requests"); err != nil {
		return err
	}
	return nil
}
