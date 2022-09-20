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

const MySQLPassword = "password"

func Resource() *schema.Resource {
	return &schema.Resource{
		CreateContext: createResource,
		ReadContext:   readResource,
		UpdateContext: updateResource,
		DeleteContext: deleteResource,
		Schema: utils.MergeSchemas(resource.DefaultSchema(), aws.Schema(), map[string]*schema.Schema{
			MySQLPassword: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "password",
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

	resourceId := utils.GetRandomString(resource.IDLength)

	err := c.Configure(ctx, resourceId, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	data.SetId(resourceId)
	err = c.CreateInfrastructure(ctx, resourceId)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create cloud infrastructure"))
	}

	pass := data.Get(MySQLPassword).(string)
	size := data.Get(resource.ClusterSize).(int)
	cfgPath := data.Get(resource.ConfigFilePath).(string)
	version := data.Get(resource.Version).(string)
	instances, err := Create(ctx, c, resourceId, pass, int64(size), cfgPath, version)
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

	resourceId := data.Id()
	if resourceId == "" {
		return diag.FromErr(fmt.Errorf("empty resource id"))
	}

	err := c.Configure(ctx, resourceId, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	err = c.DeleteInfrastructure(ctx, resourceId)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't delete cloud infrastructure"))
	}
	return nil
}
