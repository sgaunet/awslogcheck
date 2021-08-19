package main

// Format of the fluentd Docker logs
type fluentDockerLog struct {
	Log        string          `json:"log"`
	Kubernetes kubernetesInfos `json:"kubernetes"`
}

// Subpart of fluent Docker logs
type kubernetesInfos struct {
	PodName        string `json:"pod_name"`
	ContainerImage string `json:"container_image"`
	NamespaceName  string `json:"namespace_name"`
}
