package setup

import (
	_ "embed"
	"fmt"
)

//go:embed init.sh
var initial string

func Initial() string {
	return initial
}

func Start() string {
	return "sudo systemctl start mysql"
}

func Configure(password string) string {
	return fmt.Sprintf(`
	#!/usr/bin/env bash
	export MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE="select Use Strong Password Encryption (RECOMMENDED)"
	echo "percona-server-server   percona-server-server/re-root-pass password %s" | sudo debconf-set-selections
	echo "percona-server-server   percona-server-server/root-pass password %s" | sudo debconf-set-selections
	echo "percona-server-server   percona-server-server/default-auth-override ${MYSQL_SELECTION_DEFAULT_AUTH_OVERRIDE}" | sudo debconf-set-selections
	DEBIAN_FRONTEND=noninteractive sudo -E bash -c 'apt-get install -y percona-server-server'

	mysql -uroot -p%s -e "CREATE FUNCTION fnv1a_64 RETURNS INTEGER SONAME 'libfnv1a_udf.so'"
	mysql -uroot -p%s -e "CREATE FUNCTION fnv_64 RETURNS INTEGER SONAME 'libfnv_udf.so'"
	mysql -uroot -p%s -e "CREATE FUNCTION murmur_hash RETURNS INTEGER SONAME 'libmurmur_udf.so'"

	sudo chown ubuntu /etc/mysql/mysql.conf.d/
	`, password, password, password, password, password)
}

func SetupReplication(serverId int, masterIP, rootPassword, replicaPassword, binlogName, binlogPos string) string {
	cmd := fmt.Sprintf(`
	#!/usr/bin/env bash
	export CONFIG_PATH="/etc/mysql/mysql.conf.d/mysqld.cnf"
	sudo -E bash -c 'sed -i "$ a log_bin = /var/log/mysql/mysql-bin.log" $CONFIG_PATH'
	sudo -E bash -c 'sed -i "$ a server_id = %d" $CONFIG_PATH'
`, serverId)

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
