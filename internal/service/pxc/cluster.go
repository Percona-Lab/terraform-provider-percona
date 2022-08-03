package pxc

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"terraform-percona/internal/models/aws"
	"terraform-percona/internal/models/gcp"
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
	cloud, ok := meta.(service.Cloud)
	if !ok {
		return diag.Errorf("nil aws controller")
	}

	resourceId := utils.GetRandomString(service.ResourceIdLen)

	err := cloud.Configure(resourceId, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	err = cloud.CreateInfrastructure(resourceId)
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

	instances, err := pxc.Create(ctx, cloud, resourceId, pass, int64(size), cfgPath)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create pxc cluster"))
	}

	//TODO add creation of terraform resource id
	data.SetId(resourceId)
	args := make(map[string]interface{})
	if size > 1 {
		for i, instance := range instances {
			if i == 0 {
				args[service.LogArgMasterIP] = instance.PublicIpAddress
				continue
			}
			if args[service.LogArgReplicaIP] == nil {
				args[service.LogArgReplicaIP] = []string{}
			}
			args[service.LogArgReplicaIP] = append(args[service.LogArgReplicaIP].([]string), instance.PublicIpAddress)
		}
	} else if size == 1 {
		args[service.LogArgMasterIP] = instances[0].PublicIpAddress
	}
	tflog.Info(ctx, "Percona XtraDB Cluster resource created", args)
	return nil
}

func resourceInstanceRead(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func resourceInstanceUpdate(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	//TODO
	return nil
}

func resourceInstanceDelete(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	cloud, ok := meta.(service.Cloud)
	if !ok {
		return diag.Errorf("nil aws controller")
	}

	resourceId := data.Id()
	if resourceId == "" {
		return diag.FromErr(fmt.Errorf("empty resource id"))
	}

	err := cloud.Configure(resourceId, data)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	err = cloud.DeleteInfrastructure(resourceId)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't delete cloud infrastructure"))
	}
	return nil
}
