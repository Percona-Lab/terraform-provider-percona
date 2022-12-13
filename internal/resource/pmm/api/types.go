package api

type AddRDSRequest struct {
	Region                    string            `json:"region,omitempty"`
	Az                        string            `json:"az,omitempty"`
	InstanceID                string            `json:"instance_id,omitempty"`
	NodeModel                 string            `json:"node_model,omitempty"`
	Address                   string            `json:"address,omitempty"`
	Port                      int64             `json:"port,omitempty"`
	Engine                    string            `json:"engine,omitempty"`
	NodeName                  string            `json:"node_name,omitempty"`
	ServiceName               string            `json:"service_name,omitempty"`
	Environment               string            `json:"environment,omitempty"`
	Cluster                   string            `json:"cluster,omitempty"`
	ReplicationSet            string            `json:"replication_set,omitempty"`
	Username                  string            `json:"username,omitempty"`
	Password                  string            `json:"password,omitempty"`
	AwsAccessKey              string            `json:"aws_access_key,omitempty"`
	AwsSecretKey              string            `json:"aws_secret_key,omitempty"`
	RdsExporter               bool              `json:"rds_exporter,omitempty"`
	QanMysqlPerfschema        bool              `json:"qan_mysql_perfschema,omitempty"`
	CustomLabels              map[string]string `json:"custom_labels,omitempty"`
	SkipConnectionCheck       bool              `json:"skip_connection_check,omitempty"`
	TLS                       bool              `json:"tls,omitempty"`
	TLSSkipVerify             bool              `json:"tls_skip_verify,omitempty"`
	DisableQueryExamples      bool              `json:"disable_query_examples,omitempty"`
	TablestatsGroupTableLimit int               `json:"tablestats_group_table_limit,omitempty"`
	DisableBasicMetrics       bool              `json:"disable_basic_metrics,omitempty"`
	DisableEnhancedMetrics    bool              `json:"disable_enhanced_metrics,omitempty"`
	MetricsMode               int               `json:"metrics_mode,omitempty"`
	QanPostgresqlPgstatements bool              `json:"qan_postgresql_pgstatements,omitempty"`
	AgentPassword             string            `json:"agent_password,omitempty"`
}

type AddRDSResponse struct {
	Node struct {
		NodeID       string            `json:"node_id,omitempty"`
		NodeName     string            `json:"node_name,omitempty"`
		Address      string            `json:"address,omitempty"`
		NodeModel    string            `json:"node_model,omitempty"`
		Region       string            `json:"region,omitempty"`
		Az           string            `json:"az,omitempty"`
		CustomLabels map[string]string `json:"custom_labels,omitempty"`
	} `json:"node,omitempty"`
	RdsExporter struct {
		AgentID                 string            `json:"agent_id,omitempty"`
		PmmAgentID              string            `json:"pmm_agent_id,omitempty"`
		Disabled                bool              `json:"disabled,omitempty"`
		NodeID                  string            `json:"node_id,omitempty"`
		AwsAccessKey            string            `json:"aws_access_key,omitempty"`
		CustomLabels            map[string]string `json:"custom_labels,omitempty"`
		Status                  string            `json:"status,omitempty"`
		ListenPort              int               `json:"listen_port,omitempty"`
		BasicMetricsDisabled    bool              `json:"basic_metrics_disabled,omitempty"`
		EnhancedMetricsDisabled bool              `json:"enhanced_metrics_disabled,omitempty"`
		PushMetricsEnabled      bool              `json:"push_metrics_enabled,omitempty"`
		ProcessExecPath         string            `json:"process_exec_path,omitempty"`
		LogLevel                string            `json:"log_level,omitempty"`
	} `json:"rds_exporter,omitempty"`
	Mysql struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Address        string            `json:"address,omitempty"`
		Port           int               `json:"port,omitempty"`
		Socket         string            `json:"socket,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
	} `json:"mysql,omitempty"`
	MysqldExporter struct {
		AgentID                   string            `json:"agent_id,omitempty"`
		PmmAgentID                string            `json:"pmm_agent_id,omitempty"`
		Disabled                  bool              `json:"disabled,omitempty"`
		ServiceID                 string            `json:"service_id,omitempty"`
		Username                  string            `json:"username,omitempty"`
		TLS                       bool              `json:"tls,omitempty"`
		TLSSkipVerify             bool              `json:"tls_skip_verify,omitempty"`
		TLSCa                     string            `json:"tls_ca,omitempty"`
		TLSCert                   string            `json:"tls_cert,omitempty"`
		TLSKey                    string            `json:"tls_key,omitempty"`
		TablestatsGroupTableLimit int               `json:"tablestats_group_table_limit,omitempty"`
		CustomLabels              map[string]string `json:"custom_labels,omitempty"`
		PushMetricsEnabled        bool              `json:"push_metrics_enabled,omitempty"`
		DisabledCollectors        []string          `json:"disabled_collectors,omitempty"`
		Status                    string            `json:"status,omitempty"`
		ListenPort                int               `json:"listen_port,omitempty"`
		TablestatsGroupDisabled   bool              `json:"tablestats_group_disabled,omitempty"`
		ProcessExecPath           string            `json:"process_exec_path,omitempty"`
		LogLevel                  string            `json:"log_level,omitempty"`
	} `json:"mysqld_exporter,omitempty"`
	QanMysqlPerfschema struct {
		AgentID               string            `json:"agent_id,omitempty"`
		PmmAgentID            string            `json:"pmm_agent_id,omitempty"`
		Disabled              bool              `json:"disabled,omitempty"`
		ServiceID             string            `json:"service_id,omitempty"`
		Username              string            `json:"username,omitempty"`
		TLS                   bool              `json:"tls,omitempty"`
		TLSSkipVerify         bool              `json:"tls_skip_verify,omitempty"`
		TLSCa                 string            `json:"tls_ca,omitempty"`
		TLSCert               string            `json:"tls_cert,omitempty"`
		TLSKey                string            `json:"tls_key,omitempty"`
		MaxQueryLength        int               `json:"max_query_length,omitempty"`
		QueryExamplesDisabled bool              `json:"query_examples_disabled,omitempty"`
		CustomLabels          map[string]string `json:"custom_labels,omitempty"`
		Status                string            `json:"status,omitempty"`
		ProcessExecPath       string            `json:"process_exec_path,omitempty"`
		LogLevel              string            `json:"log_level,omitempty"`
	} `json:"qan_mysql_perfschema,omitempty"`
	TableCount int `json:"table_count,omitempty"`
	Postgresql struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		DatabaseName   string            `json:"database_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Address        string            `json:"address,omitempty"`
		Port           int               `json:"port,omitempty"`
		Socket         string            `json:"socket,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
	} `json:"postgresql,omitempty"`
	PostgresqlExporter struct {
		AgentID            string            `json:"agent_id,omitempty"`
		PmmAgentID         string            `json:"pmm_agent_id,omitempty"`
		Disabled           bool              `json:"disabled,omitempty"`
		ServiceID          string            `json:"service_id,omitempty"`
		Username           string            `json:"username,omitempty"`
		TLS                bool              `json:"tls,omitempty"`
		TLSSkipVerify      bool              `json:"tls_skip_verify,omitempty"`
		CustomLabels       map[string]string `json:"custom_labels,omitempty"`
		PushMetricsEnabled bool              `json:"push_metrics_enabled,omitempty"`
		DisabledCollectors []string          `json:"disabled_collectors,omitempty"`
		Status             string            `json:"status,omitempty"`
		ListenPort         int               `json:"listen_port,omitempty"`
		ProcessExecPath    string            `json:"process_exec_path,omitempty"`
		LogLevel           string            `json:"log_level,omitempty"`
	} `json:"postgresql_exporter,omitempty"`
	QanPostgresqlPgstatements struct {
		AgentID         string            `json:"agent_id,omitempty"`
		PmmAgentID      string            `json:"pmm_agent_id,omitempty"`
		Disabled        bool              `json:"disabled,omitempty"`
		ServiceID       string            `json:"service_id,omitempty"`
		Username        string            `json:"username,omitempty"`
		MaxQueryLength  int               `json:"max_query_length,omitempty"`
		TLS             bool              `json:"tls,omitempty"`
		TLSSkipVerify   bool              `json:"tls_skip_verify,omitempty"`
		CustomLabels    map[string]string `json:"custom_labels,omitempty"`
		Status          string            `json:"status,omitempty"`
		ProcessExecPath string            `json:"process_exec_path,omitempty"`
		LogLevel        string            `json:"log_level,omitempty"`
	} `json:"qan_postgresql_pgstatements,omitempty"`
}

type DiscoverRDSRequest struct {
	AwsAccessKey string `json:"aws_access_key,omitempty"`
	AwsSecretKey string `json:"aws_secret_key,omitempty"`
}

type DiscoverRDSResponse struct {
	RdsInstances []RDSInstance `json:"rds_instances,omitempty"`
}

type RDSInstance struct {
	Region        string `json:"region,omitempty"`
	Az            string `json:"az,omitempty"`
	InstanceID    string `json:"instance_id,omitempty"`
	NodeModel     string `json:"node_model,omitempty"`
	Address       string `json:"address,omitempty"`
	Port          int64  `json:"port,omitempty"`
	Engine        string `json:"engine,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`
}

type ServicesListRequest struct {
	NodeID        string `json:"node_id,omitempty"`
	ServiceType   string `json:"service_type,omitempty"`
	ExternalGroup string `json:"external_group,omitempty"`
}

type ServicesListResponse struct {
	Mysql []struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Address        string            `json:"address,omitempty"`
		Port           int               `json:"port,omitempty"`
		Socket         string            `json:"socket,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
	} `json:"mysql,omitempty"`
	Mongodb []struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Address        string            `json:"address,omitempty"`
		Port           int               `json:"port,omitempty"`
		Socket         string            `json:"socket,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
	} `json:"mongodb,omitempty"`
	Postgresql []struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		DatabaseName   string            `json:"database_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Address        string            `json:"address,omitempty"`
		Port           int               `json:"port,omitempty"`
		Socket         string            `json:"socket,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
	} `json:"postgresql,omitempty"`
	Proxysql []struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Address        string            `json:"address,omitempty"`
		Port           int               `json:"port,omitempty"`
		Socket         string            `json:"socket,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
	} `json:"proxysql,omitempty"`
	Haproxy []struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
	} `json:"haproxy,omitempty"`
	External []struct {
		ServiceID      string            `json:"service_id,omitempty"`
		ServiceName    string            `json:"service_name,omitempty"`
		NodeID         string            `json:"node_id,omitempty"`
		Environment    string            `json:"environment,omitempty"`
		Cluster        string            `json:"cluster,omitempty"`
		ReplicationSet string            `json:"replication_set,omitempty"`
		CustomLabels   map[string]string `json:"custom_labels,omitempty"`
		Group          string            `json:"group,omitempty"`
	} `json:"external,omitempty"`
}

type ServicesRemoveRequest struct {
	ServiceID string `json:"service_id,omitempty"`
	Force     bool   `json:"force,omitempty"`
}
