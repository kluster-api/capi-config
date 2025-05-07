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
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
	"sigs.k8s.io/yaml"
)

func NewCmdCAPZ() *cobra.Command {
	var (
		systemMPMinSize int64
		systemMPMaxSize int64

		userMPMinSize int64
		userMPMaxSize int64
	)
	cmd := &cobra.Command{
		Use:               "capz",
		Short:             "Configure CAPZ config",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}

			var out bytes.Buffer
			var foundCP bool
			var foundUserManagedMP bool
			var foundSysMP bool
			var foundSysManagedMP bool
			var foundUserMP bool
			err = parser.ProcessResources(in, func(ri parser.ResourceInfo) error {
				if ri.Object.GetAPIVersion() == infraApiVersion &&
					ri.Object.GetKind() == "AzureManagedControlPlane" {
					foundCP = true

					if err := SetAzureNetworkConfiguration(ri); err != nil {
						return err
					}

				} else if ri.Object.GetAPIVersion() == infraApiVersion &&
					ri.Object.GetKind() == "AzureManagedMachinePool" {

					mode, ok, err := unstructured.NestedString(ri.Object.UnstructuredContent(), "spec", "mode")
					if err != nil {
						return err
					}
					if !ok {
						return errors.New("mode in spec of AzureManagedMachinePool is missing")
					}

					var minSize int64
					var maxSize int64
					var newName string
					if mode == "System" {
						foundSysManagedMP = true
						minSize = systemMPMinSize
						maxSize = systemMPMaxSize
						newName = "sys0"

					} else if mode == "User" {
						foundUserManagedMP = true
						minSize = userMPMinSize
						maxSize = userMPMaxSize
						newName = deafultMachinePoolName
					}

					if err := SetAzureManagedMPConfiguration(ri, newName, mode, minSize, maxSize); err != nil {
						return err
					}

				} else if ri.Object.GetAPIVersion() == clusterApiVersion &&
					ri.Object.GetKind() == "MachinePool" {

					name, ok, err := unstructured.NestedString(ri.Object.UnstructuredContent(), "metadata", "name")
					if err != nil {
						return err
					}
					if !ok {
						return errors.New("name in MachinePool is missing")
					}
					mode := strings.HasSuffix(name, "pool0")

					var newName string
					var minSize int64
					var maxSize int64
					if mode {
						foundSysMP = true
						minSize = systemMPMinSize
						maxSize = systemMPMaxSize
						newName = "sys0"
					} else {
						foundUserMP = true
						minSize = userMPMinSize
						maxSize = userMPMaxSize
						newName = "default"
					}
					if err := SetMPConfiguration(ri, newName, minSize, maxSize); err != nil {
						return err
					}

				} else if ri.Object.GetAPIVersion() == infraApiVersion &&
					ri.Object.GetKind() == "AzureClusterIdentity" {

					clientSecretName := os.Getenv("AZURE_CLUSTER_IDENTITY_SECRET_NAME")
					clientSecretNamespace := os.Getenv("AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE")

					if clientSecretNamespace != "" && clientSecretName != "" {
						clientSecret := map[string]any{
							"name":      clientSecretName,
							"namespace": clientSecretNamespace,
						}
						if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), clientSecret, "spec", "clientSecret"); err != nil {
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
			if !foundSysManagedMP {
				return errors.New("system AzureManagedMachinePool not found")
			}
			if !foundUserManagedMP {
				return errors.New("user AzureManagedMachinePool not found")
			}
			if !foundSysMP {
				return errors.New("system MachinePool not found")
			}
			if !foundUserMP {
				return errors.New("user MachinePool not found")
			}

			_, err = os.Stdout.Write(out.Bytes())
			return err
		},
	}

	cmd.Flags().Int64Var(&systemMPMinSize, "system-min-size", 1, "Minimum node count for System Machine Pool")
	cmd.Flags().Int64Var(&systemMPMaxSize, "system-max-size", 2, "Minimum node count for System Machine Pool")

	cmd.Flags().Int64Var(&userMPMinSize, "user-min-size", 1, "Minimum node count for User Machine Pool")
	cmd.Flags().Int64Var(&userMPMaxSize, "user-max-size", 5, "Minimum node count for User Machine Pool")
	return cmd
}

func SetAzureManagedMPConfiguration(ri parser.ResourceInfo, name string, mode string, minSize int64, maxSize int64) error {
	if mode == "System" {
		taint := map[string]any{
			"key":    "CriticalAddonsOnly",
			"value":  "true",
			"effect": "NoSchedule",
		}
		taints := []interface{}{taint}
		if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), taints, "spec", "taints"); err != nil {
			return err
		}
	}

	scalingCfg := map[string]any{
		"minSize": minSize,
		"maxSize": maxSize,
	}
	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), scalingCfg, "spec", "scaling"); err != nil {
		return err
	}

	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), name, "metadata", "name"); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), name, "spec", "name"); err != nil {
		return err
	}
	return nil
}

func SetAzureNetworkConfiguration(ri parser.ResourceInfo) error {
	resourceGroupName, ok, err := unstructured.NestedString(ri.Object.UnstructuredContent(), "spec", "resourceGroupName")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("resourceGroupName is missing")
	}
	vNetCidr := os.Getenv("VNET_CIDR")
	subnetCidr := os.Getenv("SUBNET_CIDR")
	if vNetCidr == "" || subnetCidr == "" {
		return nil
	}

	netcfg := map[string]any{
		"name":      resourceGroupName + "-vnet",
		"cidrBlock": vNetCidr,
		"subnet": map[string]any{
			"name":      resourceGroupName + "-subnet",
			"cidrBlock": subnetCidr,
		},
	}
	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), netcfg, "spec", "virtualNetwork"); err != nil {
		return err
	}
	return nil
}
