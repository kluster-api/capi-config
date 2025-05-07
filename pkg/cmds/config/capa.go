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
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
	"sigs.k8s.io/yaml"
)

const (
	awsManagedControlPlaneKind = "AWSManagedControlPlane"
	awsManagedMachinePoolKind  = "AWSManagedMachinePool"
	machinePoolKind            = "MachinePool"
	clusterKind                = "Cluster"
	controlplaneRoleAnnotation = "eks.amazonaws.com/controlplane-role"
	machinepoolRoleAnnotation  = "eks.amazonaws.com/machinepool-role"
)

func setAWSManagedCPCIDR(ri *parser.ResourceInfo, vpcCidr string) error {
	netcfg := map[string]any{
		"vpc": map[string]any{
			"cidrBlock": vpcCidr,
		},
	}
	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), netcfg, "spec", "network"); err != nil {
		return err
	}
	return nil
}

func setAWSManagedMPScaling(ri *parser.ResourceInfo, name string, minNodeCount, maxNodeCount int64) error {
	scaling := map[string]any{
		"minSize": minNodeCount,
		"maxSize": maxNodeCount,
	}
	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), scaling, "spec", "scaling"); err != nil {
		return err
	}
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), name, "metadata", "name"); err != nil {
		return err
	}
	return nil
}

func setAWSClusterAnnotations(ri *parser.ResourceInfo, managedControlplaneRole, managedMachinepoolRole string) error {
	if managedControlplaneRole != "" {
		if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), managedControlplaneRole, "metadata", "annotations", controlplaneRoleAnnotation); err != nil {
			return err
		}
	}
	if managedMachinepoolRole != "" {
		if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), managedMachinepoolRole, "metadata", "annotations", machinepoolRoleAnnotation); err != nil {
			return err
		}
	}
	return nil
}

type validationHelper struct {
	isFound                 map[string]bool
	managedControlplaneRole string
	managedMachinepoolRole  string
	vpcCidr                 string
	minCount, maxCount      int64
}

func validation(helper validationHelper) error {
	if !helper.isFound[awsManagedControlPlaneKind] {
		if helper.vpcCidr != "" {
			return errors.New("failed to get AWSManagedControlPlane for cidr update")
		}
		if helper.managedControlplaneRole != "" {
			return errors.New("failed to get AWSManagedControlPlane for role configuration")
		}
	}
	if helper.minCount > helper.maxCount {
		return errors.New("max node count can't be less than min node count")
	}
	if helper.managedMachinepoolRole != "" && !helper.isFound[awsManagedMachinePoolKind] {
		return errors.New("failed to get AWSManagedMachinePool for role configuration")
	}
	if !helper.isFound[clusterKind] {
		if helper.managedControlplaneRole != "" || helper.managedMachinepoolRole != "" {
			return errors.New("failed to get Cluster Kind to update annotations")
		}
	}
	return nil
}

func NewCmdCAPA() *cobra.Command {
	var minNodeCount, maxNodeCount int64
	isFound := make(map[string]bool)
	cmd := &cobra.Command{
		Use:               "capa",
		Short:             "Configure CAPA network config",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			vpcCidr := os.Getenv("VPC_CIDR")
			clusterName := os.Getenv("CLUSTER_NAME")
			managedControlplaneRole := os.Getenv("CONTROLPLANE_ROLE")
			ebsCSIDriverVersion := os.Getenv("EBS_CSI_DRIVER_VERSION")
			managedMachinepoolRole := fmt.Sprintf("nodes%s-%s-%s", clusterName, os.Getenv("CLUSTER_NAMESPACE"), os.Getenv("SUFFIX"))
			nodeMachineType := os.Getenv("AWS_NODE_MACHINE_TYPE")

			var out bytes.Buffer
			err = parser.ProcessResources(in, func(ri parser.ResourceInfo) error {
				if ri.Object.GetKind() == awsManagedControlPlaneKind {
					isFound[awsManagedControlPlaneKind] = true
					if vpcCidr != "" {
						err := setAWSManagedCPCIDR(&ri, vpcCidr)
						if err != nil {
							return err
						}
					}
					if managedControlplaneRole != "" {
						if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), managedControlplaneRole, "spec", "roleName"); err != nil {
							return err
						}
					}
					if clusterName != "" {
						if err = unstructured.SetNestedField(ri.Object.UnstructuredContent(), clusterName, "spec", "eksClusterName"); err != nil {
							return err
						}
					}
					addons := []interface{}{
						map[string]any{
							"name":               "aws-ebs-csi-driver",
							"version":            ebsCSIDriverVersion,
							"conflictResolution": "overwrite",
						},
					}
					if err := unstructured.SetNestedSlice(ri.Object.UnstructuredContent(), addons, "spec", "addons"); err != nil {
						return err
					}
				}

				if ri.Object.GetKind() == machinePoolKind {
					isFound[machinePoolKind] = true
					err := SetMPConfiguration(ri, deafultMachinePoolName, minNodeCount, maxNodeCount)
					if err != nil {
						return err
					}
				}

				if ri.Object.GetKind() == awsManagedMachinePoolKind {
					isFound[awsManagedMachinePoolKind] = true
					err := setAWSManagedMPScaling(&ri, deafultMachinePoolName, minNodeCount, maxNodeCount)
					if err != nil {
						return err
					}
					if managedMachinepoolRole != "" {
						if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), managedMachinepoolRole, "spec", "roleName"); err != nil {
							return err
						}
					}
					if nodeMachineType != "" {
						if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), nodeMachineType, "spec", "instanceType"); err != nil {
							return err
						}
					}
				}

				if ri.Object.GetKind() == clusterKind {
					isFound[clusterKind] = true
					err := setAWSClusterAnnotations(&ri, managedControlplaneRole, managedMachinepoolRole)
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

			// configuration operation validation
			err = validation(validationHelper{
				isFound:                 isFound,
				managedControlplaneRole: managedControlplaneRole,
				managedMachinepoolRole:  managedMachinepoolRole,
				vpcCidr:                 vpcCidr,
				minCount:                minNodeCount,
				maxCount:                maxNodeCount,
			})
			if err != nil {
				return err
			}

			_, err = os.Stdout.Write(out.Bytes())
			return err
		},
	}
	cmd.Flags().Int64Var(&minNodeCount, "min-node-count", 1, "Minimum count of nodes in nodepool")
	cmd.Flags().Int64Var(&maxNodeCount, "max-node-count", 6, "Maximum count of nodes in nodepool")
	return cmd
}
