package keystone

const (
	// ServiceName -
	ServiceName = "keystone"
	// DatabaseName -
	DatabaseName = "keystone"
	// KeystoneDatabasePassword - ref name to get the keystone db password from osp-secret
	KeystoneDatabasePassword = "KeystoneDatabasePassword"
	// AdminPassword - ref name to get the admin password from osp-secret
	AdminPassword = "AdminPassword"

	// InputHashName -Name of the hash of hashes of all resources used to indentify an input change
	InputHashName = "input"

	// KollaConfig -
	KollaConfig = "/var/lib/config-data/merged/keystone-api-config.json"

	// InitContainerCommand -
	InitContainerCommand = "/usr/local/bin/container-scripts/init.sh"
	// DBSyncCommand -
	DBSyncCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
	// BootStrapCommand -
	BootStrapCommand = "/usr/local/bin/kolla_set_configs && keystone-manage bootstrap"
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_set_configs && /usr/local/bin/kolla_start"
	// DebugCommand -
	DebugCommand = "/usr/local/bin/kolla_set_configs && /bin/sleep infinity"
)
