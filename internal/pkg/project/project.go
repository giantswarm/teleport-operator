package project

var (
	description = "The teleport-operator installs teleport kube agent app in all workload clusters managed by a management cluster."
	gitSHA      = "n/a"
	name        = "teleport-operator"
	source      = "https://github.com/giantswarm/teleport-operator"
	version     = "0.1.0-dev"
)

func Description() string {
	return description
}

func GitSHA() string {
	return gitSHA
}

func Name() string {
	return name
}

func Source() string {
	return source
}

func Version() string {
	return version
}
