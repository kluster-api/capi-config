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
	"strconv"

	"encoding/json"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
	"sigs.k8s.io/yaml"
)

func NewCmdCAPZ() *cobra.Command {
	var (
		vNetCidr   string
		subnetCidr string

		systemMinSize int
		systemMaxSize int

		userMinSize int
		userMaxSize int
	)
	cmd := &cobra.Command{
		Use:               "capz",
		Short:             "Configure CAPZ network config",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			if vNetCidr == "" && subnetCidr == "" {
				_, err = os.Stdout.Write(in)
				return err
			}
			if vNetCidr == "" {
				return errors.New("missing --vnet-cidr")
			}
			if subnetCidr == "" {
				return errors.New("missing --subnet-cidr")
			}
			if systemMaxSize == -1 {
				return errors.New("missing --systemMaxSize")
			}
			if systemMinSize == -1 {
				return errors.New("missing --systemMinSize")
			}
			if userMinSize == -1 {
				return errors.New("missing --userMinSize")
			}
			if userMaxSize == -1 {
				return errors.New("missing --userMaxSize")
			}

			var out bytes.Buffer
			var foundCP bool
			var foundUserAMP bool
			var foundSysMP bool
			var foundSysAMP bool
			var foundUserMP bool
			err = parser.ProcessResources(in, func(ri parser.ResourceInfo) error {
				if ri.Object.GetAPIVersion() == "infrastructure.cluster.x-k8s.io/v1beta1" &&
					ri.Object.GetKind() == "AzureManagedControlPlane" {
					foundCP = true

					err := netCfg(ri, vNetCidr, subnetCidr)
					if err != nil {
						return err
					}

				} else if ri.Object.GetAPIVersion() == "infrastructure.cluster.x-k8s.io/v1beta1" &&
					ri.Object.GetKind() == "AzureManagedMachinePool" {

					mode, ok, err := unstructured.NestedString(ri.Object.UnstructuredContent(), "spec", "mode")
					if err != nil {
						return err
					}
					if !ok {
						return errors.New("mode in spec of AzureManagedMachinePool is missing")
					}

					var minSize int
					var maxSize int
					if mode == "System" {
						foundSysAMP = true
						minSize = systemMinSize
						maxSize = systemMaxSize

					} else if mode == "User" {
						foundUserAMP = true
						minSize = userMinSize
						maxSize = userMaxSize
					}
					err = ampCfg(ri, mode, minSize, maxSize)
					if err != nil {
						return err
					}

				} else if ri.Object.GetAPIVersion() == "cluster.x-k8s.io/v1beta1" &&
					ri.Object.GetKind() == "MachinePool" {

					var minSize int
					var maxSize int
					if !foundSysMP {
						foundSysMP = true
						minSize = systemMinSize
						maxSize = systemMaxSize
					} else {
						foundUserMP = true
						minSize = userMinSize
						maxSize = userMaxSize
					}
					err := mpCfg(ri, minSize, maxSize)
					if err != nil {
						return err
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
			if !foundSysAMP {
				return errors.New("System AzureManagedMachinePool not found")
			}
			if !foundUserAMP {
				return errors.New("User AzureManagedMachinePool not found")
			}
			if !foundSysMP {
				return errors.New("System MachinePool not found")
			}
			if !foundUserMP {
				return errors.New("User MachinePool not found")
			}

			_, err = os.Stdout.Write(out.Bytes())
			return err
		},
	}
	cmd.Flags().StringVar(&vNetCidr, "vnet-cidr", "", "CIDR block to be used for vNET")
	cmd.Flags().StringVar(&subnetCidr, "subnet-cidr", "", "CIDR block to be used for subnet")

	cmd.Flags().IntVar(&systemMinSize, "system-min-size", -1, "Minimum node count for System Machine Pool")
	cmd.Flags().IntVar(&systemMaxSize, "system-max-size", -1, "Minimum node count for System Machine Pool")

	cmd.Flags().IntVar(&userMinSize, "user-min-size", -1, "Minimum node count for User Machine Pool")
	cmd.Flags().IntVar(&userMaxSize, "user-max-size", -1, "Minimum node count for User Machine Pool")
	return cmd
}

func mpCfg(ri parser.ResourceInfo, minSize int, maxSize int) error {

	scalingCfg := map[string]any{
		"cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size": strconv.Itoa(minSize),
		"cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size": strconv.Itoa(maxSize),
	}

	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), scalingCfg, "metadata", "annotations"); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), deepCopy(minSize), "spec", "replicas"); err != nil {
		return err
	}
	return nil
}

func ampCfg(ri parser.ResourceInfo, mode string, minSize int, maxSize int) error {

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
		"minSize": deepCopy(minSize),
		"maxSize": deepCopy(maxSize),
	}
	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), scalingCfg, "spec", "scaling"); err != nil {
		return err
	}
	return nil
}

func netCfg(ri parser.ResourceInfo, vNetCidr string, subnetCidr string) error {
	resourceGroupName, ok, err := unstructured.NestedString(ri.Object.UnstructuredContent(), "spec", "resourceGroupName")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("resourceGroupName is missing")
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

func deepCopy(src interface{}) interface{} {
	var copy interface{}
	data, _ := json.Marshal(src)
	json.Unmarshal(data, &copy)
	return copy
}
