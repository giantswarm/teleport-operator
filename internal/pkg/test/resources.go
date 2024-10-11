package test

import (
	"fmt"
	"strings"
	"time"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/gravitational/teleport/api/types"
	teleportTypes "github.com/gravitational/teleport/api/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

const (
	TokenName     = "oldTokenWithLengthOf32Characters"
	NewTokenName  = "newTokenWithLengthOf32Characters"
	ClusterName   = "test-cluster"
	AppName       = "app-name"
	NamespaceName = "test-namespace"
	ClusterKey    = "cluster"
	TokenTypeKey  = "type"
	JoinTokenKey  = "joinToken"

	TokenTypeKube = "kube"
	TokenTypeNode = "node"
	TokenTypeApp  = "app"

	AppCatalog            = "app-catalog"
	AppVersion            = "appVersion"
	ManagementClusterName = "management-cluster"
	ProxyAddr             = "127.0.0.1"
	IdentityFileValue     = "identity-file-value"
	TeleportVersion       = "1.0.0"

	ConfigMapValuesFormat = "authToken: %s\nproxyAddr: %s\nroles: %s\nkubeClusterName: %s\nteleportVersionOverride: %s"
)

var LastReadValue = time.Now()

type MockTokenGenerator struct {
	token string
}

func NewMockTokenGenerator(token string) token.Generator {
	return &MockTokenGenerator{token: token}
}

func (g *MockTokenGenerator) Generate() string {
	return g.token
}

func ObjectKeyFromObjectMeta(objectMeta metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{Namespace: objectMeta.Namespace, Name: objectMeta.Name}
}

func NewSecret(clusterName, namespaceName, tokenName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.GetSecretName(clusterName),
			Namespace: namespaceName,
		},
		Data:       map[string][]byte{JoinTokenKey: []byte(tokenName)},
		StringData: map[string]string{JoinTokenKey: tokenName},
	}
}

func NewIdentitySecret(namespaceName, identityFile string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.TeleportBotSecretName,
			Namespace: namespaceName,
		},
		Data: map[string][]byte{key.Identity: []byte(identityFile)},
	}
}

func NewConfigMap(clusterName, appName, namespaceName, tokenName string, roles []string) *corev1.ConfigMap {
	registerName := key.GetRegisterName(ManagementClusterName, clusterName)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.GetConfigmapName(clusterName, appName),
			Namespace: namespaceName,
		},
		Data: map[string]string{
			"values": fmt.Sprintf(ConfigMapValuesFormat, tokenName, ProxyAddr, strings.Join(roles, ","), registerName, TeleportVersion),
		},
	}
}

func NewToken(tokenName, clusterName string, roles []string, expiry ...time.Time) types.ProvisionToken {
	var expiryTime time.Time
	if len(expiry) > 0 {
		expiryTime = expiry[0]
	} else {
		expiryTime = time.Now().Add(720 * time.Hour)
	}

	newToken := &types.ProvisionTokenV2{
		Metadata: types.Metadata{
			Name: tokenName,
			Labels: map[string]string{
				ClusterKey:   key.GetRegisterName(ManagementClusterName, clusterName),
				TokenTypeKey: strings.Join(roles, ","),
			},
			Expires: &expiryTime,
		},
		Spec: types.ProvisionTokenSpecV2{
			Roles: []types.SystemRole{},
		},
	}
	for _, role := range roles {
		switch role {
		case key.RoleKube:
			newToken.Spec.Roles = append(newToken.Spec.Roles, types.RoleKube)
		case key.RoleApp:
			newToken.Spec.Roles = append(newToken.Spec.Roles, types.RoleApp)
		case key.RoleNode:
			newToken.Spec.Roles = append(newToken.Spec.Roles, types.RoleNode)
		}
	}
	return newToken
}

func NewCluster(name, namespace string, finalizers []string, deletionTimestamp time.Time) *capi.Cluster {
	cluster := &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: finalizers,
		},
	}
	if !deletionTimestamp.IsZero() {
		deletionTime := metav1.NewTime(deletionTimestamp)
		cluster.ObjectMeta.DeletionTimestamp = &deletionTime
	}
	return cluster
}

func NewKubeServer(clusterName, hostId, hostName string) teleportTypes.KubeServer {
	return &teleportTypes.KubernetesServerV3{
		Metadata: teleportTypes.Metadata{
			Name: clusterName,
		},
		Spec: teleportTypes.KubernetesServerSpecV3{
			HostID:   hostId,
			Hostname: hostName,
			Cluster: &teleportTypes.KubernetesClusterV3{
				Metadata: teleportTypes.Metadata{
					Name: key.GetRegisterName(ManagementClusterName, clusterName),
				},
				Spec: teleportTypes.KubernetesClusterSpecV3{},
			},
		},
	}
}

func NewFakeK8sClient(runtimeObjects []runtime.Object) (client.Client, error) {
	schemeBuilder := runtime.SchemeBuilder{}
	schemeBuilder.Register(capi.AddToScheme)
	schemeBuilder.Register(appv1alpha1.AddToScheme)

	err := schemeBuilder.AddToScheme(scheme.Scheme)
	if err != nil {
		return nil, err
	}

	fakeK8sClientBuilder := clientfake.NewClientBuilder().WithScheme(scheme.Scheme)
	if runtimeObjects != nil {
		fakeK8sClientBuilder.WithRuntimeObjects(runtimeObjects...)
	}
	return fakeK8sClientBuilder.Build(), nil
}
