package aws

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	defaultSubnetCidrBlock = "10.0.1.0/16"

	defaultSecurityGroupName        = "percona-security-group"
	defaultSecurityGroupDescription = "Percona Terraform plugin security group"
)

const (
	volumeThroughput = "volume_throughput"
	vpcID = "vpc_id"
)

func Schema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		volumeThroughput: {
			Type:     schema.TypeInt,
			Optional: true,
		},
		vpcID: {
			Type: schema.TypeString,
			Optional: true,
		},
	}
}
