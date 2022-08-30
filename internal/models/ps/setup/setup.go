package setup

import (
	_ "embed"
	"fmt"
)

const mysqlConfigPath = "/etc/mysql/mysql.conf.d/mysqld.cnf"

func Initial() string {
	return `#!/usr/bin/env bash
		sudo apt-get update
		sudo apt-get install -y gnupg2 curl
		sudo apt-get install -y debconf-utils
		sudo apt-get install -y net-tools

		wget https://repo.percona.com/apt/percona-release_latest.$(lsb_release -sc)_all.deb
		sudo dpkg -i percona-release_latest.$(lsb_release -sc)_all.deb

		sudo apt-get update
		sudo percona-release setup ps80`
}

func Restart() string {
	return "sudo systemctl restart mysql"
}

func RetrieveVersions() string {
	return `apt-cache show percona-server-server | grep 'Version' | sed 's/Version: //'`
}

func InstallPerconaServer(password, version string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	DEBIAN_FRONTEND=noninteractive sudo -E bash -c 'apt-get install -y percona-server-client=%s percona-server-common=%s percona-server-server=%s'

	mysql -uroot -p%s -e "CREATE FUNCTION fnv1a_64 RETURNS INTEGER SONAME 'libfnv1a_udf.so'"
	mysql -uroot -p%s -e "CREATE FUNCTION fnv_64 RETURNS INTEGER SONAME 'libfnv_udf.so'"
	mysql -uroot -p%s -e "CREATE FUNCTION murmur_hash RETURNS INTEGER SONAME 'libmurmur_udf.so'"

	sudo chown ubuntu /etc/mysql/mysql.conf.d/`, version, version, version, password, password, password)
}

func Configure(password string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
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
	sudo ps-admin --enable-rocksdb -uroot -p%s

	sudo -E bash -c 'sed -i "$ a default-storage-engine=rocksdb" %s'
`, version, password, mysqlConfigPath)
}

func SetupReplication(serverId int, masterIP, rootPassword, replicaPassword, binlogName, binlogPos string) string {
	cmd := fmt.Sprintf(`
	#!/usr/bin/env bash
	export CONFIG_PATH="%s"
	sudo -E bash -c 'sed -i "$ a log_bin = /var/log/mysql/mysql-bin.log" $CONFIG_PATH'
	sudo -E bash -c 'sed -i "$ a server_id = %d" $CONFIG_PATH'
`, mysqlConfigPath, serverId)

	if serverId == 1 {
		cmd += fmt.Sprintf(`
			mysql -uroot -p%s -e "CREATE USER 'replica_user'@'%%' IDENTIFIED WITH mysql_native_password BY '%s'; GRANT REPLICATION SLAVE ON *.* TO 'replica_user'@'%%'; FLUSH PRIVILEGES;";
			sudo -E bash -c 'sed -i "$ a bind-address = %s" $CONFIG_PATH'
        `, rootPassword, replicaPassword, masterIP)
		return cmd
	}
	cmd += fmt.Sprintf(`
		sudo -E bash -c 'sed -i "$ a relay-log = /var/log/mysql/mysql-relay-bin.log" $CONFIG_PATH'
        mysql -uroot -p%s -e "SET GLOBAL server_id=%d; CHANGE REPLICATION SOURCE TO SOURCE_HOST='%s', SOURCE_USER='replica_user', SOURCE_PASSWORD='%s', SOURCE_LOG_FILE='%s', SOURCE_LOG_POS=%s; START REPLICA;"
	`, rootPassword, serverId, masterIP, replicaPassword, binlogName, binlogPos)
	return cmd
}

func ShowMasterStatus(pass string) string {
	return fmt.Sprintf(`mysql -uroot -p%s -Ne "SHOW MASTER STATUS" 2>&1 | grep -v "mysql:"`, pass)
}
