package pmm

import (
	"context"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/cloud/aws"
	"terraform-percona/internal/resource"
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
		resource.Instances: {
			Type:     schema.TypeSet,
			Computed: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					resource.InstancesSchemaKeyPublicIP: {
						Type:     schema.TypeString,
						Computed: true,
					},
					resource.InstancesSchemaKeyPrivateIP: {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
	})
}

func (r *PMM) Create(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
	resourceID := utils.GetRandomString(resource.IDLength)
	err := c.Configure(ctx, resourceID, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}
	data.SetId(resourceID)
	err = c.CreateInfrastructure(ctx, resourceID)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create cloud infrastructure"))
	}
	instances, err := c.CreateInstances(ctx, resourceID, 1)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "create instances"))
	}
	instance := instances[0]
	if _, err := c.RunCommand(ctx, resourceID, instance, cmd.Initial()); err != nil {
		return diag.FromErr(errors.Wrap(err, "failed initial setup"))
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
