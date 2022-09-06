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
	"terraform-percona/internal/cloud/gcp"
	"terraform-percona/internal/models/pxc"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
)

func ResourceInstance() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceInitCluster,
		ReadContext:   resourceInstanceRead,
		UpdateContext: resourceInstanceUpdate,
		DeleteContext: resourceInstanceDelete,
		Schema: utils.MergeSchemas(service.DefaultSchema(), aws.Schema(), gcp.Schema(), map[string]*schema.Schema{
			pxc.MySQLPassword: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "password",
			},
		}),
	}
}

func resourceInitCluster(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c, ok := meta.(cloud.Cloud)
	if !ok {
		return diag.Errorf("nil aws controller")
	}

	resourceId := utils.GetRandomString(service.ResourceIdLen)

	err := c.Configure(ctx, resourceId, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	data.SetId(resourceId)
	err = c.CreateInfrastructure(ctx, resourceId)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create cloud infrastructure"))
	}

	pass, ok := data.Get(pxc.MySQLPassword).(string)
	if !ok {
		return diag.FromErr(errors.New("can't get mysql password"))
	}

	size, ok := data.Get(service.ClusterSize).(int)
	if !ok {
		return diag.FromErr(errors.New("can't get cluster size"))
	}

	cfgPath, ok := data.Get(service.ConfigFilePath).(string)
	if !ok {
		return diag.FromErr(errors.New("can't get config file path"))
	}

	version, ok := data.Get(service.Version).(string)
	if !ok {
		return diag.FromErr(errors.New("can't get config file path"))
	}

	instances, err := pxc.Create(ctx, c, resourceId, pass, int64(size), cfgPath, version)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create pxc cluster"))
	}

	args := make(map[string]interface{})
	args[service.LogArgInstanceIP] = []string{}
	for _, instance := range instances {
		args[service.LogArgInstanceIP] = append(args[service.LogArgInstanceIP].([]string), instance.PublicIpAddress)
	}
	tflog.Info(ctx, "Percona XtraDB Cluster resource created", args)
	return nil
}

func resourceInstanceRead(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func resourceInstanceUpdate(_ context.Context, _ *schema.ResourceData, _ interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func resourceInstanceDelete(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c, ok := meta.(cloud.Cloud)
	if !ok {
		return diag.Errorf("nil aws controller")
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
