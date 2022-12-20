package ps

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-cty/cty"
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
	schemaKeyReplicaPassword      = "replica_password"
	schemaKeyMyRocksInstall       = "myrocks_install"
	schemaKeyOrchestatorSize      = "orchestrator_size"
	schemaKeyOrchestatorPassword  = "orchestrator_password"
	schemaKeyOrchestatorInstances = "orchestrator_instances"
)

type PerconaServer struct {
}

func (r *PerconaServer) Name() string {
	return "ps"
}

func (r *PerconaServer) Schema() map[string]*schema.Schema {
	return utils.MergeSchemas(resource.DefaultMySQLSchema(), aws.Schema(), map[string]*schema.Schema{
		schemaKeyReplicaPassword: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   "replicaPassword",
			Sensitive: true,
		},
		schemaKeyMyRocksInstall: {
			Type:     schema.TypeBool,
			Optional: true,
			Default:  false,
		},
		schemaKeyOrchestatorSize: {
			Type:     schema.TypeInt,
			Optional: true,
			Default:  0,
			ValidateDiagFunc: func(v interface{}, path cty.Path) diag.Diagnostics {
				size := v.(int)
				if size < 3 && size > 0 {
					return diag.Errorf("orchestrator size should be 3 or more")
				}
				return nil
			},
		},
		schemaKeyOrchestatorPassword: {
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
					"is_replica": {
						Type:     schema.TypeBool,
						Computed: true,
					},
				},
			},
		},
		schemaKeyOrchestatorInstances: {
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
					"url": {
						Type:     schema.TypeString,
						Computed: true,
					},
				},
			},
		},
	})
}

func (r *PerconaServer) Create(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
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

	manager := newManager(c, resourceID, data)
	err = manager.createCluster(ctx)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't create ps cluster"))
	}

	if err := setOutputValues(ctx, c, resourceID, data); err != nil {
		return diag.FromErr(errors.Wrap(err, "failed to set output values"))
	}

	tflog.Info(ctx, "Percona Server resource created")
	return nil
}

func setOutputValues(ctx context.Context, c cloud.Cloud, resourceID string, data *schema.ResourceData) error {
	instances, err := c.ListInstances(ctx, resourceID, map[string]string{
		resource.LabelKeyInstanceType: resource.LabelValueInstanceTypeMySQL,
	})
	if err != nil {
		diag.FromErr(errors.Wrap(err, "failed to list instances"))
	}

	set := data.Get(resource.SchemaKeyInstances).(*schema.Set)
	for i, instance := range instances {
		set.Add(map[string]interface{}{
			"is_replica":                         i != 0,
			resource.SchemaKeyInstancesPublicIP:  instance.PublicIpAddress,
			resource.SchemaKeyInstancesPrivateIP: instance.PrivateIpAddress,
		})
	}
	err = data.Set(resource.SchemaKeyInstances, set)
	if err != nil {
		return errors.Wrap(err, "can't set mysql instances")
	}

	instances, err = c.ListInstances(ctx, resourceID, map[string]string{
		resource.LabelKeyInstanceType: resource.LabelValueInstanceTypeOrchestrator,
	})
	if err != nil {
		diag.FromErr(errors.Wrap(err, "failed to list instances"))
	}

	set = data.Get(schemaKeyOrchestatorInstances).(*schema.Set)
	for _, instance := range instances {
		set.Add(map[string]interface{}{
			"url":                                fmt.Sprintf("http://%s:%d/%s", instance.PublicIpAddress, defaultOrchestratorListenPort, defaultOrchestratorURLPrefix),
			resource.SchemaKeyInstancesPublicIP:  instance.PublicIpAddress,
			resource.SchemaKeyInstancesPrivateIP: instance.PrivateIpAddress,
		})
	}
	err = data.Set(schemaKeyOrchestatorInstances, set)
	if err != nil {
		return errors.Wrap(err, "can't set orchestrator instances")
	}
	return nil
}

func (r *PerconaServer) Read(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	// TODO
	return nil
}
func (r *PerconaServer) Update(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	// TODO
	return nil
}
func (r *PerconaServer) Delete(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
	resourceID := data.Id()
	if resourceID == "" {
		return diag.Errorf("empty resource id")
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
