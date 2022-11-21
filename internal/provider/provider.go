package provider

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/resource/pmm"
	"terraform-percona/internal/resource/ps"
	"terraform-percona/internal/resource/pxc"

	awsCloud "terraform-percona/internal/cloud/aws"
	"terraform-percona/internal/cloud/gcp"
)

const (
	schemaKeyCloud = "cloud"

	schemaKeyCloudRegion = "region"

	schemaKeyAWSProfile = "profile"

	schemaKeyGCPProject = "project"
	schemaKeyGCPZone    = "zone"

	schemaKeyIgnoreErrorsOnDestroy = "ignore_errors_on_destroy"
	schemaKeyDisableTelemetry      = "disable_telemetry"
)

func New() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			schemaKeyCloudRegion: {
				Type:     schema.TypeString,
				Required: true,
			},
			schemaKeyGCPProject: {
				Type:     schema.TypeString,
				Optional: true,
			},
			schemaKeyGCPZone: {
				Type:     schema.TypeString,
				Optional: true,
			},
			schemaKeyAWSProfile: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "default",
			},
			schemaKeyCloud: {
				Type:     schema.TypeString,
				Required: true,
			},
			schemaKeyIgnoreErrorsOnDestroy: {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			schemaKeyDisableTelemetry: {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
		ResourcesMap: resource.ResourcesMap(
			new(ps.PerconaServer),
			new(pmm.PMM),
			new(pxc.PerconaXtraDBCluster),
		),
		ConfigureContextFunc: Configure,
	}
}

func Configure(_ context.Context, data *schema.ResourceData) (interface{}, diag.Diagnostics) {
	cloudOpt := data.Get(schemaKeyCloud).(string)
	switch cloudOpt {
	case "aws":
		return &awsCloud.Cloud{
			Region:  aws.String(data.Get(schemaKeyCloudRegion).(string)),
			Profile: aws.String(data.Get(schemaKeyAWSProfile).(string)),
			Meta: cloud.Metadata{
				IgnoreErrorsOnDestroy: data.Get(schemaKeyIgnoreErrorsOnDestroy).(bool),
				DisableTelemetry:      data.Get(schemaKeyDisableTelemetry).(bool),
			},
		}, nil
	case "gcp":
		return &gcp.Cloud{
			Project: data.Get(schemaKeyGCPProject).(string),
			Region:  data.Get(schemaKeyCloudRegion).(string),
			Zone:    data.Get(schemaKeyGCPZone).(string),
			Meta: cloud.Metadata{
				IgnoreErrorsOnDestroy: data.Get(schemaKeyIgnoreErrorsOnDestroy).(bool),
				DisableTelemetry:      data.Get(schemaKeyDisableTelemetry).(bool),
			},
		}, nil
	}
	return nil, diag.FromErr(errors.New("cloud is not supported"))
}
