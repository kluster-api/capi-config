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
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
)

func SetMPConfiguration(ri parser.ResourceInfo, minSize int64, maxSize int64) error {
	scalingCfg := map[string]any{
		"cluster.x-k8s.io/cluster-api-autoscaler-node-group-min-size": strconv.FormatInt(minSize, 10),
		"cluster.x-k8s.io/cluster-api-autoscaler-node-group-max-size": strconv.FormatInt(maxSize, 10),
	}

	if err := unstructured.SetNestedMap(ri.Object.UnstructuredContent(), scalingCfg, "metadata", "annotations"); err != nil {
		return err
	}
	return nil
}
