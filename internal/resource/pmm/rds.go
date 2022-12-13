package pmm

import (
	"context"
	"terraform-percona/internal/cloud"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/resource/pmm/api"
	"terraform-percona/internal/utils"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
)

const (
	schemaKeyRDSIdentifier      = "rds_id"
	schemaKeyRDSUsername        = "rds_username"
	schemaKeyRDSPassword        = "rds_password"
	schemaKeyRDSPMMUserPassword = "rds_pmm_user_password"
)

type RDS struct {
}

func (r *RDS) Name() string {
	return "pmm_rds"
}

func (r *RDS) Schema() map[string]*schema.Schema {
	return utils.MergeSchemas(map[string]*schema.Schema{
		resource.PMMAddress: {
			Type:      schema.TypeString,
			Required:  true,
			Sensitive: true,
		},
		schemaKeyRDSIdentifier: {
			Type:      schema.TypeString,
			Required:  true,
			Sensitive: false,
		},
		schemaKeyRDSUsername: {
			Type:      schema.TypeString,
			Required:  true,
			Sensitive: false,
		},
		schemaKeyRDSPassword: {
			Type:      schema.TypeString,
			Required:  true,
			Sensitive: true,
		},
		schemaKeyRDSPMMUserPassword: {
			Type:      schema.TypeString,
			Optional:  true,
			Default:   "password",
			Sensitive: true,
		},
	})
}

func (r *RDS) Create(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
	resourceID := utils.GetRandomString(resource.IDLength)
	err := c.Configure(ctx, resourceID, nil)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "can't configure cloud"))
	}

	pmmAddress := data.Get(resource.PMMAddress).(string)
	pmmAddress, err = utils.ParsePMMAddress(pmmAddress)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "failed to parse pmm address"))
	}

	rdsID := data.Get(schemaKeyRDSIdentifier).(string)
	rdsUsername := data.Get(schemaKeyRDSUsername).(string)
	rdsPassword := data.Get(schemaKeyRDSPassword).(string)
	pmmPassword := data.Get(schemaKeyRDSPMMUserPassword).(string)

	pmmClient, err := api.NewClient(pmmAddress)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "failed to create pmm client"))
	}
	creds, err := c.Credentials()
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "failed to get aws credentials"))
	}
	instances, err := pmmClient.RDSDiscover(creds.AccessKey, creds.SecretKey)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "rds discover"))
	}
	found := false
	for _, instance := range instances {
		if instance.InstanceID == rdsID {
			found = true
			if err := pmmClient.AddRDSInstanceToPMM(ctx, resourceID, &instance, creds, rdsUsername, rdsPassword, pmmPassword); err != nil {
				return diag.FromErr(err)
			}
			break
		}
	}
	if !found {
		return diag.Errorf("RDS instance is not found")
	}
	data.SetId(resourceID)
	tflog.Info(ctx, "PMM resource created")
	return nil
}

func (r *RDS) Read(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	//TODO
	return nil
}

func (r *RDS) Update(_ context.Context, _ *schema.ResourceData, _ cloud.Cloud) diag.Diagnostics {
	//TODO
	return nil
}

func (r *RDS) Delete(ctx context.Context, data *schema.ResourceData, c cloud.Cloud) diag.Diagnostics {
	resourceID := data.Id()
	if resourceID == "" {
		return diag.FromErr(errors.New("empty resource id"))
	}
	pmmAddress := data.Get(resource.PMMAddress).(string)
	pmmAddress, err := utils.ParsePMMAddress(pmmAddress)
	if err != nil {
		return diag.FromErr(errors.Wrap(err, "failed to parse pmm address"))
	}
	pmmClient, err := api.NewClient(pmmAddress)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := pmmClient.DeleteServicesByResourceID(resourceID); err != nil {
		return diag.FromErr(errors.Wrap(err, "failed to delete services"))
	}

	return nil
}
