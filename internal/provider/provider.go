package provider

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	awsModel "terraform-percona/internal/models/aws"
	"terraform-percona/internal/service/pxc"
)

func New() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"region": {
				Type:         schema.TypeString,
				Required:     true,
				InputDefault: "us-east-1",
			},
			"profile": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "default",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"percona_cluster": pxc.ResourceInstance(),
		},
		ConfigureContextFunc: Configure,
	}
}

func Configure(_ context.Context, data *schema.ResourceData) (interface{}, diag.Diagnostics) {
	config := &awsModel.Config{
		Region:           aws.String(data.Get("region").(string)),
		Profile:          aws.String(data.Get("profile").(string)),
		InstanceSettings: &awsModel.InstanceSettings{},
	}

	xtraDBClusterManager, err := awsModel.NewXtraDBClusterManager(config)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return &awsModel.AWSController{
		XtraDBClusterManager: xtraDBClusterManager,
	}, nil
}
