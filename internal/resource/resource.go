package resource

import (
	"context"
	"terraform-percona/internal/cloud"
	"terraform-percona/internal/metrics"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type Resource interface {
	Name() string
	Schema() map[string]*schema.Schema

	Create(ctx context.Context, data *schema.ResourceData, cloud cloud.Cloud) diag.Diagnostics
	Read(ctx context.Context, data *schema.ResourceData, cloud cloud.Cloud) diag.Diagnostics
	Update(ctx context.Context, data *schema.ResourceData, cloud cloud.Cloud) diag.Diagnostics
	Delete(ctx context.Context, data *schema.ResourceData, cloud cloud.Cloud) diag.Diagnostics
}

func toTerraformResource(resource Resource) *schema.Resource {
	return &schema.Resource{
		CreateContext: func(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
			c, ok := meta.(cloud.Cloud)
			if !ok {
				return diag.Errorf("failed to get cloud controller")
			}
			createDiag := resource.Create(ctx, data, c)
			go func() {
				if data.Id() != "" && !c.Metadata().DisableTelemetry {
					if err := metrics.SendTelemetry(resource.Name(), resource.Schema(), data); err != nil {
						tflog.Error(ctx, "failed to send telemetry", map[string]interface{}{"error": err})
					}
				}
			}()
			return createDiag
		},
		ReadContext: func(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
			c, ok := meta.(cloud.Cloud)
			if !ok {
				return diag.Errorf("failed to get cloud controller")
			}
			return resource.Read(ctx, data, c)
		},
		UpdateContext: func(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
			c, ok := meta.(cloud.Cloud)
			if !ok {
				return diag.Errorf("failed to get cloud controller")
			}
			return resource.Update(ctx, data, c)
		},
		DeleteContext: func(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
			c, ok := meta.(cloud.Cloud)
			if !ok {
				return diag.Errorf("failed to get cloud controller")
			}
			return resource.Delete(ctx, data, c)
		},

		Schema: resource.Schema(),
	}
}

func ResourcesMap(resources ...Resource) map[string]*schema.Resource {
	m := make(map[string]*schema.Resource, len(resources))
	for _, r := range resources {
		m["percona_"+r.Name()] = toTerraformResource(r)
	}
	return m
}
