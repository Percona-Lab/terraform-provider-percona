package cmd

import (
	"fmt"
)

func Restart() string {
	return "sudo systemctl restart mysql"
}

func RetrieveVersions() string {
	return `apt-cache show percona-server-server | grep 'Version' | sed 's/Version: //'`
}

func InstallPerconaServer(password, version string, port int) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	DEBIAN_FRONTEND=noninteractive sudo -E bash -c 'apt-get install -y percona-server-client=%s percona-server-common=%s percona-server-server=%s'
	mysql -uroot -p%s -e "RENAME USER 'root'@'localhost' TO 'root'@'%%';FLUSH PRIVILEGES;"
	sudo chown ubuntu /etc/mysql/mysql.conf.d/
	sudo chown ubuntu /etc/mysql/mysql.conf.d/mysqld.cnf
	`, version, version, version, password)
}

func CreateReplicaUser(rootPassword, replicaPassword string, port int) string {
	return fmt.Sprintf(`mysql -uroot -p%s -e "CREATE USER 'replica_user'@'%%' IDENTIFIED WITH mysql_native_password BY '%s'; GRANT REPLICATION SLAVE ON *.* TO 'replica_user'@'%%'; FLUSH PRIVILEGES;";
	`, rootPassword, replicaPassword)
}

func Configure(password string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	sudo apt-get update
	sudo apt-get install -y gnupg2 curl
	sudo apt-get install -y debconf-utils
	sudo apt-get install -y net-tools

	wget https://repo.percona.com/apt/percona-release_latest.$(lsb_release -sc)_all.deb
	sudo dpkg -i percona-release_latest.$(lsb_release -sc)_all.deb

	sudo apt-get update
	sudo percona-release setup ps80
	export MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE="select Use Strong Password Encryption (RECOMMENDED)"
	echo "percona-server-server   percona-server-server/re-root-pass password %s" | sudo debconf-set-selections
	echo "percona-server-server   percona-server-server/root-pass password %s" | sudo debconf-set-selections
	echo "percona-server-server   percona-server-server/default-auth-override ${MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE}" | sudo debconf-set-selections
	`, password, password)
}

func InstallMyRocks(password, version string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	sudo apt-get install -y percona-server-rocksdb=%s
	sudo ps-admin --enable-rocksdb -uroot -p%s`, version, password)
}
