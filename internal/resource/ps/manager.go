package ps

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-uuid"
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
	size                 int
	pass                 string
	replicaPass          string
	installMyRocks       bool
	cfgPath              string
	version              string
	port                 int
	pmmAddress           string
	pmmPassword          string
	orchestratorSize     int
	orchestratorPassword string
	replicationType      string

	resourceID string

	cloud cloud.Cloud
}

func newManager(cloud cloud.Cloud, resourceID string, data *schema.ResourceData) *manager {
	return &manager{
		size:                 data.Get(resource.SchemaKeyClusterSize).(int),
		pass:                 data.Get(resource.SchemaKeyRootPassword).(string),
		replicationType:      data.Get(schemaKeyReplicationType).(string),
		replicaPass:          data.Get(schemaKeyReplicaPassword).(string),
		installMyRocks:       data.Get(schemaKeyMyRocksInstall).(bool),
		orchestratorSize:     data.Get(schemaKeyOrchestatorSize).(int),
		orchestratorPassword: data.Get(schemaKeyOrchestatorPassword).(string),
		cfgPath:              data.Get(resource.SchemaKeyConfigFilePath).(string),
		version:              data.Get(resource.SchemaKeyVersion).(string),
		port:                 data.Get(resource.SchemaKeyPort).(int),
		pmmAddress:           data.Get(resource.SchemaKeyPMMAddress).(string),
		pmmPassword:          data.Get(resource.SchemaKeyPMMPassword).(string),
		resourceID:           resourceID,
		cloud:                cloud,
	}
}

const (
	defaultMysqlConfigPath = "/etc/mysql/mysql.conf.d/mysqld.cnf"
	customMysqlConfigPath  = "/etc/mysql/mysql.conf.d/custom.cnf"
)

func (m *manager) createCluster(ctx context.Context) error {
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return m.setupPerconaServer(gCtx)
	})
	g.Go(func() error {
		return m.setupOrchestrator(gCtx)
	})
	if err := g.Wait(); err != nil {
		return err
	}
	if err := m.addPSInstancesToOrchestrator(ctx); err != nil {
		return err
	}
	return nil
}

func (m *manager) addPSInstancesToOrchestrator(ctx context.Context) error {
	if m.orchestratorSize <= 0 {
		return nil
	}

	instances, err := m.cloud.ListInstances(ctx, m.resourceID, map[string]string{
		resource.LabelKeyInstanceType: resource.LabelValueInstanceTypeMySQL,
	})
	if err != nil {
		return errors.Wrap(err, "failed to list percona server instances")
	}
	orcInstances, err := m.cloud.ListInstances(ctx, m.resourceID, map[string]string{
		resource.LabelKeyInstanceType: resource.LabelValueInstanceTypeOrchestrator,
	})
	if err != nil {
		return errors.Wrap(err, "failed to list orchestrator instances")
	}

	orchestratorAPIHosts := make([]string, 0, len(orcInstances))
	for _, i := range orcInstances {
		if _, err = m.runCommand(ctx, i, "sudo systemctl restart orchestrator"); err != nil {
			return errors.Wrap(err, "failed to restart orchestrator")
		}
		orchestratorAPIHosts = append(orchestratorAPIHosts, fmt.Sprintf("http://%s:%d/%s", i.PrivateIpAddress, defaultOrchestratorListenPort, defaultOrchestratorURLPrefix))
	}

	time.Sleep(30 * time.Second)
	for _, i := range instances {
		tflog.Info(ctx, fmt.Sprintf("Adding instance %s to orhestrator", i.PublicIpAddress), map[string]any{})
		_, err := m.runCommand(ctx, i, fmt.Sprintf(`ORCHESTRATOR_API="%s" orchestrator-client -c discover -i %s:%d`, strings.Join(orchestratorAPIHosts, " "), i.PrivateIpAddress, m.port))
		if err != nil {
			tflog.Info(ctx, fmt.Sprintf("failed to add instance %s to orchestrator", i.PublicIpAddress), map[string]any{
				"error": err,
			})
		}
	}
	return nil
}

func (m *manager) setupOrchestrator(ctx context.Context) error {
	if m.orchestratorSize <= 0 {
		return nil
	}
	time.Sleep(time.Second * 5)

	tflog.Info(ctx, "Creating orchestrator instances")
	instances, err := m.cloud.CreateInstances(ctx, m.resourceID, int64(m.orchestratorSize), map[string]string{
		resource.LabelKeyInstanceType: resource.LabelValueInstanceTypeOrchestrator,
	})
	if err != nil {
		return errors.Wrap(err, "create orchestrator instances")
	}
	tflog.Info(ctx, "Orchestrator instances created")
	g, gCtx := errgroup.WithContext(ctx)
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err := m.runCommand(gCtx, instance, cmd.Init())
			if err != nil {
				return errors.Wrap(err, "run command")
			}
			_, err = m.runCommand(gCtx, instance, cmd.InstallOrchestrator())
			if err != nil {
				return errors.Wrap(err, "run command")
			}
			tflog.Info(ctx, "Orchestrator installed")
			cfg, err := orchestratorConfig(instance, instances)
			if err != nil {
				return errors.Wrap(err, "failed to create orchestrator config")
			}
			tflog.Info(ctx, "Orchestrator config created")
			err = m.sendFile(gCtx, instance, bytes.NewReader(cfg), defaultOrchestratorConfigPath)
			if err != nil {
				return errors.Wrap(err, "failed to send orchestrator config file")
			}
			creds, err := orchestratorTopologyCredentials(m.orchestratorPassword)
			if err != nil {
				return errors.Wrap(err, "failed to create orchestrator credentials file")
			}
			err = m.sendFile(gCtx, instance, creds, defaultOrchestratorCredentialsPath)
			if err != nil {
				return errors.Wrap(err, "failed to send orchestrator credentials file")
			}
			tflog.Info(ctx, "Orchestrator started")
			_, err = m.runCommand(gCtx, instance, "sudo systemctl start orchestrator")
			if err != nil {
				return errors.Wrap(err, "start orchestrator")
			}
			return nil
		})
	}
	tflog.Info(ctx, "Waiting orchestrator to be configured")
	if err := g.Wait(); err != nil {
		return errors.Wrap(err, "failed to setup orchestrator")
	}
	return nil
}

func (m *manager) setupPerconaServer(ctx context.Context) error {
	tflog.Info(ctx, "Creating instances")
	instances, err := m.cloud.CreateInstances(ctx, m.resourceID, int64(m.size), map[string]string{
		resource.LabelKeyInstanceType: resource.LabelValueInstanceTypeMySQL,
	})
	if err != nil {
		return errors.Wrap(err, "create instances")
	}
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(len(instances))
	for _, instance := range instances {
		instance := instance
		g.Go(func() error {
			_, err := m.runCommand(gCtx, instance, cmd.Init())
			if err != nil {
				return errors.Wrap(err, "init")
			}
			_, err = m.runCommand(gCtx, instance, cmd.Configure(m.pass))
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
				cfgFile, err := os.Open(m.cfgPath)
				if err != nil {
					return errors.Wrap(err, "failed to open config file")
				}
				defer cfgFile.Close()
				if err = m.sendFile(gCtx, instance, cfgFile, customMysqlConfigPath); err != nil {
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
			if m.orchestratorSize > 0 {
				err = db.CreateOrchestratorUser(gCtx, m.orchestratorPassword, false)
				if err != nil {
					return errors.Wrap(err, "failed to create orchestrator user")
				}
				_, err = m.runCommand(gCtx, instance, cmd.InstallOrchestratorClient())
				if err != nil {
					return errors.Wrap(err, "failed to install orchestrator-client")
				}
			}
			return nil
		})
	}
	tflog.Info(ctx, "Configuring instances")
	if err = g.Wait(); err != nil {
		return errors.Wrap(err, "configure instances")
	}
	tflog.Info(ctx, "Starting instances")
	if err := m.setupInstances(ctx, instances); err != nil {
		return errors.Wrap(err, "setup instances")
	}
	return nil
}

func (m *manager) editDefaultCfg(ctx context.Context, instance cloud.Instance, section string, keysAndValues map[string]string) error {
	return m.editFile(ctx, instance, defaultMysqlConfigPath, utils.SetIniFields(section, keysAndValues))
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

const defaultMySQLGroupReplicationPort = 33061

func (m *manager) instanceConfig(ctx context.Context, instance cloud.Instance, instances []cloud.Instance, serverID int, uuid string) map[string]string {
	switch m.replicationType {
	case replicationTypeAsync:
		cfg := map[string]string{
			"log_bin":                  "/var/log/mysql/mysql-bin.log",
			"server_id":                strconv.Itoa(serverID),
			"relay-log":                "/var/log/mysql/mysql-relay-bin.log",
			"gtid-mode":                "ON",
			"enforce-gtid-consistency": "ON",
		}
		if serverID == 1 {
			cfg["bind-address"] = strings.Join([]string{instance.PrivateIpAddress, "localhost"}, ",")
		}
		return cfg
	case replicationTypeGR:
		cfg := map[string]string{
			"disabled_storage_engines": "MyISAM,BLACKHOLE,FEDERATED,ARCHIVE,MEMORY",
			"server_id":                strconv.Itoa(serverID),
			"log_bin":                  "/var/log/mysql/mysql-bin.log",
			"relay-log":                "/var/log/mysql/mysql-relay-bin.log",
			"gtid-mode":                "ON",
			"enforce-gtid-consistency": "ON",

			"plugin_load_add":                   "group_replication.so",
			"group_replication_group_name":      uuid,
			"group_replication_start_on_boot":   "off",
			"group_replication_local_address":   fmt.Sprintf("%s:%d", instance.PrivateIpAddress, defaultMySQLGroupReplicationPort),
			"group_replication_bootstrap_group": "off",
		}
		seeds := make([]string, 0, len(instances))
		allowList := make([]string, 0, len(instances))
		for _, i := range instances {
			seeds = append(seeds, fmt.Sprintf("%s:%d", i.PrivateIpAddress, defaultMySQLGroupReplicationPort))
			allowList = append(allowList, i.PrivateIpAddress)
		}
		cfg["group_replication_group_seeds"] = strings.Join(seeds, ",")
		cfg["group_replication_ip_allowlist"] = strings.Join(allowList, ",")
		return cfg
	}
	return nil
}

func (m *manager) setupInstances(ctx context.Context, instances []cloud.Instance) error {
	switch m.replicationType {
	case replicationTypeAsync:
		if err := m.setupAsyncInstances(ctx, instances); err != nil {
			return errors.Wrap(err, "failed to setup async instances")
		}
	case replicationTypeGR:
		if err := m.setupGRInstances(ctx, instances); err != nil {
			return errors.Wrap(err, "failed to setup GR instances")
		}
	default:
		return errors.New("unknown replication type")
	}
	for _, instance := range instances {
		if m.pmmAddress != "" {
			_, err := m.runCommand(ctx, instance, cmd.AddServiceToPMM(m.pmmPassword, m.port))
			if err != nil {
				return errors.Wrap(err, "add service to pmm")
			}
		}
	}
	return nil
}

func (m *manager) setupAsyncInstances(ctx context.Context, instances []cloud.Instance) error {
	masterInstance := instances[0]
	db, err := m.newClient(masterInstance, internaldb.UserRoot, m.pass)
	if err != nil {
		return errors.Wrap(err, "new client")
	}
	defer db.Close()
	if err := db.CreateReplicaUser(ctx, m.replicaPass, false); err != nil {
		return errors.Wrap(err, "create replica user")
	}
	for i, instance := range instances {
		serverID := i + 1
		if len(instances) > 1 {
			cfg := m.instanceConfig(ctx, instance, instances, serverID, "")
			if err := m.editDefaultCfg(ctx, instance, "mysqld", cfg); err != nil {
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
			if err := m.startReplica(ctx, instance, masterInstance.PrivateIpAddress); err != nil {
				return errors.Wrap(err, "start replica")
			}
		}
	}
	return nil
}

func (m *manager) setupGRInstances(ctx context.Context, instances []cloud.Instance) error {
	g, gCtx := errgroup.WithContext(ctx)
	groupUUID, err := uuid.GenerateUUID()
	if err != nil {
		return errors.New("failed to generate uuid")
	}
	for i, instance := range instances {
		instance := instance
		serverID := i + 1
		g.Go(func() error {
			cfg := m.instanceConfig(gCtx, instance, instances, serverID, groupUUID)
			if err := m.editDefaultCfg(gCtx, instance, "mysqld", cfg); err != nil {
				return errors.Wrap(err, "edit default cfg for replication")
			}
			_, err = m.runCommand(gCtx, instance, cmd.Restart())
			if err != nil {
				return errors.Wrap(err, "restart mysql")
			}
			db, err := m.newClient(instance, internaldb.UserRoot, m.pass)
			if err != nil {
				return errors.Wrap(err, "new client")
			}
			defer db.Close()

			if err := db.CreateReplicaUser(gCtx, m.replicaPass, true); err != nil {
				return errors.Wrap(err, "failed to create replica user")
			}
			if serverID == 1 {
				if m.replicationType == replicationTypeGR {
					if err := db.ChangeGroupReplicationSource(gCtx, internaldb.UserReplica, m.replicaPass); err != nil {
						return errors.Wrap(err, "failed to change group replication source")
					}
					if err := db.SetGroupReplicationBootstrapGroup(gCtx, true); err != nil {
						return errors.Wrap(err, "set group_replication_bootstrap_group=ON")
					}
					if err := db.StartGroupReplication(gCtx); err != nil {
						return errors.Wrap(err, "start group replication")
					}
					if err := db.SetGroupReplicationBootstrapGroup(gCtx, false); err != nil {
						return errors.Wrap(err, "set group_replication_bootstrap_group=OFF")
					}
				}
			}

			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	for i, instance := range instances {
		err := func() error {
			if i == 0 {
				return nil
			}
			db, err := m.newClient(instance, internaldb.UserRoot, m.pass)
			if err != nil {
				return errors.Wrap(err, "new client")
			}
			defer db.Close()
			if err := db.ChangeGroupReplicationSource(ctx, internaldb.UserReplica, m.replicaPass); err != nil {
				return errors.Wrap(err, "failed to change group replication source")
			}
			if err := db.StartGroupReplication(ctx); err != nil {
				return errors.Wrap(err, "start group replication for instance")
			}
			return nil
		}()
		if err != nil {
			return errors.Wrapf(err, "failed to start group replication for instance %s", instance.PrivateIpAddress)
		}
	}
	return nil
}

func (m *manager) startReplica(ctx context.Context, instance cloud.Instance, masterIP string) error {
	db, err := m.newClient(instance, internaldb.UserRoot, m.pass)
	if err != nil {
		return errors.Wrap(err, "failed to establish sql connection")
	}
	defer db.Close()
	if err := db.ChangeReplicationSource(ctx, masterIP, m.port, internaldb.UserReplica, m.replicaPass); err != nil {
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

func (m *manager) sendFile(ctx context.Context, instance cloud.Instance, file io.Reader, remotePath string) error {
	fileName := path.Base(remotePath)

	tmpPath := path.Join("/opt/percona", fileName)

	err := m.cloud.SendFile(ctx, m.resourceID, instance, file, path.Join("/opt/percona", fileName))
	if err != nil {
		return errors.Wrapf(err, "send file %s", fileName)
	}

	_, err = m.runCommand(ctx, instance, fmt.Sprintf("sudo mv %s %s", tmpPath, remotePath))
	if err != nil {
		return errors.Wrapf(err, "failed to move file from %s to %s", tmpPath, remotePath)
	}

	_, err = m.runCommand(ctx, instance, fmt.Sprintf("sudo chown root:root %s", remotePath))
	if err != nil {
		return errors.Wrapf(err, "failed to change permissions for %s", remotePath)
	}

	return nil
}

func (m *manager) editFile(ctx context.Context, instance cloud.Instance, remotePath string, editFunc func(io.ReadWriteSeeker) error) error {
	fileName := path.Base(remotePath)

	tmpPath := path.Join("/opt/percona", fileName)

	_, err := m.runCommand(ctx, instance, fmt.Sprintf("sudo cp %s %s", remotePath, tmpPath))
	if err != nil {
		return errors.Wrapf(err, "failed to copy file from %s to %s", remotePath, tmpPath)
	}

	_, err = m.runCommand(ctx, instance, fmt.Sprintf("sudo chown ubuntu %s", tmpPath))
	if err != nil {
		return errors.Wrapf(err, "failed to change permissions for %s", tmpPath)
	}

	if err = m.cloud.EditFile(ctx, m.resourceID, instance, tmpPath, editFunc); err != nil {
		return errors.Wrap(err, "failed to edit file")
	}

	_, err = m.runCommand(ctx, instance, fmt.Sprintf("sudo chown root:root %s", tmpPath))
	if err != nil {
		return errors.Wrapf(err, "failed to change permissions for %s", tmpPath)
	}

	_, err = m.runCommand(ctx, instance, fmt.Sprintf("sudo mv %s %s", tmpPath, remotePath))
	if err != nil {
		return errors.Wrapf(err, "failed to move file from %s to %s", tmpPath, remotePath)
	}
	return nil
}
