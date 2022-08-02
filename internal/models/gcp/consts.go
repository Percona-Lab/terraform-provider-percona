package gcp

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

const (
	MachineType          = "machine_type"
	KeyPairName          = "key_pair_name"
	PathToKeyPairStorage = "path_to_key_pair_storage"
	ClusterSize          = "cluster_size"

	ClusterResourcesTagName = "percona-cluster-stack-id"
)

func Schema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		MachineType: {
			Type:     schema.TypeString,
			Optional: true,
			Default:  "e2-micro",
		},
		KeyPairName: {
			Type:     schema.TypeString,
			Required: true,
		},
		PathToKeyPairStorage: {
			Type:     schema.TypeString,
			Optional: true,
			Default:  ".",
		},
		ClusterSize: {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  3,
		},
	}
}
