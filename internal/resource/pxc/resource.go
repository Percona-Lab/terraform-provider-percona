package pxc

import (
	"context"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"

	"terraform-percona/internal/cloud"
	"terraform-percona/internal/cloud/aws"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/utils"
)

const (
	galeraPort = "galera_port"
)

type PerconaXtraDBCluster struct {
}

func (r *PerconaXtraDBCluster) Name() string {
	return "pxc"
}

func (r *PerconaXtraDBCluster) Schema() map[string]*schema.Schema {
	return utils.MergeSchemas(resource.DefaultMySQLSchema(), aws.Schema(), map[string]*schema.Schema{
		galeraPort: {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  4567,
		},
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

func (r *PerconaXtraDBCluster) Create(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
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

	manager := newManager(c, resourceID, data)
	instances, err := manager.Create(ctx)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create pxc cluster"))
	}

	set := data.Get(resource.Instances).(*schema.Set)
	for _, instance := range instances {
		set.Add(map[string]interface{}{
			resource.InstancesSchemaKeyPublicIP:  instance.PublicIpAddress,
			resource.InstancesSchemaKeyPrivateIP: instance.PrivateIpAddress,
		})
	}
	err = data.Set(resource.Instances, set)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't set instances"))
	}

	args := make(map[string]interface{})
	args[resource.LogArgInstanceIP] = []string{}
	for _, instance := range instances {
		args[resource.LogArgInstanceIP] = append(args[resource.LogArgInstanceIP].([]string), instance.PublicIpAddress)
	}
	tflog.Info(ctx, "Percona XtraDB Cluster resource created", args)
	return nil
}

func (r *PerconaXtraDBCluster) Read(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	//TODO
	return nil
}

func (r *PerconaXtraDBCluster) Update(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	//TODO
	return nil
}

func (r *PerconaXtraDBCluster) Delete(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
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
