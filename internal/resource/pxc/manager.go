package pxc

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"terraform-percona/internal/cloud"
	internaldb "terraform-percona/internal/db"
	"terraform-percona/internal/db/mysql"
	"terraform-percona/internal/resource"
	"terraform-percona/internal/resource/pxc/cmd"
	"terraform-percona/internal/utils"
)

type manager struct {
	size       int
	password   string
	cfgPath    string
	version    string
	mysqlPort  int
	galeraPort int

	pmmAddress  string
	pmmPassword string

	resourceID string

	cloud cloud.Cloud
}

func newManager(cloud cloud.Cloud, resourceID string, data *schema.ResourceData) *manager {
	return &manager{
		size:        data.Get(resource.ClusterSize).(int),
		password:    data.Get(resource.RootPassword).(string),
		cfgPath:     data.Get(resource.ConfigFilePath).(string),
		version:     data.Get(resource.Version).(string),
		mysqlPort:   data.Get(resource.Port).(int),
		galeraPort:  data.Get(galeraPort).(int),
		resourceID:  resourceID,
		cloud:       cloud,
		pmmAddress:  data.Get(resource.PMMAddress).(string),
		pmmPassword: data.Get(resource.PMMPassword).(string),
	}
}

func (m *manager) Create(ctx context.Context) ([]cloud.Instance, error) {
	tflog.Info(ctx, "Creating instances")
	instances, err := m.cloud.CreateInstances(ctx, m.resourceID, int64(m.size))
	if err != nil {
		return nil, errors.Wrap(err, "create instances")
	}
	clusterHosts := make([]string, 0, len(instances))
	for _, instance := range instances {
		clusterHosts = append(clusterHosts, instance.PrivateIpAddress+":"+strconv.Itoa(m.galeraPort))
	}
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(len(instances))
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err = m.cloud.RunCommand(gCtx, m.resourceID, instance, cmd.Configure(m.password))
			if err != nil {
				return errors.Wrap(err, "run command pxc configure")
			}
			if err := m.installPXC(gCtx, instance, clusterHosts); err != nil {
				return errors.Wrap(err, "install pxc")
			}
			if m.cfgPath != "" {
				if err = m.cloud.SendFile(gCtx, m.resourceID, instance, m.cfgPath, customMysqlConfigPath); err != nil {
					return errors.Wrap(err, "failed to send config file")
				}
			}
			return nil
		})
	}
	tflog.Info(ctx, "Configuring instances")
	if err = g.Wait(); err != nil {
		return nil, errors.Wrap(err, "configure instances")
	}
	tflog.Info(ctx, "Starting instances")
	for i, instance := range instances {
		_, err = m.cloud.RunCommand(ctx, m.resourceID, instance, cmd.Start(i == 0))
		if err != nil {
			return nil, errors.Wrap(err, "run command pxc start")
		}
		if m.pmmAddress != "" {
			db, err := m.newClient(instance, internaldb.UserRoot, m.password)
			if err != nil {
				return nil, errors.Wrap(err, "failed to create new mysql client")
			}
			defer db.Close()
			if err := db.CreatePMMUser(ctx, m.pmmPassword); err != nil {
				return nil, errors.Wrap(err, "failed to create pmm user")
			}
			addr, err := utils.ParsePMMAddress(m.pmmAddress)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse pmm address")
			}
			_, err = m.runCommand(ctx, instance, cmd.InstallPMMClient(addr))
			if err != nil {
				return nil, errors.Wrap(err, "install pmm client")
			}
			err = m.editDefaultCfg(ctx, instance, "mysqld", map[string]string{
				// Slow query log
				"slow_query_log":                    "ON",
				"log_output":                        "FILE",
				"long_query_time":                   "0",
				"log_slow_admin_statements":         "ON",
				"log_slow_slave_statements":         "ON",
				"log_slow_rate_limit":               "100",
				"log_slow_rate_type":                "query",
				"slow_query_log_always_write_time":  "1",
				"log_slow_verbosity":                "full",
				"slow_query_log_use_global_control": "all",
				// While you can use both slow query log and performance schema at the same time it's recommended to use only one
				// There is some overlap in the data reported, and each incurs a small performance penalty
				// https://docs.percona.com/percona-monitoring-and-management/setting-up/client/mysql.html#choose-and-configure-a-source
				// We should disable performance schema
				"performance_schema": "OFF",
				"userstat":           "ON", // User statistics
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to edit default cfg for pmm")
			}
			_, err = m.runCommand(ctx, instance, cmd.AddServiceToPMM("pmm", m.pmmPassword, m.mysqlPort))
			if err != nil {
				return nil, errors.Wrap(err, "add service to pmm")
			}
		}
	}

	if len(instances) > 1 {
		if _, err = m.cloud.RunCommand(ctx, m.resourceID, instances[0], cmd.Stop(true)); err != nil {
			return nil, errors.Wrap(err, "run command bootstrap stop")
		}
		if _, err = m.cloud.RunCommand(ctx, m.resourceID, instances[0], cmd.Start(false)); err != nil {
			return nil, errors.Wrap(err, "run command first node pxc start")
		}
	}
	return instances, nil
}

func (m *manager) installPXC(ctx context.Context, instance cloud.Instance, clusterHosts []string) error {
	availableVersions, err := m.versionList(ctx, instance)
	if err != nil {
		return errors.Wrap(err, "retrieve versions")
	}
	if m.version != "" {
		fullVersion := utils.SelectVersion(availableVersions, m.version)
		if fullVersion == "" {
			return errors.Errorf("version not found, available versions: %v", availableVersions)
		}
		m.version = fullVersion
	} else {
		m.version = availableVersions[0]
	}
	_, err = m.runCommand(ctx, instance, cmd.InstallPerconaXtraDBCluster(m.version))
	if err != nil {
		return errors.Wrap(err, "failed to run pxc install cmd")
	}
	err = m.editDefaultCfg(ctx, instance, "mysqld", map[string]string{
		"port":                        strconv.Itoa(m.mysqlPort),
		"wsrep_cluster_address":       "gcomm://" + strings.Join(clusterHosts, ","),
		"wsrep_node_name":             instance.PrivateIpAddress,
		"wsrep_node_address":          instance.PrivateIpAddress + ":" + strconv.Itoa(m.galeraPort),
		"wsrep_provider_options":      fmt.Sprintf("base_port=%d", m.galeraPort),
		"pxc-encrypt-cluster-traffic": "OFF",
	})
	if err != nil {
		return errors.Wrap(err, "failed to edit default config")
	}
	return nil
}

func (m *manager) versionList(ctx context.Context, instance cloud.Instance) ([]string, error) {
	out, err := m.cloud.RunCommand(ctx, m.resourceID, instance, cmd.RetrieveVersions())
	if err != nil {
		return nil, errors.Wrap(err, "retrieve versions")
	}
	versions := strings.Split(out, "\n")
	if len(versions) == 0 {
		return nil, errors.New("no available versions")
	}
	return versions, nil
}

func (m *manager) runCommand(ctx context.Context, instance cloud.Instance, cmd string) (string, error) {
	return m.cloud.RunCommand(ctx, m.resourceID, instance, cmd)
}

const (
	defaultMysqlConfigPath = "/etc/mysql/mysql.conf.d/mysqld.cnf"
	customMysqlConfigPath  = "/etc/mysql/mysql.conf.d/custom.cnf"
)

func (m *manager) editDefaultCfg(ctx context.Context, instance cloud.Instance, section string, keysAndValues map[string]string) error {
	return m.cloud.EditFile(ctx, m.resourceID, instance, defaultMysqlConfigPath, utils.SetIniFields(section, keysAndValues))
}

func (m *manager) newClient(instance cloud.Instance, user, pass string) (*mysql.DB, error) {
	return mysql.NewClient(instance.PublicIpAddress+":"+strconv.Itoa(m.mysqlPort), user, pass)
}
