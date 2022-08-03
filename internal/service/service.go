package service

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"terraform-percona/internal/utils"
)

const (
	ResourceIdLen           = 20
	ClusterResourcesTagName = "percona-cluster-stack-id"
)

type Instance struct {
	PublicIpAddress  string
	PrivateIpAddress string
}

type Cloud interface {
	Configure(resourceId string, data *schema.ResourceData) error
	CreateInfrastructure(resourceId string) error
	DeleteInfrastructure(resourceId string) error
	RunCommand(resourceId string, instance Instance, cmd string) (string, error)
	SendFile(resourceId, filePath, remotePath string, instance Instance) error
	CreateInstances(resourceId string, size int64) ([]Instance, error)
}

const (
	KeyPairName          = "key_pair_name"
	PathToKeyPairStorage = "path_to_key_pair_storage"
	ClusterSize          = "cluster_size"
	ConfigFilePath       = "config_file_path"
	InstanceType         = "instance_type"
)

func DefaultSchema() map[string]*schema.Schema {
	return utils.MergeSchemas(map[string]*schema.Schema{
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
		ConfigFilePath: {
			Type:     schema.TypeString,
			Optional: true,
			Default:  "",
		},
		InstanceType: {
			Type:     schema.TypeString,
			Required: true,
		},
	})
}

const (
	LogArgMasterIP  = "percona_master_ip"
	LogArgReplicaIP = "percona_replica_ip"
)
