package ps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"terraform-percona/internal/cloud"
	"terraform-percona/internal/db"

	"github.com/go-ini/ini"
	"github.com/pkg/errors"
)

type orchestratorConfiguration struct {
	Debug                                      bool              `json:",omitempty"` // set debug mode (similar to --debug option)
	EnableSyslog                               bool              `json:",omitempty"` // Should logs be directed (in addition) to syslog daemon?
	ListenAddress                              string            `json:",omitempty"` // Where orchestrator HTTP should listen for TCP
	ListenSocket                               string            `json:",omitempty"` // Where orchestrator HTTP should listen for unix socket (default: empty; when given, TCP is disabled)
	HTTPAdvertise                              string            `json:",omitempty"` // optional, for raft setups, what is the HTTP address this node will advertise to its peers (potentially use where behind NAT or when rerouting ports; example: "http://11.22.33.44:3030")
	AgentsServerPort                           string            `json:",omitempty"` // port orchestrator agents talk back to
	MySQLTopologyUser                          string            `json:",omitempty"`
	MySQLTopologyPassword                      string            `json:",omitempty"`
	MySQLTopologyCredentialsConfigFile         string            `json:",omitempty"` // my.cnf style configuration file from where to pick credentials. Expecting `user`, `password` under `[client]` section
	MySQLTopologySSLPrivateKeyFile             string            `json:",omitempty"` // Private key file used to authenticate with a Topology mysql instance with TLS
	MySQLTopologySSLCertFile                   string            `json:",omitempty"` // Certificate PEM file used to authenticate with a Topology mysql instance with TLS
	MySQLTopologySSLCAFile                     string            `json:",omitempty"` // Certificate Authority PEM file used to authenticate with a Topology mysql instance with TLS
	MySQLTopologySSLSkipVerify                 bool              `json:",omitempty"` // If true, do not strictly validate mutual TLS certs for Topology mysql instances
	MySQLTopologyUseMutualTLS                  bool              `json:",omitempty"` // Turn on TLS authentication with the Topology MySQL instances
	MySQLTopologyUseMixedTLS                   bool              `json:",omitempty"` // Mixed TLS and non-TLS authentication with the Topology MySQL instances
	TLSCacheTTLFactor                          uint              `json:",omitempty"` // Factor of InstancePollSeconds that we set as TLS info cache expiry
	BackendDB                                  string            `json:",omitempty"` // EXPERIMENTAL: type of backend db; either "mysql" or "sqlite3"
	SQLite3DataFile                            string            `json:",omitempty"` // when BackendDB == "sqlite3", full path to sqlite3 datafile
	SkipOrchestratorDatabaseUpdate             bool              `json:",omitempty"` // When true, do not check backend database schema nor attempt to update it. Useful when you may be running multiple versions of orchestrator, and you only wish certain boxes to dictate the db structure (or else any time a different orchestrator version runs it will rebuild database schema)
	PanicIfDifferentDatabaseDeploy             bool              `json:",omitempty"` // When true, and this process finds the orchestrator backend DB was provisioned by a different version, panic
	RaftEnabled                                bool              `json:",omitempty"` // When true, setup orchestrator in a raft consensus layout. When false (default) all Raft* variables are ignored
	RaftBind                                   string            `json:",omitempty"`
	RaftAdvertise                              string            `json:",omitempty"`
	RaftDataDir                                string            `json:",omitempty"`
	DefaultRaftPort                            int               `json:",omitempty"` // if a RaftNodes entry does not specify port, use this one
	RaftNodes                                  []string          `json:",omitempty"` // Raft nodes to make initial connection with
	ExpectFailureAnalysisConcensus             bool              `json:",omitempty"`
	MySQLOrchestratorHost                      string            `json:",omitempty"`
	MySQLOrchestratorMaxPoolConnections        int               `json:",omitempty"` // The maximum size of the connection pool to the Orchestrator backend.
	MySQLOrchestratorPort                      uint              `json:",omitempty"`
	MySQLOrchestratorDatabase                  string            `json:",omitempty"`
	MySQLOrchestratorUser                      string            `json:",omitempty"`
	MySQLOrchestratorPassword                  string            `json:",omitempty"`
	MySQLOrchestratorCredentialsConfigFile     string            `json:",omitempty"` // my.cnf style configuration file from where to pick credentials. Expecting `user`, `password` under `[client]` section
	MySQLOrchestratorSSLPrivateKeyFile         string            `json:",omitempty"` // Private key file used to authenticate with the Orchestrator mysql instance with TLS
	MySQLOrchestratorSSLCertFile               string            `json:",omitempty"` // Certificate PEM file used to authenticate with the Orchestrator mysql instance with TLS
	MySQLOrchestratorSSLCAFile                 string            `json:",omitempty"` // Certificate Authority PEM file used to authenticate with the Orchestrator mysql instance with TLS
	MySQLOrchestratorSSLSkipVerify             bool              `json:",omitempty"` // If true, do not strictly validate mutual TLS certs for the Orchestrator mysql instances
	MySQLOrchestratorUseMutualTLS              bool              `json:",omitempty"` // Turn on TLS authentication with the Orchestrator MySQL instance
	MySQLOrchestratorReadTimeoutSeconds        int               `json:",omitempty"` // Number of seconds before backend mysql read operation is aborted (driver-side)
	MySQLOrchestratorRejectReadOnly            bool              `json:",omitempty"` // Reject read only connections https://github.com/go-sql-driver/mysql#rejectreadonly
	MySQLConnectTimeoutSeconds                 int               `json:",omitempty"` // Number of seconds before connection is aborted (driver-side)
	MySQLDiscoveryReadTimeoutSeconds           int               `json:",omitempty"` // Number of seconds before topology mysql read operation is aborted (driver-side). Used for discovery queries.
	MySQLTopologyReadTimeoutSeconds            int               `json:",omitempty"` // Number of seconds before topology mysql read operation is aborted (driver-side). Used for all but discovery queries.
	MySQLConnectionLifetimeSeconds             int               `json:",omitempty"` // Number of seconds the mysql driver will keep database connection alive before recycling it
	DefaultInstancePort                        int               `json:",omitempty"` // In case port was not specified on command line
	SlaveLagQuery                              string            `json:",omitempty"` // Synonym to ReplicationLagQuery
	ReplicationLagQuery                        string            `json:",omitempty"` // custom query to check on replica lg (e.g. heartbeat table). Must return a single row with a single numeric column, which is the lag.
	ReplicationCredentialsQuery                string            `json:",omitempty"` // custom query to get replication credentials. Must return a single row, with five text columns: 1st is username, 2nd is password, 3rd is SSLCaCert, 4th is SSLCert, 5th is SSLKey. This is optional, and can be used by orchestrator to configure replication after master takeover or setup of co-masters. You need to ensure the orchestrator user has the privileges to run this query
	DiscoverByShowSlaveHosts                   bool              `json:",omitempty"` // Attempt SHOW SLAVE HOSTS before PROCESSLIST
	UseSuperReadOnly                           bool              `json:",omitempty"` // Should orchestrator super_read_only any time it sets read_only
	InstancePollSeconds                        uint              `json:",omitempty"` // Number of seconds between instance reads
	ReasonableInstanceCheckSeconds             uint              `json:",omitempty"` // Number of seconds an instance read is allowed to take before it is considered invalid, i.e. before LastCheckValid will be false
	InstanceWriteBufferSize                    int               `json:",omitempty"` // Instance write buffer size (max number of instances to flush in one INSERT ODKU)
	BufferInstanceWrites                       bool              `json:",omitempty"` // Set to 'true' for write-optimization on backend table (compromise: writes can be stale and overwrite non stale data)
	InstanceFlushIntervalMilliseconds          int               `json:",omitempty"` // Max interval between instance write buffer flushes
	SkipMaxScaleCheck                          bool              `json:",omitempty"` // If you don't ever have MaxScale BinlogServer in your topology (and most people don't), set this to 'true' to save some pointless queries
	UnseenInstanceForgetHours                  uint              `json:",omitempty"` // Number of hours after which an unseen instance is forgotten
	SnapshotTopologiesIntervalHours            uint              `json:",omitempty"` // Interval in hour between snapshot-topologies invocation. Default: 0 (disabled)
	DiscoveryMaxConcurrency                    uint              `json:",omitempty"` // Number of goroutines doing hosts discovery
	DiscoveryQueueCapacity                     uint              `json:",omitempty"` // Buffer size of the discovery queue. Should be greater than the number of DB instances being discovered
	DiscoveryQueueMaxStatisticsSize            int               `json:",omitempty"` // The maximum number of individual secondly statistics taken of the discovery queue
	DiscoveryCollectionRetentionSeconds        uint              `json:",omitempty"` // Number of seconds to retain the discovery collection information
	DiscoverySeeds                             []string          `json:",omitempty"` // Hard coded array of hostname:port, ensuring orchestrator discovers these hosts upon startup, assuming not already known to orchestrator
	InstanceBulkOperationsWaitTimeoutSeconds   uint              `json:",omitempty"` // Time to wait on a single instance when doing bulk (many instances) operation
	HostnameResolveMethod                      string            `json:",omitempty"` // Method by which to "normalize" hostname ("none"/"default"/"cname")
	MySQLHostnameResolveMethod                 string            `json:",omitempty"` // Method by which to "normalize" hostname via MySQL server. ("none"/"@@hostname"/"@@report_host"; default "@@hostname")
	SkipBinlogServerUnresolveCheck             bool              `json:",omitempty"` // Skip the double-check that an unresolved hostname resolves back to same hostname for binlog servers
	ExpiryHostnameResolvesMinutes              int               `json:",omitempty"` // Number of minutes after which to expire hostname-resolves
	RejectHostnameResolvePattern               string            `json:",omitempty"` // Regexp pattern for resolved hostname that will not be accepted (not cached, not written to db). This is done to avoid storing wrong resolves due to network glitches.
	ReasonableReplicationLagSeconds            int               `json:",omitempty"` // Above this value is considered a problem
	ProblemIgnoreHostnameFilters               []string          `json:",omitempty"` // Will minimize problem visualization for hostnames matching given regexp filters
	VerifyReplicationFilters                   bool              `json:",omitempty"` // Include replication filters check before approving topology refactoring
	ReasonableMaintenanceReplicationLagSeconds int               `json:",omitempty"` // Above this value move-up and move-below are blocked
	CandidateInstanceExpireMinutes             uint              `json:",omitempty"` // Minutes after which a suggestion to use an instance as a candidate replica (to be preferably promoted on master failover) is expired.
	AuditLogFile                               string            `json:",omitempty"` // Name of log file for audit operations. Disabled when empty.
	AuditToSyslog                              bool              `json:",omitempty"` // If true, audit messages are written to syslog
	AuditToBackendDB                           bool              `json:",omitempty"` // If true, audit messages are written to the backend DB's `audit` table (default: true)
	AuditPurgeDays                             uint              `json:",omitempty"` // Days after which audit entries are purged from the database
	RemoveTextFromHostnameDisplay              string            `json:",omitempty"` // Text to strip off the hostname on cluster/clusters pages
	ReadOnly                                   bool              `json:",omitempty"`
	AuthenticationMethod                       string            `json:",omitempty"` // Type of autherntication to use, if any. "" for none, "basic" for BasicAuth, "multi" for advanced BasicAuth, "proxy" for forwarded credentials via reverse proxy, "token" for token based access
	OAuthClientId                              string            `json:",omitempty"`
	OAuthClientSecret                          string            `json:",omitempty"`
	OAuthScopes                                []string          `json:",omitempty"`
	HTTPAuthUser                               string            `json:",omitempty"` // Username for HTTP Basic authentication (blank disables authentication)
	HTTPAuthPassword                           string            `json:",omitempty"` // Password for HTTP Basic authentication
	AuthUserHeader                             string            `json:",omitempty"` // HTTP header indicating auth user, when AuthenticationMethod is "proxy"
	PowerAuthUsers                             []string          `json:",omitempty"` // On AuthenticationMethod == "proxy", list of users that can make changes. All others are read-only.
	PowerAuthGroups                            []string          `json:",omitempty"` // list of unix groups the authenticated user must be a member of to make changes.
	AccessTokenUseExpirySeconds                uint              `json:",omitempty"` // Time by which an issued token must be used
	AccessTokenExpiryMinutes                   uint              `json:",omitempty"` // Time after which HTTP access token expires
	ClusterNameToAlias                         map[string]string `json:",omitempty"` // map between regex matching cluster name to a human friendly alias
	DetectClusterAliasQuery                    string            `json:",omitempty"` // Optional query (executed on topology instance) that returns the alias of a cluster. Query will only be executed on cluster master (though until the topology's master is resovled it may execute on other/all replicas). If provided, must return one row, one column
	DetectClusterDomainQuery                   string            `json:",omitempty"` // Optional query (executed on topology instance) that returns the VIP/CNAME/Alias/whatever domain name for the master of this cluster. Query will only be executed on cluster master (though until the topology's master is resovled it may execute on other/all replicas). If provided, must return one row, one column
	DetectInstanceAliasQuery                   string            `json:",omitempty"` // Optional query (executed on topology instance) that returns the alias of an instance. If provided, must return one row, one column
	DetectPromotionRuleQuery                   string            `json:",omitempty"` // Optional query (executed on topology instance) that returns the promotion rule of an instance. If provided, must return one row, one column.
	DataCenterPattern                          string            `json:",omitempty"` // Regexp pattern with one group, extracting the datacenter name from the hostname
	RegionPattern                              string            `json:",omitempty"` // Regexp pattern with one group, extracting the region name from the hostname
	PhysicalEnvironmentPattern                 string            `json:",omitempty"` // Regexp pattern with one group, extracting physical environment info from hostname (e.g. combination of datacenter & prod/dev env)
	DetectDataCenterQuery                      string            `json:",omitempty"` // Optional query (executed on topology instance) that returns the data center of an instance. If provided, must return one row, one column. Overrides DataCenterPattern and useful for installments where DC cannot be inferred by hostname
	DetectRegionQuery                          string            `json:",omitempty"` // Optional query (executed on topology instance) that returns the region of an instance. If provided, must return one row, one column. Overrides RegionPattern and useful for installments where Region cannot be inferred by hostname
	DetectPhysicalEnvironmentQuery             string            `json:",omitempty"` // Optional query (executed on topology instance) that returns the physical environment of an instance. If provided, must return one row, one column. Overrides PhysicalEnvironmentPattern and useful for installments where env cannot be inferred by hostname
	DetectSemiSyncEnforcedQuery                string            `json:",omitempty"` // Optional query (executed on topology instance) to determine whether semi-sync is fully enforced for master writes (async fallback is not allowed under any circumstance). If provided, must return one row, one column, value 0 or 1.
	SupportFuzzyPoolHostnames                  bool              `json:",omitempty"` // Should "submit-pool-instances" command be able to pass list of fuzzy instances (fuzzy means non-fqdn, but unique enough to recognize). Defaults 'true', implies more queries on backend db
	InstancePoolExpiryMinutes                  uint              `json:",omitempty"` // Time after which entries in database_instance_pool are expired (resubmit via `submit-pool-instances`)
	PromotionIgnoreHostnameFilters             []string          `json:",omitempty"` // Orchestrator will not promote replicas with hostname matching pattern (via -c recovery; for example, avoid promoting dev-dedicated machines)
	ServeAgentsHttp                            bool              `json:",omitempty"` // Spawn another HTTP interface dedicated for orchestrator-agent
	AgentsUseSSL                               bool              `json:",omitempty"` // When "true" orchestrator will listen on agents port with SSL as well as connect to agents via SSL
	AgentsUseMutualTLS                         bool              `json:",omitempty"` // When "true" Use mutual TLS for the server to agent communication
	AgentSSLSkipVerify                         bool              `json:",omitempty"` // When using SSL for the Agent, should we ignore SSL certification error
	AgentSSLPrivateKeyFile                     string            `json:",omitempty"` // Name of Agent SSL private key file, applies only when AgentsUseSSL = true
	AgentSSLCertFile                           string            `json:",omitempty"` // Name of Agent SSL certification file, applies only when AgentsUseSSL = true
	AgentSSLCAFile                             string            `json:",omitempty"` // Name of the Agent Certificate Authority file, applies only when AgentsUseSSL = true
	AgentSSLValidOUs                           []string          `json:",omitempty"` // Valid organizational units when using mutual TLS to communicate with the agents
	UseSSL                                     bool              `json:",omitempty"` // Use SSL on the server web port
	UseMutualTLS                               bool              `json:",omitempty"` // When "true" Use mutual TLS for the server's web and API connections
	SSLSkipVerify                              bool              `json:",omitempty"` // When using SSL, should we ignore SSL certification error
	SSLPrivateKeyFile                          string            `json:",omitempty"` // Name of SSL private key file, applies only when UseSSL = true
	SSLCertFile                                string            `json:",omitempty"` // Name of SSL certification file, applies only when UseSSL = true
	SSLCAFile                                  string            `json:",omitempty"` // Name of the Certificate Authority file, applies only when UseSSL = true
	SSLValidOUs                                []string          `json:",omitempty"` // Valid organizational units when using mutual TLS
	StatusEndpoint                             string            `json:",omitempty"` // Override the status endpoint.  Defaults to '/api/status'
	StatusOUVerify                             bool              `json:",omitempty"` // If true, try to verify OUs when Mutual TLS is on.  Defaults to false
	AgentPollMinutes                           uint              `json:",omitempty"` // Minutes between agent polling
	UnseenAgentForgetHours                     uint              `json:",omitempty"` // Number of hours after which an unseen agent is forgotten
	StaleSeedFailMinutes                       uint              `json:",omitempty"` // Number of minutes after which a stale (no progress) seed is considered failed.
	SeedAcceptableBytesDiff                    int64             `json:",omitempty"` // Difference in bytes between seed source & target data size that is still considered as successful copy
	SeedWaitSecondsBeforeSend                  int64             `json:",omitempty"` // Number of seconds for waiting before start send data command on agent
	AutoPseudoGTID                             bool              `json:",omitempty"` // Should orchestrator automatically inject Pseudo-GTID entries to the masters
	PseudoGTIDPattern                          string            `json:",omitempty"` // Pattern to look for in binary logs that makes for a unique entry (pseudo GTID). When empty, Pseudo-GTID based refactoring is disabled.
	PseudoGTIDPatternIsFixedSubstring          bool              `json:",omitempty"` // If true, then PseudoGTIDPattern is not treated as regular expression but as fixed substring, and can boost search time
	PseudoGTIDMonotonicHint                    string            `json:",omitempty"` // subtring in Pseudo-GTID entry which indicates Pseudo-GTID entries are expected to be monotonically increasing
	DetectPseudoGTIDQuery                      string            `json:",omitempty"` // Optional query which is used to authoritatively decide whether pseudo gtid is enabled on instance
	BinlogEventsChunkSize                      int               `json:",omitempty"` // Chunk size (X) for SHOW BINLOG|RELAYLOG EVENTS LIMIT ?,X statements. Smaller means less locking and mroe work to be done
	SkipBinlogEventsContaining                 []string          `json:",omitempty"` // When scanning/comparing binlogs for Pseudo-GTID, skip entries containing given texts. These are NOT regular expressions (would consume too much CPU while scanning binlogs), just substrings to find.
	ReduceReplicationAnalysisCount             bool              `json:",omitempty"` // When true, replication analysis will only report instances where possibility of handled problems is possible in the first place (e.g. will not report most leaf nodes, that are mostly uninteresting). When false, provides an entry for every known instance
	FailureDetectionPeriodBlockMinutes         int               `json:",omitempty"` // The time for which an instance's failure discovery is kept "active", so as to avoid concurrent "discoveries" of the instance's failure; this preceeds any recovery process, if any.
	RecoveryPeriodBlockMinutes                 int               `json:",omitempty"` // (supported for backwards compatibility but please use newer `RecoveryPeriodBlockSeconds` instead) The time for which an instance's recovery is kept "active", so as to avoid concurrent recoveries on same instance as well as flapping
	RecoveryPeriodBlockSeconds                 int               `json:",omitempty"` // (overrides `RecoveryPeriodBlockMinutes`) The time for which an instance's recovery is kept "active", so as to avoid concurrent recoveries on same instance as well as flapping
	RecoveryIgnoreHostnameFilters              []string          `json:",omitempty"` // Recovery analysis will completely ignore hosts matching given patterns
	RecoverMasterClusterFilters                []string          `json:",omitempty"` // Only do master recovery on clusters matching these regexp patterns (of course the ".*" pattern matches everything)
	RecoverIntermediateMasterClusterFilters    []string          `json:",omitempty"` // Only do IM recovery on clusters matching these regexp patterns (of course the ".*" pattern matches everything)
	ProcessesShellCommand                      string            `json:",omitempty"` // Shell that executes command scripts
	OnFailureDetectionProcesses                []string          `json:",omitempty"` // Processes to execute when detecting a failover scenario (before making a decision whether to failover or not). May and should use some of these placeholders: {failureType}, {instanceType}, {isMaster}, {isCoMaster}, {failureDescription}, {command}, {failedHost}, {failureCluster}, {failureClusterAlias}, {failureClusterDomain}, {failedPort}, {successorHost}, {successorPort}, {successorAlias}, {countReplicas}, {replicaHosts}, {isDowntimed}, {autoMasterRecovery}, {autoIntermediateMasterRecovery}
	PreGracefulTakeoverProcesses               []string          `json:",omitempty"` // Processes to execute before doing a failover (aborting operation should any once of them exits with non-zero code; order of execution undefined). May and should use some of these placeholders: {failureType}, {instanceType}, {isMaster}, {isCoMaster}, {failureDescription}, {command}, {failedHost}, {failureCluster}, {failureClusterAlias}, {failureClusterDomain}, {failedPort}, {successorHost}, {successorPort}, {countReplicas}, {replicaHosts}, {isDowntimed}
	PreFailoverProcesses                       []string          `json:",omitempty"` // Processes to execute before doing a failover (aborting operation should any once of them exits with non-zero code; order of execution undefined). May and should use some of these placeholders: {failureType}, {instanceType}, {isMaster}, {isCoMaster}, {failureDescription}, {command}, {failedHost}, {failureCluster}, {failureClusterAlias}, {failureClusterDomain}, {failedPort}, {countReplicas}, {replicaHosts}, {isDowntimed}
	PostFailoverProcesses                      []string          `json:",omitempty"` // Processes to execute after doing a failover (order of execution undefined). May and should use some of these placeholders: {failureType}, {instanceType}, {isMaster}, {isCoMaster}, {failureDescription}, {command}, {failedHost}, {failureCluster}, {failureClusterAlias}, {failureClusterDomain}, {failedPort}, {successorHost}, {successorPort}, {successorBinlogCoordinates}, {successorAlias}, {countReplicas}, {replicaHosts}, {isDowntimed}, {isSuccessful}, {lostReplicas}, {countLostReplicas}
	PostUnsuccessfulFailoverProcesses          []string          `json:",omitempty"` // Processes to execute after a not-completely-successful failover (order of execution undefined). May and should use some of these placeholders: {failureType}, {instanceType}, {isMaster}, {isCoMaster}, {failureDescription}, {command}, {failedHost}, {failureCluster}, {failureClusterAlias}, {failureClusterDomain}, {failedPort}, {successorHost}, {successorPort}, {successorBinlogCoordinates}, {successorAlias}, {countReplicas}, {replicaHosts}, {isDowntimed}, {isSuccessful}, {lostReplicas}, {countLostReplicas}
	PostMasterFailoverProcesses                []string          `json:",omitempty"` // Processes to execute after doing a master failover (order of execution undefined). Uses same placeholders as PostFailoverProcesses
	PostIntermediateMasterFailoverProcesses    []string          `json:",omitempty"` // Processes to execute after doing a master failover (order of execution undefined). Uses same placeholders as PostFailoverProcesses
	PostGracefulTakeoverProcesses              []string          `json:",omitempty"` // Processes to execute after runnign a graceful master takeover. Uses same placeholders as PostFailoverProcesses
	PostTakeMasterProcesses                    []string          `json:",omitempty"` // Processes to execute after a successful Take-Master event has taken place
	RecoverNonWriteableMaster                  bool              `json:",omitempty"` // When 'true', orchestrator treats a read-only master as a failure scenario and attempts to make the master writeable
	CoMasterRecoveryMustPromoteOtherCoMaster   bool              `json:",omitempty"` // When 'false', anything can get promoted (and candidates are prefered over others). When 'true', orchestrator will promote the other co-master or else fail
	DetachLostSlavesAfterMasterFailover        bool              `json:",omitempty"` // synonym to DetachLostReplicasAfterMasterFailover
	DetachLostReplicasAfterMasterFailover      bool              `json:",omitempty"` // Should replicas that are not to be lost in master recovery (i.e. were more up-to-date than promoted replica) be forcibly detached
	ApplyMySQLPromotionAfterMasterFailover     bool              `json:",omitempty"` // Should orchestrator take upon itself to apply MySQL master promotion: set read_only=0, detach replication, etc.
	PreventCrossDataCenterMasterFailover       bool              `json:",omitempty"` // When true (default: false), cross-DC master failover are not allowed, orchestrator will do all it can to only fail over within same DC, or else not fail over at all.
	PreventCrossRegionMasterFailover           bool              `json:",omitempty"` // When true (default: false), cross-region master failover are not allowed, orchestrator will do all it can to only fail over within same region, or else not fail over at all.
	MasterFailoverLostInstancesDowntimeMinutes uint              `json:",omitempty"` // Number of minutes to downtime any server that was lost after a master failover (including failed master & lost replicas). 0 to disable
	MasterFailoverDetachSlaveMasterHost        bool              `json:",omitempty"` // synonym to MasterFailoverDetachReplicaMasterHost
	MasterFailoverDetachReplicaMasterHost      bool              `json:",omitempty"` // Should orchestrator issue a detach-replica-master-host on newly promoted master (this makes sure the new master will not attempt to replicate old master if that comes back to life). Defaults 'false'. Meaningless if ApplyMySQLPromotionAfterMasterFailover is 'true'.
	FailMasterPromotionOnLagMinutes            uint              `json:",omitempty"` // when > 0, fail a master promotion if the candidate replica is lagging >= configured number of minutes.
	FailMasterPromotionIfSQLThreadNotUpToDate  bool              `json:",omitempty"` // when true, and a master failover takes place, if candidate master has not consumed all relay logs, promotion is aborted with error
	DelayMasterPromotionIfSQLThreadNotUpToDate bool              `json:",omitempty"` // when true, and a master failover takes place, if candidate master has not consumed all relay logs, delay promotion until the sql thread has caught up
	PostponeSlaveRecoveryOnLagMinutes          uint              `json:",omitempty"` // Synonym to PostponeReplicaRecoveryOnLagMinutes
	PostponeReplicaRecoveryOnLagMinutes        uint              `json:",omitempty"` // On crash recovery, replicas that are lagging more than given minutes are only resurrected late in the recovery process, after master/IM has been elected and processes executed. Value of 0 disables this feature
	OSCIgnoreHostnameFilters                   []string          `json:",omitempty"` // OSC replicas recommendation will ignore replica hostnames matching given patterns
	GraphiteAddr                               string            `json:",omitempty"` // Optional; address of graphite port. If supplied, metrics will be written here
	GraphitePath                               string            `json:",omitempty"` // Prefix for graphite path. May include {hostname} magic placeholder
	GraphiteConvertHostnameDotsToUnderscores   bool              `json:",omitempty"` // If true, then hostname's dots are converted to underscores before being used in graphite path
	GraphitePollSeconds                        int               `json:",omitempty"` // Graphite writes interval. 0 disables.
	URLPrefix                                  string            `json:",omitempty"` // URL prefix to run orchestrator on non-root web path, e.g. /orchestrator to put it behind nginx.
	DiscoveryIgnoreReplicaHostnameFilters      []string          `json:",omitempty"` // Regexp filters to apply to prevent auto-discovering new replicas. Usage: unreachable servers due to firewalls, applications which trigger binlog dumps
	DiscoveryIgnoreMasterHostnameFilters       []string          `json:",omitempty"` // Regexp filters to apply to prevent auto-discovering a master. Usage: pointing your master temporarily to replicate some data from external host
	DiscoveryIgnoreHostnameFilters             []string          `json:",omitempty"` // Regexp filters to apply to prevent discovering instances of any kind
	ConsulAddress                              string            `json:",omitempty"` // Address where Consul HTTP api is found. Example: 127.0.0.1:8500
	ConsulScheme                               string            `json:",omitempty"` // Scheme (http or https) for Consul
	ConsulAclToken                             string            `json:",omitempty"` // ACL token used to write to Consul KV
	ConsulCrossDataCenterDistribution          bool              `json:",omitempty"` // should orchestrator automatically auto-deduce all consul DCs and write KVs in all DCs
	ConsulKVStoreProvider                      string            `json:",omitempty"` // Consul KV store provider (consul or consul-txn), default: "consul"
	ConsulMaxKVsPerTransaction                 int               `json:",omitempty"` // Maximum number of KV operations to perform in a single Consul Transaction. Requires the "consul-txn" ConsulKVStoreProvider
	ZkAddress                                  string            `json:",omitempty"` // UNSUPPERTED YET. Address where (single or multiple) ZooKeeper servers are found, in `srv1[:port1][,srv2[:port2]...]` format. Default port is 2181. Example: srv-a,srv-b:12181,srv-c
	KVClusterMasterPrefix                      string            `json:",omitempty"` // Prefix to use for clusters' masters entries in KV stores (internal, consul, ZK), default: "mysql/master"
	WebMessage                                 string            `json:",omitempty"` // If provided, will be shown on all web pages below the title bar
	MaxConcurrentReplicaOperations             int               `json:",omitempty"` // Maximum number of concurrent operations on replicas
	EnforceExactSemiSyncReplicas               bool              `json:",omitempty"` // If true, semi-sync replicas will be enabled/disabled to match the wait count in the desired priority order; this applies to LockedSemiSyncMaster and MasterWithTooManySemiSyncReplicas
	RecoverLockedSemiSyncMaster                bool              `json:",omitempty"` // If true, orchestrator will recover from a LockedSemiSync state by enabling semi-sync on replicas to match the wait count; this behavior can be overridden by EnforceExactSemiSyncReplicas
	ReasonableLockedSemiSyncMasterSeconds      uint              `json:",omitempty"` // Time to evaluate the LockedSemiSyncHypothesis before triggering the LockedSemiSync analysis; falls back to ReasonableReplicationLagSeconds if not set
}

const (
	defaultOrchestratorURLPrefix       = "orchestrator"
	defaultOrchestratorListenPort      = 3000
	defaultOrchestratorCredentialsPath = "/etc/mysql/orchestrator-topology.cnf"
	defaultOrchestratorConfigPath      = "/etc/orchestrator.conf.json"
)

func orchestratorConfig(instance cloud.Instance, instances []cloud.Instance) ([]byte, error) {
	raftNodes := []string{}
	for _, i := range instances {
		raftNodes = append(raftNodes, i.PrivateIpAddress)
	}
	cfg := &orchestratorConfiguration{
		Debug:                              true,
		ListenAddress:                      fmt.Sprintf(":%d", defaultOrchestratorListenPort),
		RaftEnabled:                        true,
		RaftNodes:                          raftNodes,
		RaftDataDir:                        "/var/lib/orchestrator",
		RaftBind:                           instance.PrivateIpAddress,
		DefaultRaftPort:                    10008,
		URLPrefix:                          fmt.Sprintf("/%s", defaultOrchestratorURLPrefix),
		MySQLTopologyCredentialsConfigFile: defaultOrchestratorCredentialsPath,
		InstancePollSeconds:                5,
		DiscoverByShowSlaveHosts:           false,
		BackendDB:                          "sqlite",
		SQLite3DataFile:                    "/var/lib/orchestrator/orchestrator.db",
		HostnameResolveMethod:              "none",
		MySQLHostnameResolveMethod:         "@@hostname",
		InstanceFlushIntervalMilliseconds:  100,
	}
	return json.Marshal(cfg)
}

func orchestratorTopologyCredentials(password string) (io.Reader, error) {
	f := ini.Empty()
	f.Section("client").Key("user").SetValue(db.UserOrchestrator)
	f.Section("client").Key("password").SetValue(password)
	b := new(bytes.Buffer)
	_, err := f.WriteTo(b)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write ini to buffer")
	}
	return b, nil
}
