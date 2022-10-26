package cmd

import (
	"fmt"
)

func RetrieveVersions() string {
	return `apt-cache show percona-xtradb-cluster | grep 'Version' | sed 's/Version: 1://'`
}

func InstallPerconaXtraDBCluster(version string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	DEBIAN_FRONTEND=noninteractive sudo -E bash -c 'apt-get install -y percona-xtradb-cluster-common=1:%s percona-xtradb-cluster-server=1:%s percona-xtradb-cluster-client=1:%s percona-xtradb-cluster=1:%s'

	sudo chown ubuntu /etc/mysql/mysql.conf.d/
	sudo chown ubuntu /etc/mysql/mysql.conf.d/mysqld.cnf
	`, version, version, version, version)
}

func Configure(password string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	sudo apt-get update
	sudo apt-get install net-tools

	# install from repository
	sudo apt-get install debconf-utils
	sudo apt-get install -y wget gnupg2 lsb-release curl
	wget https://repo.percona.com/apt/percona-release_latest.generic_all.deb
	sudo dpkg -i percona-release_latest.generic_all.deb
	sudo apt-get update
	sudo percona-release setup pxc80
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

func Stop(bootstrap bool) string {
	if bootstrap {
		return "sudo systemctl stop mysql@bootstrap.service"
	}
	return "sudo systemctl stop mysql"
}
