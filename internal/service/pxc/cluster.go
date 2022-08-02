package pxc

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	awsModel "terraform-percona/internal/models/aws"
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
		Schema: utils.MergeSchemas(awsModel.Schema(), gcp.Schema(), map[string]*schema.Schema{
			pxc.MySQLPassword: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "password",
			},
		}),
	}
}

func resourceInitCluster(_ context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
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

	size, ok := data.Get(awsModel.ClusterSize).(int)
	if !ok {
		return diag.FromErr(errors.New("can't get cluster size"))
	}

	err = pxc.Create(cloud, resourceId, pass, int64(size))
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create pxc cluster"))
	}

	//TODO add creation of terraform resource id
	data.SetId(resourceId)
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
