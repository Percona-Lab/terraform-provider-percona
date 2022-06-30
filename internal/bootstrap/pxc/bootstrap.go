package pxc

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

// TODO: make prettier
// TODO: secure passwords using SecureString
func Configure(clusterAddress []string, password string) string {
	clusterAddrStr := strings.Join(clusterAddress, ",")
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE="select Use Strong Password Encryption (RECOMMENDED)"
	echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/re-root-pass password %s" | debconf-set-selections
	echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/root-pass password %s" | debconf-set-selections
	echo "percona-xtradb-cluster-server   percona-xtradb-cluster-server/default-auth-override ${MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE}" | debconf-set-selections
	DEBIAN_FRONTEND=noninteractive apt-get install -y percona-xtradb-cluster

	CONFIG_PATH="/etc/mysql/mysql.conf.d/mysqld.cnf"
	PRIVATE_NODE_ADDRESS=$(ifconfig | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*' | grep -v '127.0.0.1')
	sed -i "s/^wsrep_cluster_address=.*/wsrep_cluster_address=gcomm:\/\/%s/" $CONFIG_PATH
	sed -i "s/^wsrep_node_name=.*/wsrep_node_name=${PRIVATE_NODE_ADDRESS}/" $CONFIG_PATH
	sed -i "s/^#wsrep_node_address=.*/wsrep_node_address=${PRIVATE_NODE_ADDRESS}/" $CONFIG_PATH

	if grep -q "pxc-encrypt-cluster-traffic" $CONFIG_PATH; then
	:
	else
	echo "pxc-encrypt-cluster-traffic=OFF" | tee -a $CONFIG_PATH
	fi`, password, password, clusterAddrStr)
}

func Start(bootstrap bool) string {
	if bootstrap {
		return "systemctl start mysql@bootstrap.service"
	}
	return "systemctl start mysql"
}
