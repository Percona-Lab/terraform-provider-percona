package api

import (
	"context"
	"strconv"
	"terraform-percona/internal/cloud"
	"terraform-percona/internal/db"
	"terraform-percona/internal/db/mysql"
	"terraform-percona/internal/db/psql"
	"terraform-percona/internal/resource"

	"github.com/pkg/errors"
)

func (c *Client) DeleteServicesByResourceID(resourceID string) error {
	resp, err := c.ServicesList(&ServicesListRequest{})
	if err != nil {
		return errors.Wrap(err, "services list")
	}
	var serviceIDs []string
	for _, service := range resp.Mysql {
		if v, ok := service.CustomLabels[resource.TagName]; ok && v == resourceID {
			serviceIDs = append(serviceIDs, service.ServiceID)
		}
	}
	for _, service := range resp.Mongodb {
		if v, ok := service.CustomLabels[resource.TagName]; ok && v == resourceID {
			serviceIDs = append(serviceIDs, service.ServiceID)
		}
	}
	for _, service := range resp.Postgresql {
		if v, ok := service.CustomLabels[resource.TagName]; ok && v == resourceID {
			serviceIDs = append(serviceIDs, service.ServiceID)
		}
	}
	for _, service := range resp.Proxysql {
		if v, ok := service.CustomLabels[resource.TagName]; ok && v == resourceID {
			serviceIDs = append(serviceIDs, service.ServiceID)
		}
	}
	for _, service := range resp.Haproxy {
		if v, ok := service.CustomLabels[resource.TagName]; ok && v == resourceID {
			serviceIDs = append(serviceIDs, service.ServiceID)
		}
	}
	for _, service := range resp.External {
		if v, ok := service.CustomLabels[resource.TagName]; ok && v == resourceID {
			serviceIDs = append(serviceIDs, service.ServiceID)
		}
	}

	for _, id := range serviceIDs {
		if err := c.ServicesRemove(&ServicesRemoveRequest{
			ServiceID: id,
			Force:     true,
		}); err != nil {
			return errors.Wrapf(err, "failed to remove service %s", id)
		}
	}
	return nil
}

func (c *Client) AddRDSInstanceToPMM(ctx context.Context, resourceID string, instance *RDSInstance, creds cloud.Credentials, username, password, pmmPassword string) error {
	switch instance.Engine {
	case "DISCOVER_RDS_MYSQL":
		db, err := mysql.NewClient(instance.Address+":"+strconv.FormatInt(instance.Port, 10), username, password)
		if err != nil {
			return errors.Wrap(err, "failed to create new mysql client")
		}
		defer db.Close()
		if err := db.CreatePMMUserForRDS(ctx, pmmPassword); err != nil {
			return errors.Wrap(err, "failed to create pmm user")
		}
	case "DISCOVER_RDS_POSTGRESQL":
		db, err := psql.NewClient(instance.Address+":"+strconv.FormatInt(instance.Port, 10), username, password)
		if err != nil {
			return errors.Wrap(err, "failed to create new mysql client")
		}
		defer db.Close()
		if err := db.CreatePMMUserForRDS(ctx, pmmPassword); err != nil {
			return errors.Wrap(err, "failed to create pmm user")
		}
	default:
		return errors.Errorf("engine %s is not supported", instance.Engine)
	}
	req := &AddRDSRequest{
		Region:       instance.Region,
		Az:           instance.Az,
		InstanceID:   instance.InstanceID,
		NodeModel:    instance.NodeModel,
		Address:      instance.Address,
		Port:         instance.Port,
		Engine:       instance.Engine,
		ServiceName:  instance.InstanceID,
		Username:     db.UserPMM,
		Password:     pmmPassword,
		AwsAccessKey: creds.AccessKey,
		AwsSecretKey: creds.SecretKey,
		CustomLabels: map[string]string{
			resource.TagName: resourceID,
		},
		RdsExporter:               true,
		QanMysqlPerfschema:        true,
		SkipConnectionCheck:       false,
		TLS:                       false,
		TLSSkipVerify:             false,
		TablestatsGroupTableLimit: -1,
		MetricsMode:               1,
		QanPostgresqlPgstatements: true,
	}
	_, err := c.RDSAdd(req)
	if err != nil {
		return errors.Wrap(err, "rds discover")
	}
	return nil
}
