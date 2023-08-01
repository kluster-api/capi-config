package config

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"kmodules.xyz/client-go/tools/parser"
	"strconv"
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
