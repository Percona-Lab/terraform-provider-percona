package gcp

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

const (
	ClusterResourcesTagName = "percona-cluster-stack-id"
)

func Schema() map[string]*schema.Schema {
	return map[string]*schema.Schema{}
}
