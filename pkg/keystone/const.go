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

	// KollaConfig -
	KollaConfig = "/var/lib/config-data/merged/keystone-api-config.json"

	// InitContainerCommand -
	InitContainerCommand = "/usr/local/bin/container-scripts/init.sh"
	// DBSyncCommand -
	DBSyncCommand = "/usr/local/bin/kolla_start"
	// BootStrapCommand -
	BootStrapCommand = "keystone-manage bootstrap"
	// ServiceCommand -
	ServiceCommand = "/usr/local/bin/kolla_start"
	// DebugCommand -
	DebugCommand = "/bin/sleep infinity"
)
