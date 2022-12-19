package ps

import (
	"context"
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
	"terraform-percona/internal/resource/ps/cmd"
	"terraform-percona/internal/utils"
)

type manager struct {
	size           int
	pass           string
	replicaPass    string
	installMyRocks bool
	cfgPath        string
	version        string
	port           int
	pmmAddress     string
	pmmPassword    string

	resourceID string

	cloud cloud.Cloud
}

func newManager(cloud cloud.Cloud, resourceID string, data *schema.ResourceData) *manager {
	return &manager{
		size:           data.Get(resource.ClusterSize).(int),
		pass:           data.Get(resource.RootPassword).(string),
		replicaPass:    data.Get(ReplicaPassword).(string),
		installMyRocks: data.Get(MyRocksInstall).(bool),
		cfgPath:        data.Get(resource.ConfigFilePath).(string),
		version:        data.Get(resource.Version).(string),
		port:           data.Get(resource.Port).(int),
		pmmAddress:     data.Get(resource.PMMAddress).(string),
		pmmPassword:    data.Get(resource.PMMPassword).(string),
		resourceID:     resourceID,
		cloud:          cloud,
	}
}

const (
	defaultMysqlConfigPath = "/etc/mysql/mysql.conf.d/mysqld.cnf"
	customMysqlConfigPath  = "/etc/mysql/mysql.conf.d/custom.cnf"
)

func (m *manager) createCluster(ctx context.Context) ([]cloud.Instance, error) {
	tflog.Info(ctx, "Creating instances")
	instances, err := m.cloud.CreateInstances(ctx, m.resourceID, int64(m.size))
	if err != nil {
		return nil, errors.Wrap(err, "create instances")
	}
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(len(instances))
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err := m.runCommand(gCtx, instance, cmd.Configure(m.pass))
			if err != nil {
				return errors.Wrap(err, "run command")
			}
			if err := m.installPerconaServer(gCtx, instance); err != nil {
				return errors.Wrap(err, "install percona server")
			}
			_, err = m.runCommand(gCtx, instance, cmd.Restart())
			if err != nil {
				return errors.Wrap(err, "restart mysql")
			}
			db, err := m.newClient(instance, internaldb.UserRoot, m.pass)
			if err != nil {
				return errors.Wrap(err, "failed to establish sql connection")
			}
			defer db.Close()
			if err := db.InstallPerconaServerUDF(gCtx); err != nil {
				return errors.Wrap(err, "failed to install percona server UDF")
			}
			if m.cfgPath != "" {
				if err = m.cloud.SendFile(gCtx, m.resourceID, instance, m.cfgPath, customMysqlConfigPath); err != nil {
					return errors.Wrap(err, "failed to send config file")
				}
			}
			if m.installMyRocks {
				_, err = m.runCommand(gCtx, instance, cmd.InstallMyRocks(m.pass, m.version))
				if err != nil {
					return errors.Wrap(err, "install myrocks")
				}
				if err := m.editDefaultCfg(gCtx, instance, "mysqld", map[string]string{"default-storage-engine": "rocksdb"}); err != nil {
					return errors.Wrap(err, "set default-storage-engine")
				}
			}
			if m.pmmAddress != "" {
				addr, err := utils.ParsePMMAddress(m.pmmAddress)
				if err != nil {
					return errors.Wrap(err, "failed to parse pmm address")
				}
				_, err = m.runCommand(gCtx, instance, cmd.InstallPMMClient(addr))
				if err != nil {
					return errors.Wrap(err, "install pmm client")
				}

				err = db.CreatePMMUser(gCtx, m.pmmPassword)
				if err != nil {
					return errors.Wrap(err, "create pmm user")
				}
				err = m.editDefaultCfg(gCtx, instance, "mysqld", map[string]string{
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
					return errors.Wrap(err, "failed to edit default cfg for pmm")
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
	if err := m.setupInstances(ctx, instances); err != nil {
		return nil, errors.Wrap(err, "setup instances")
	}
	return instances, nil
}

func (m *manager) editDefaultCfg(ctx context.Context, instance cloud.Instance, section string, keysAndValues map[string]string) error {
	return m.cloud.EditFile(ctx, m.resourceID, instance, defaultMysqlConfigPath, utils.SetIniFields(section, keysAndValues))
}

func (m *manager) installPerconaServer(ctx context.Context, instance cloud.Instance) error {
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
	tflog.Info(ctx, "Installing Percona Server", map[string]interface{}{
		resource.LogArgVersion:    m.version,
		resource.LogArgInstanceIP: instance.PublicIpAddress,
	})
	_, err = m.runCommand(ctx, instance, cmd.InstallPerconaServer(m.pass, m.version, m.port))
	if err != nil {
		return errors.Wrap(err, "install percona server")
	}
	if err := m.editDefaultCfg(ctx, instance, "mysqld", map[string]string{"port": strconv.Itoa(m.port)}); err != nil {
		return errors.Wrap(err, "set port")
	}
	return nil
}

func (m *manager) setupInstances(ctx context.Context, instances []cloud.Instance) error {
	masterInstance := instances[0]
	db, err := m.newClient(masterInstance, internaldb.UserRoot, m.pass)
	if err != nil {
		return errors.Wrap(err, "new client")
	}
	defer db.Close()
	if err := db.CreateReplicaUser(ctx, m.replicaPass); err != nil {
		return errors.Wrap(err, "create replica user")
	}
	for i, instance := range instances {
		serverID := i + 1
		if len(instances) > 1 {
			cfgValues := map[string]string{
				"log_bin":   "/var/log/mysql/mysql-bin.log",
				"server_id": strconv.Itoa(serverID),
				"relay-log": "/var/log/mysql/mysql-relay-bin.log",
			}
			if serverID == 1 {
				cfgValues["bind-address"] = instance.PrivateIpAddress
			}

			if err := m.editDefaultCfg(ctx, instance, "mysqld", cfgValues); err != nil {
				return errors.Wrap(err, "edit default cfg for replication")
			}
		}
		if serverID == 1 {
			db.Close()
		}
		_, err := m.runCommand(ctx, instance, cmd.Restart())
		if err != nil {
			return errors.Wrap(err, "restart mysql")
		}
		switch serverID {
		case 1:
			if err := db.Open(); err != nil {
				return errors.Wrap(err, "failed to reopen connection")
			}
		default:
			binlogName, binlogPos, err := db.BinlogFileAndPosition(ctx)
			if err != nil {
				return errors.Wrap(err, "get binlog name and position")
			}
			if err := m.startReplica(ctx, instance, masterInstance.PrivateIpAddress, binlogName, binlogPos); err != nil {
				return errors.Wrap(err, "start replica")
			}
		}
		if m.pmmAddress != "" {
			_, err = m.runCommand(ctx, instance, cmd.AddServiceToPMM("pmm", m.pmmPassword, m.port))
			if err != nil {
				return errors.Wrap(err, "add service to pmm")
			}
		}
	}
	return nil
}

func (m *manager) startReplica(ctx context.Context, instance cloud.Instance, masterIP, binlogName string, binlogPos int64) error {
	db, err := m.newClient(instance, internaldb.UserRoot, m.pass)
	if err != nil {
		return errors.Wrap(err, "failed to establish sql connection")
	}
	defer db.Close()
	if err := db.ChangeReplicationSource(ctx, masterIP, m.port, internaldb.UserReplica, m.replicaPass, binlogName, binlogPos); err != nil {
		return errors.Wrap(err, "change replication source")
	}
	if err := db.StartReplica(ctx); err != nil {
		return errors.Wrap(err, "start replica")
	}
	return nil
}

func (m *manager) versionList(ctx context.Context, instance cloud.Instance) ([]string, error) {
	out, err := m.runCommand(ctx, instance, cmd.RetrieveVersions())
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

func (m *manager) newClient(instance cloud.Instance, user, pass string) (*mysql.DB, error) {
	return mysql.NewClient(instance.PublicIpAddress+":"+strconv.Itoa(m.port), user, pass)
}
