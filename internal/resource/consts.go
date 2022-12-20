package resource

import (
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/mod/semver"

	"terraform-percona/internal/utils"
)

const (
	LabelKeyInstanceType = "percona_terraform_instance_type"
	LabelKeyResourceID   = "percona_terraform_resource_id"
)

const (
	LabelValueInstanceTypeMySQL        = "mysql"
	LabelValueInstanceTypeOrchestrator = "orchestrator"
)

const (
	SchemaKeyKeyPairName          = "key_pair_name"
	SchemaKeyPathToKeyPairStorage = "path_to_key_pair_storage"
	SchemaKeyClusterSize          = "cluster_size"
	SchemaKeyConfigFilePath       = "config_file_path"
	SchemaKeyInstanceType         = "instance_type"
	SchemaKeyVersion              = "version"
	SchemaKeyVolumeType           = "volume_type"
	SchemaKeyVolumeSize           = "volume_size"
	SchemaKeyVolumeIOPS           = "volume_iops"
	SchemaKeyVPCName              = "vpc_name"
	SchemaKeyInstances            = "instances"
	SchemaKeyPort                 = "port"
	SchemaKeyRootPassword         = "password"
	SchemaKeyPMMAddress           = "pmm_address"
	SchemaKeyPMMPassword          = "pmm_password"
)

func DefaultSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		SchemaKeyKeyPairName: {
			Type:     schema.TypeString,
			Required: true,
		},
		SchemaKeyPathToKeyPairStorage: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   ".",
			Sensitive: true,
		},
		SchemaKeyInstanceType: {
			Type:     schema.TypeString,
			Required: true,
		},
		SchemaKeyVolumeType: {
			Type:     schema.TypeString,
			Optional: true,
		},
		SchemaKeyVolumeSize: {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  20,
		},
		SchemaKeyVolumeIOPS: {
			Type:     schema.TypeInt,
			Optional: true,
		},
		SchemaKeyVPCName: {
			Type:     schema.TypeString,
			Optional: true,
		},
	}
}

func DefaultMySQLSchema() map[string]*schema.Schema {
	return utils.MergeSchemas(DefaultSchema(), map[string]*schema.Schema{
		SchemaKeyClusterSize: {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  3,
		},
		SchemaKeyConfigFilePath: {
			Type:     schema.TypeString,
			Optional: true,
		},
		SchemaKeyVersion: {
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
		SchemaKeyPort: {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  3306,
		},
		SchemaKeyRootPassword: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   "password",
			Sensitive: true,
		},
		SchemaKeyPMMAddress: {
			Type:     schema.TypeString,
			Optional: true,
		},
		SchemaKeyPMMPassword: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   "password",
			Sensitive: true,
		},
	})
}

const (
	LogArgVersion    = "percona_version"
	LogArgInstanceIP = "percona_instance_ip"
)

const (
	SchemaKeyInstancesPublicIP  = "public_ip_address"
	SchemaKeyInstancesPrivateIP = "private_ip_address"
)
