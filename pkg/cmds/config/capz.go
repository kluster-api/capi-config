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

func NewCmdAPZ() *cobra.Command {
	var (
		vNetCidr   string
		subnetCidr string
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

			/*
				apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
				kind: AzureManagedControlPlane
				metadata:
				  name: shaad-test
				  namespace: default
				spec:
				  identityRef:
				    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
				    kind: AzureClusterIdentity
				    name: cluster-identity
				  location: eastus
				  resourceGroupName: shaad-test
				  sshPublicKey: ""
				  subscriptionID: 1bfc9f66-316d-433e-b13d-c55589f642ca
				  version: v1.24.6
				  virtualNetwork:
				    name: shaad-vNet
				    cidrBlock: 10.2.0.0/16
				    subnet:
				      name: shaad-subnet
				      cidrBlock: 10.2.0.0/24
			*/

			var out bytes.Buffer
			var foundCP bool
			err = parser.ProcessResources(in, func(ri parser.ResourceInfo) error {
				if ri.Object.GetAPIVersion() == "infrastructure.cluster.x-k8s.io/v1beta1" &&
					ri.Object.GetKind() == "AzureManagedControlPlane" {
					foundCP = true

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

			_, err = os.Stdout.Write(out.Bytes())
			return err
		},
	}
	cmd.Flags().StringVar(&vNetCidr, "vnet-cidr", "", "CIDR block to be used for vNET")
	cmd.Flags().StringVar(&subnetCidr, "subnet-cidr", "", "CIDR block to be used for subnet")
	return cmd
}
