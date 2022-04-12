package pxc

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	awsModel "terraform-percona/internal/models/aws"
)

func ResourceInstance() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceInitCluster,
		ReadContext:   resourceInstanceRead,
		UpdateContext: resourceInstanceUpdate,
		DeleteContext: resourceInstanceDelete,

		Schema: map[string]*schema.Schema{
			awsModel.Ami: {
				Type:         schema.TypeString,
				Required:     true,
				InputDefault: "ami-0d527b8c289b4af7f",
			},
			awsModel.InstanceType: {
				Type:         schema.TypeString,
				Required:     true,
				InputDefault: "t4g.nano",
			},
			awsModel.KeyPairName: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "pxc",
			},
			awsModel.PathToClusterBootstrapScript: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "./bootstrap.sh",
			},
			awsModel.PathToKeyPairStorage: {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "/tmp/",
			},
			awsModel.ClusterSize: {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  3,
			},
			awsModel.MinCount: {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
			awsModel.MaxCount: {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  1,
			},
		},
	}
}

func resourceInitCluster(_ context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	awsController, ok := meta.(*awsModel.AWSController)
	if !ok {
		return diag.Errorf("nil aws controller")
	}

	xtraDBClusterManager := awsController.XtraDBClusterManager
	if v, ok := data.Get(awsModel.Ami).(string); ok {
		xtraDBClusterManager.Config.Ami = aws.String(v)
	}
	if v, ok := data.Get(awsModel.InstanceType).(string); ok {
		xtraDBClusterManager.Config.InstanceType = aws.String(v)
	}
	if v, ok := data.Get(awsModel.MinCount).(int); ok {
		xtraDBClusterManager.Config.MinCount = aws.Int64(int64(v))
	}
	if v, ok := data.Get(awsModel.MaxCount).(int); ok {
		xtraDBClusterManager.Config.MaxCount = aws.Int64(int64(v))
	}
	if v, ok := data.Get(awsModel.KeyPairName).(string); ok {
		xtraDBClusterManager.Config.KeyPairName = aws.String(v)
	}
	if v, ok := data.Get(awsModel.PathToClusterBootstrapScript).(string); ok {
		xtraDBClusterManager.Config.PathToClusterBoostrapScript = aws.String(v)
	}
	if v, ok := data.Get(awsModel.PathToKeyPairStorage).(string); ok {
		xtraDBClusterManager.Config.PathToKeyPairStorage = aws.String(v)
	}
	if v, ok := data.Get(awsModel.ClusterSize).(int); ok {
		xtraDBClusterManager.Config.ClusterSize = aws.Int64(int64(v))
	}

	if _, err := xtraDBClusterManager.CreateCluster(); err != nil {
		return diag.Errorf("Error occurred during cluster creating: %w", err)
	}

	//TODO add creation of terraform resource id
	data.SetId("resource-id")
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
	//TODO
	return nil
}
