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
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
	"sigs.k8s.io/yaml"
)

func NewCmdCAPG() *cobra.Command {
	var subnetCidr string
	var kubernetesVersion string
	var minSize int64
	var maxSize int64
	var clusterName string

	cmd := &cobra.Command{
		Use:               "capg",
		Short:             "Configure CAPG config",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			if subnetCidr == "" {
				_, err = os.Stdout.Write(in)
				return err
			}

			var out bytes.Buffer
			var foundCP bool
			var foundMP bool
			var foundManagedMP bool
			err = parser.ProcessResources(in, func(ri parser.ResourceInfo) error {
				if ri.Object.GetAPIVersion() == infraApiVersion &&
					ri.Object.GetKind() == "GCPManagedCluster" {
					foundCP = true

					if err = SetGCPNetworkConfiguration(ri, subnetCidr); err != nil {
						return err
					}

				} else if ri.Object.GetAPIVersion() == infraApiVersion &&
					ri.Object.GetKind() == "GCPManagedMachinePool" {
					foundManagedMP = true

					if err = SetGCPManagedMPConfiguration(ri, deafultMachinePoolName, minSize, maxSize); err != nil {
						return err
					}

				} else if ri.Object.GetAPIVersion() == clusterApiVersion &&
					ri.Object.GetKind() == "MachinePool" {
					foundMP = true

					if err = SetMPConfiguration(ri, deafultMachinePoolName, minSize, maxSize); err != nil {
						return err
					}
				} else if ri.Object.GetAPIVersion() == "infrastructure.cluster.x-k8s.io/v1beta1" &&
					ri.Object.GetKind() == "GCPManagedControlPlane" {
					if clusterName != "" {
						if err = unstructured.SetNestedField(ri.Object.UnstructuredContent(), clusterName, "spec", "clusterName"); err != nil {
							return err
						}
					}
					if kubernetesVersion != "" {
						if err = unstructured.SetNestedField(ri.Object.UnstructuredContent(), kubernetesVersion, "spec", "controlPlaneVersion"); err != nil {
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
			if !foundCP {
				return errors.New("control plane not found, check apiVersion")
			}
			if !foundMP {
				return errors.New("MachinePool not found")
			}
			if !foundManagedMP {
				return errors.New("GCPManagedMachinePool not found")
			}
			_, err = os.Stdout.Write(out.Bytes())
			return err
		},
	}
	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "Name of upstream cluster")
	cmd.Flags().StringVar(&subnetCidr, "subnet-cidr", "", "CIDR block to be used for subnet")
	cmd.Flags().StringVar(&kubernetesVersion, "kubernetes-version", "", "Kubernetes Version of Control Plane")
	cmd.Flags().Int64Var(&minSize, "min-count", 3, "Minimum count of nodes in nodepool")
	cmd.Flags().Int64Var(&maxSize, "max-count", 6, "Maximum count of nodes in nodepool")
	return cmd
}

func SetGCPManagedMPConfiguration(ri parser.ResourceInfo, name string, minSize int64, maxSize int64) error {
	scalingCfg := map[string]any{
		"minCount": minSize,
		"maxCount": maxSize,
	}
	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), scalingCfg, "spec", "scaling"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), name, "metadata", "name"); err != nil {
		return err
	}
	return nil
}

func SetGCPNetworkConfiguration(ri parser.ResourceInfo, subnetCidr string) error {
	networkName, ok, err := unstructured.NestedString(ri.Object.UnstructuredContent(), "spec", "network", "name")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("network name is missing")
	}

	region, ok, err := unstructured.NestedString(ri.Object.UnstructuredContent(), "spec", "region")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("region name is missing")
	}

	subnets := []interface{}{
		map[string]any{
			"name":      networkName + "-subnet",
			"region":    region,
			"cidrBlock": subnetCidr,
		},
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), false, "spec", "network", "autoCreateSubnetworks"); err != nil {
		return err
	}
	if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), subnets, "spec", "network", "subnets"); err != nil {
		return err
	}
	return nil
}
