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

const (
	awsManagedControlPlaneKind = "AWSManagedControlPlane"
	awsManagedMachinePoolKind  = "AWSManagedMachinePool"
	machinePoolKind            = "MachinePool"
	clusterKind                = "Cluster"
	controlplaneRoleAnnotation = "eks.amazonaws.com/controlplane-role"
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

func setAWSManagedCPRole(ri *parser.ResourceInfo, roleName string) error {
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), roleName, "spec", "roleName"); err != nil {
		return err
	}
	return nil
}

func setAWSManagedMPScaling(ri *parser.ResourceInfo, minNodeCount, maxNodeCount int64) error {
	scaling := map[string]any{
		"minSize": minNodeCount,
		"maxSize": maxNodeCount,
	}
	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), scaling, "spec", "scaling"); err != nil {
		return err
	}
	return nil
}

func setAWSManagedMPRole(ri *parser.ResourceInfo, roleName string) error {
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), roleName, "spec", "roleName"); err != nil {
		return err
	}
	return nil
}

func setAWSClusterAnnotations(ri *parser.ResourceInfo, managedControlplaneRole string) error {
	if err := unstructured.SetNestedField(ri.Object.UnstructuredContent(), managedControlplaneRole, "metadata", "annotations", controlplaneRoleAnnotation); err != nil {
		return err
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
	if !helper.isFound[clusterKind] && helper.managedControlplaneRole != "" {
		return errors.New("failed to get ControlPlane to update annotations")
	}
	return nil
}

func NewCmdCAPA() *cobra.Command {
	var vpcCidr, managedControlplaneRole, managedMachinepoolRole string
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
						err := setAWSManagedCPRole(&ri, managedControlplaneRole)
						if err != nil {
							return err
						}
					}
				}

				if ri.Object.GetKind() == machinePoolKind {
					isFound[machinePoolKind] = true
					err := SetMPConfiguration(ri, minNodeCount, maxNodeCount)
					if err != nil {
						return err
					}
				}

				if ri.Object.GetKind() == awsManagedMachinePoolKind {
					isFound[awsManagedMachinePoolKind] = true
					err := setAWSManagedMPScaling(&ri, minNodeCount, maxNodeCount)
					if err != nil {
						return err
					}
					if managedMachinepoolRole != "" {
						err = setAWSManagedMPRole(&ri, managedMachinepoolRole)
						if err != nil {
							return err
						}
					}
				}

				if ri.Object.GetKind() == clusterKind {
					isFound[clusterKind] = true
					err := setAWSClusterAnnotations(&ri, managedControlplaneRole)
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
	cmd.Flags().StringVar(&vpcCidr, "vpc-cidr", "", "CIDR block to be used for vpc")
	cmd.Flags().StringVar(&managedControlplaneRole, "managedcp-role", "", "Managed ControlPlane role for CAPA")
	cmd.Flags().StringVar(&managedMachinepoolRole, "managedmp-role", "", "Managed MachinePool role for CAPA")
	cmd.Flags().Int64Var(&minNodeCount, "min-node-count", 1, "Minimum count of nodes in nodepool")
	cmd.Flags().Int64Var(&maxNodeCount, "max-node-count", 6, "Maximum count of nodes in nodepool")
	return cmd
}
