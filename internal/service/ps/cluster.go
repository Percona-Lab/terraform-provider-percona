package ps

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	awsModel "terraform-percona/internal/models/aws"
	"terraform-percona/internal/models/ps"
	"terraform-percona/internal/service"
	"terraform-percona/internal/utils"
)

func ResourceInstance() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceInitCluster,
		ReadContext:   resourceInstanceRead,
		UpdateContext: resourceInstanceUpdate,
		DeleteContext: resourceInstanceDelete,

		Schema: utils.MergeSchemas(awsModel.Schema(), map[string]*schema.Schema{
			ps.RootPassword: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "password",
			},
			ps.ReplicaPassword: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "replicaPassword",
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

	size, ok := data.Get(awsModel.ClusterSize).(int)
	if !ok {
		return diag.FromErr(errors.New("can't get cluster size"))
	}

	pass, ok := data.Get(ps.RootPassword).(string)
	if !ok {
		return diag.FromErr(errors.New("can't get mysql password"))
	}

	replicaPass, ok := data.Get(ps.ReplicaPassword).(string)
	if !ok {
		return diag.FromErr(errors.New("can't get mysql password"))
	}
	err = ps.Create(cloud, resourceId, int64(size), pass, replicaPass)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create ps cluster"))
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
