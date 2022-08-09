package service

import (
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/mod/semver"
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
	Version              = "version"
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
		},
		InstanceType: {
			Type:     schema.TypeString,
			Required: true,
		},
		Version: {
			Type:     schema.TypeString,
			Optional: true,
			ValidateDiagFunc: func(v interface{}, path cty.Path) diag.Diagnostics {
				version := v.(string)
				if version == "" {
					return diag.Errorf("empty version provided, use semantic versioning (MAJOR.MINOR.PATCH)")
				}
				if !semver.IsValid("v" + version) {
					return diag.Errorf("invalid version: %s, use semantic versioning (MAJOR.MINOR.PATCH)", version)
				}
				return nil
			},
		},
	})
}

const (
	LogArgMasterIP   = "percona_master_ip"
	LogArgReplicaIP  = "percona_replica_ip"
	LogArgVersion    = "percona_version"
	LogArgInstanceIP = "percona_instance_ip"
)
