package provider

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	awsModel "terraform-percona/internal/models/aws"
	"terraform-percona/internal/service/ps"
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
			"cloud": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"percona_pxc": pxc.ResourceInstance(),
			"percona_ps":  ps.ResourceInstance(),
		},
		ConfigureContextFunc: Configure,
	}
}

func Configure(_ context.Context, data *schema.ResourceData) (interface{}, diag.Diagnostics) {
	cloudOpt := data.Get("cloud").(string)
	switch cloudOpt {
	case "aws":
		return &awsModel.Cloud{
			Region:  aws.String(data.Get("region").(string)),
			Profile: aws.String(data.Get("profile").(string)),
		}, nil
	}
	return nil, diag.FromErr(errors.New("cloud is not supported"))
}
