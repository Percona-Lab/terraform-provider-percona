package setup

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed init.sh
var initial string

func Initial() string {
	return initial
}

func RetrieveVersions() string {
	return `apt-cache show percona-xtradb-cluster | grep 'Version' | sed 's/Version: 1://'`
}

func InstallPerconaXtraDBCluster(clusterAddress []string, version string) string {
	clusterAddrStr := strings.Join(clusterAddress, ",")
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	DEBIAN_FRONTEND=noninteractive sudo -E bash -c 'apt-get install -y percona-xtradb-cluster=1:%s'

	export CONFIG_PATH="/etc/mysql/mysql.conf.d/mysqld.cnf"
	export PRIVATE_NODE_ADDRESS=$(ifconfig | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*' | grep -v '127.0.0.1')
	sudo -E bash -c 'sed -i "s/^wsrep_cluster_address=.*/wsrep_cluster_address=gcomm:\/\/%s/" $CONFIG_PATH'
	sudo -E bash -c 'sed -i "s/^wsrep_node_name=.*/wsrep_node_name=${PRIVATE_NODE_ADDRESS}/" $CONFIG_PATH'
	sudo -E bash -c 'sed -i "s/^#wsrep_node_address=.*/wsrep_node_address=${PRIVATE_NODE_ADDRESS}/" $CONFIG_PATH'

	if grep -q "pxc-encrypt-cluster-traffic" $CONFIG_PATH; then
	:
	else
	echo "pxc-encrypt-cluster-traffic=OFF" | sudo -E bash -c 'tee -a $CONFIG_PATH'
	fi

	sudo chown ubuntu /etc/mysql/mysql.conf.d/`, version, clusterAddrStr)
}

func Configure(password string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	export MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE="select Use Strong Password Encryption (RECOMMENDED)"
	echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/re-root-pass password %s" | sudo debconf-set-selections
	echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/root-pass password %s" | sudo debconf-set-selections
	echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/default-auth-override ${MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE}" | sudo debconf-set-selections
	`, password, password)
}

func Start(bootstrap bool) string {
	if bootstrap {
		return "sudo systemctl start mysql@bootstrap.service"
	}
	return "sudo systemctl start mysql"
}
