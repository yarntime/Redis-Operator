package v1

const (
	// ClusterNameLabelKey Label key for the ClusterName
	ClusterNameLabelKey string = "redis-operator.k8s.io/cluster-name"
	// PodSpecMD5LabelKey label key for the PodSpec MD5 hash
	PodSpecMD5LabelKey string = "redis-operator.k8s.io/podspec-md5"
	// StatefulSpecMD5LabelKey label key for the statefulsetspec MD5 hash
	StatefulSetSpecMD5LabelKey string = "redis-operator.k8s.io/statefulsetspec-md5"
)
