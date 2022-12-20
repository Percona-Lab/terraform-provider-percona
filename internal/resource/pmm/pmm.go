package pmm

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/cloud/aws"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/resource/pmm/api"
	"terraform-percona/internal/resource/pmm/cmd"
	"terraform-percona/internal/utils"
)

type PMM struct {
}

func (r *PMM) Name() string {
	return "pmm"
}

func (r *PMM) Schema() map[string]*schema.Schema {
	return utils.MergeSchemas(resource.DefaultSchema(), aws.Schema(), map[string]*schema.Schema{
		schemaKeyRDSUsername: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   "",
			Sensitive: true,
		},
		schemaKeyRDSPassword: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   "",
			Sensitive: true,
		},
		schemaKeyRDSPMMUserPassword: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   "password",
			Sensitive: true,
		},
		resource.SchemaKeyInstances: {
			Type:     schema.TypeSet,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					resource.SchemaKeyInstancesPublicIP: {
						Type:     schema.TypeString,
						Computed: true,
					},
					resource.SchemaKeyInstancesPrivateIP: {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
	})
}

func (r *PMM) Create(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
	resourceID := utils.GenerateResourceID()
	err := c.Configure(ctx, resourceID, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}
	data.SetId(resourceID)
	err = c.CreateInfrastructure(ctx, resourceID)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create cloud infrastructure"))
	}
	instances, err := c.CreateInstances(ctx, resourceID, 1, nil)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "create instances"))
	}
	instance := instances[0]
	if _, err := c.RunCommand(ctx, resourceID, instance, cmd.Initial()); err != nil {
		return diag.FromErr(errors.Wrap(err, "failed initial setup"))
	}

	set := data.Get(resource.SchemaKeyInstances).(*schema.Set)
	for _, instance := range instances {
		set.Add(map[string]interface{}{
			resource.SchemaKeyInstancesPublicIP:  instance.PublicIpAddress,
			resource.SchemaKeyInstancesPrivateIP: instance.PrivateIpAddress,
		})
	}
	err = data.Set(resource.SchemaKeyInstances, set)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't set instances"))
	}

	rdsUsername := data.Get(schemaKeyRDSUsername).(string)
	rdsPassword := data.Get(schemaKeyRDSPassword).(string)
	rdsPMMUserPassword := data.Get(schemaKeyRDSPMMUserPassword).(string)

	if rdsUsername != "" && rdsPassword != "" {
		pmmAddress, err := utils.ParsePMMAddress("http://" + instance.PublicIpAddress)
		if err != nil {
			return diag.FromErr(errors.Wrap(err, "failed to parse pmm address"))
		}
		pmmClient, err := api.NewClient(pmmAddress)
		if err != nil {
			return diag.FromErr(err)
		}
		creds, err := c.Credentials()
		if err != nil {
			return diag.FromErr(err)
		}
		time.Sleep(time.Second * 30)
		instances, err := pmmClient.RDSDiscover(creds.AccessKey, creds.SecretKey)
		if err != nil {
			return diag.FromErr(err)
		}
		for _, instance := range instances {
			if err := pmmClient.AddRDSInstanceToPMM(ctx, resourceID, &instance, creds, rdsUsername, rdsPassword, rdsPMMUserPassword); err != nil {
				tflog.Error(ctx, "failed to add RDS instance to PMM", map[string]interface{}{
					"percona_rds_id": instance.InstanceID,
					"error":          err,
				})
				continue
			}
			tflog.Info(ctx, "RDS instance added to PMM", map[string]interface{}{
				"percona_rds_id": instance.InstanceID,
			})
		}
	}

	tflog.Info(ctx, "PMM resource created")
	return nil
}

func (r *PMM) Read(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	//TODO
	return nil
}

func (r *PMM) Update(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	//TODO
	return nil
}

func (r *PMM) Delete(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
	resourceID := data.Id()
	if resourceID == "" {
		return diag.FromErr(errors.New("empty resource id"))
	}

	err := c.Configure(ctx, resourceID, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	err = c.DeleteInfrastructure(ctx, resourceID)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't delete cloud infrastructure"))
	}
	return nil
}
