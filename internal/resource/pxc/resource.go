package pxc

import (
	"context"
	"fmt"

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

func Resource() *schema.Resource {
	return &schema.Resource{
		CreateContext: createResource,
		ReadContext:   readResource,
		UpdateContext: updateResource,
		DeleteContext: deleteResource,
		Schema: utils.MergeSchemas(resource.DefaultSchema(), aws.Schema(), map[string]*schema.Schema{
			galeraPort: {
				Type:      schema.TypeInt,
				Optional:  true,
				Default:   4567,
				Sensitive: true,
			},
			resource.Instances: {
				Type:     schema.TypeSet,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"public_ip_address": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"private_ip_address": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
		}),
	}
}

func createResource(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c, ok := meta.(cloud.Cloud)
	if !ok {
		return diag.Errorf("failed to get cloud controller")
	}

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
			"public_ip_address":  instance.PublicIpAddress,
			"private_ip_address": instance.PrivateIpAddress,
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

func readResource(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func updateResource(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func deleteResource(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c, ok := meta.(cloud.Cloud)
	if !ok {
		return diag.Errorf("failed to get cloud controller")
	}

	resourceID := data.Id()
	if resourceID == "" {
		return diag.FromErr(fmt.Errorf("empty resource id"))
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
