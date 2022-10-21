package provider

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"terraform-percona/internal/resource/pmm"
	"terraform-percona/internal/resource/ps"
	"terraform-percona/internal/resource/pxc"

	awsCloud "terraform-percona/internal/cloud/aws"
	"terraform-percona/internal/cloud/gcp"
)

func New() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"region": {
				Type:     schema.TypeString,
				Required: true,
			},
			"project": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"zone": {
				Type:     schema.TypeString,
				Optional: true,
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
			"ignore_errors_on_destroy": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"percona_pxc": pxc.Resource(),
			"percona_ps":  ps.Resource(),
			"percona_pmm": pmm.Resource(),
		},
		ConfigureContextFunc: Configure,
	}
}

func Configure(_ context.Context, data *schema.ResourceData) (interface{}, diag.Diagnostics) {
	cloudOpt := data.Get("cloud").(string)
	switch cloudOpt {
	case "aws":
		return &awsCloud.Cloud{
			Region:                aws.String(data.Get("region").(string)),
			Profile:               aws.String(data.Get("profile").(string)),
			IgnoreErrorsOnDestroy: data.Get("ignore_errors_on_destroy").(bool),
		}, nil
	case "gcp":
		return &gcp.Cloud{
			Project:               data.Get("project").(string),
			Region:                data.Get("region").(string),
			Zone:                  data.Get("zone").(string),
			IgnoreErrorsOnDestroy: data.Get("ignore_errors_on_destroy").(bool),
		}, nil
	}
	return nil, diag.FromErr(errors.New("cloud is not supported"))
}
