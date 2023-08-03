package teleport

type Config struct {
	Namespace string
}

type ClusterRegisterConfig struct {
	ClusterName         string
	RegisterName        string
	InstallNamespace    string
	IsManagementCluster bool
}
