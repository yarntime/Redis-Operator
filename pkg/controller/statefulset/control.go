package statefulset

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	apps "k8s.io/api/apps/v1beta1"
	kapiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	applisters "k8s.io/client-go/listers/apps/v1beta1"
	"k8s.io/client-go/tools/record"

	rapi "github.com/amadeusitgroup/redis-operator/pkg/api/redis/v1"
	"github.com/golang/glog"
)

type StatefulSetControlInteface interface {
	GetRedisClusterStatefulSet(redisCluster *rapi.RedisCluster) ([]*apps.StatefulSet, error)
	CreateStatefulSet(redisCluster *rapi.RedisCluster) (*apps.StatefulSet, error)
	DeleteStatefulSet(redisCluster *rapi.RedisCluster, statefulSetName string) error
}

var _ StatefulSetControlInteface = &StatefulSetControl{}

// StatefulSetControl contains requieres accessor to managing the RedisCluster statefulset
type StatefulSetControl struct {
	StatefulSetLister applisters.StatefulSetLister
	KubeClient        clientset.Interface
	Recorder          record.EventRecorder
}

// StatefulSetControl builds and returns new StatefulSetControl instance
func NewStatefulSetControl(lister applisters.StatefulSetLister, client clientset.Interface, rec record.EventRecorder) *StatefulSetControl {
	ctrl := &StatefulSetControl{
		StatefulSetLister: lister,
		KubeClient:        client,
		Recorder:          rec,
	}
	return ctrl
}

func (p *StatefulSetControl) GetRedisClusterStatefulSet(redisCluster *rapi.RedisCluster) ([]*apps.StatefulSet, error) {
	selector, err := CreateRedisClusterLabelSelector(redisCluster)
	if err != nil {
		return nil, err
	}
	return p.StatefulSetLister.List(selector)
}

func (p *StatefulSetControl) CreateStatefulSet(redisCluster *rapi.RedisCluster) (*apps.StatefulSet, error) {
	set, err := initSet(redisCluster)
	if err != nil {
		return set, err
	}
	glog.V(6).Infof("CreateStatefulSet: %s/%s", redisCluster.Namespace, set.Name)
	return p.KubeClient.AppsV1beta1().StatefulSets(redisCluster.Namespace).Create(set)
}

func (p *StatefulSetControl) DeleteStatefulSet(redisCluster *rapi.RedisCluster, statefulSetName string) error {
	glog.V(6).Infof("DeleteStatefulSet: %s/%s", redisCluster.Namespace, statefulSetName)
	now := int64(0)
	return p.KubeClient.AppsV1beta1().StatefulSets(redisCluster.Namespace).Delete(statefulSetName, &metav1.DeleteOptions{GracePeriodSeconds: &now})
}

func initSet(redisCluster *rapi.RedisCluster) (*apps.StatefulSet, error) {
	if redisCluster == nil {
		return nil, fmt.Errorf("rediscluster nil pointer")
	}

	desiredLabels, err := GetLabelsSet(redisCluster)
	if err != nil {
		return nil, err
	}
	desiredAnnotations, err := GetAnnotationsSet(redisCluster)
	if err != nil {
		return nil, err
	}

	if redisCluster.Spec.PodTemplate == nil {
		return nil, fmt.Errorf("rediscluster[%s/%s] PodTemplate missing", redisCluster.Namespace, redisCluster.Name)
	}
	podTemplate := *redisCluster.Spec.PodTemplate.DeepCopy()
	podTemplate.Labels = desiredLabels
	setName := redisCluster.Name
	result := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            redisCluster.Name,
			Labels:          desiredLabels,
			Annotations:     desiredAnnotations,
			OwnerReferences: []metav1.OwnerReference{BuildOwnerReference(redisCluster)},
		},
		Spec: apps.StatefulSetSpec{
			Replicas:    rapi.NewInt32(*redisCluster.Spec.NumberOfMaster * (1 + *redisCluster.Spec.ReplicationFactor)),
			ServiceName: setName,
			Template:    podTemplate,
		},
	}

	if redisCluster.Spec.Storage != nil && redisCluster.Spec.Storage.UseExternalDisk {
		configSetVolume(redisCluster.Spec.Storage.DataDiskSize, result, desiredLabels)
	}

	hash, err := GenerateMD5Spec(&result.Spec)
	if err != nil {
		return nil, err
	}
	result.Annotations[rapi.StatefulSetSpecMD5LabelKey] = hash

	return result, nil
}

func configSetVolume(diskSize string, set *apps.StatefulSet, labels map[string]string) {
	volumeSize, _ := resource.ParseQuantity(diskSize)
	set.Spec.Template.Spec.Containers[0].VolumeMounts = []kapiv1.VolumeMount{
		{
			Name:      "redis-data",
			MountPath: "/redis-data",
		},
	}
	set.Spec.VolumeClaimTemplates = []kapiv1.PersistentVolumeClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "redis-data",
				Labels: labels,
			},
			Spec: kapiv1.PersistentVolumeClaimSpec{
				AccessModes: []kapiv1.PersistentVolumeAccessMode{
					kapiv1.ReadWriteOnce,
				},
				Resources: kapiv1.ResourceRequirements{
					Requests: kapiv1.ResourceList{
						kapiv1.ResourceStorage: volumeSize,
					},
				},
			},
		},
	}
}

func GenerateMD5Spec(spec *apps.StatefulSetSpec) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	io.Copy(hash, bytes.NewReader(b))
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// BuildOwnerReference used to build the OwnerReference from a RedisCluster
func BuildOwnerReference(cluster *rapi.RedisCluster) metav1.OwnerReference {
	controllerRef := metav1.OwnerReference{
		APIVersion:         rapi.SchemeGroupVersion.String(),
		Kind:               rapi.ResourceKind,
		Name:               cluster.Name,
		UID:                cluster.UID,
		BlockOwnerDeletion: boolPtr(true),
		Controller:         boolPtr(true),
	}

	return controllerRef
}

func boolPtr(value bool) *bool {
	return &value
}
