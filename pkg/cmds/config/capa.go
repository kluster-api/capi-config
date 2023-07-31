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

func NewCmdCAPA() *cobra.Command {
	var vpcCidr string
	cmd := &cobra.Command{
		Use:               "capa",
		Short:             "Configure CAPA network config",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			in, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			if vpcCidr == "" {
				_, err = os.Stdout.Write(in)
				return err
			}

			var out bytes.Buffer
			var foundCP bool
			err = parser.ProcessResources(in, func(ri parser.ResourceInfo) error {
				if ri.Object.GetKind() == "AWSManagedControlPlane" {
					foundCP = true

					netcfg := map[string]any{
						"vpc": map[string]any{
							"cidrBlock": vpcCidr,
						},
					}
					if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), netcfg, "spec", "network"); err != nil {
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
	cmd.Flags().StringVar(&vpcCidr, "vpc-cidr", "", "CIDR block to be used for vpc")
	return cmd
}
