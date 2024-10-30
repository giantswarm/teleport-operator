package config

import (
	"context"
	"fmt"

	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

type TeleportInstance struct {
	ProxyAddr             string
	TeleportVersion       string
	ManagementClusterName string
	IdentityFile          string
	Enabled               bool
}

type Config struct {
	ProxyAddr             string
	TeleportVersion       string
	ManagementClusterName string
	AppName               string
	AppVersion            string
	AppCatalog            string
	TestInstance          *TeleportInstance
}

func GetConfigFromConfigMap(ctx context.Context, ctrlClient client.Client, namespace string) (*Config, error) {
	configMap := &corev1.ConfigMap{}
	if err := ctrlClient.Get(ctx, types.NamespacedName{
		Name:      key.TeleportOperatorConfigName,
		Namespace: namespace,
	}, configMap); err != nil {
		return nil, microerror.Mask(err)
	}

	proxyAddr, err := getConfigMapString(configMap, key.ProxyAddr)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	managementClusterName, err := getConfigMapString(configMap, key.ManagementClusterName)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	teleportVersion, err := getConfigMapString(configMap, key.TeleportVersion)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appName, err := getConfigMapString(configMap, key.AppName)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appVersion, err := getConfigMapString(configMap, key.AppVersion)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appCatalog, err := getConfigMapString(configMap, key.AppCatalog)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	config := &Config{
		ProxyAddr:             proxyAddr,
		TeleportVersion:       teleportVersion,
		ManagementClusterName: managementClusterName,
		AppName:               appName,
		AppVersion:            appVersion,
		AppCatalog:            appCatalog,
	}

	testProxyAddr, err := getConfigMapString(configMap, "test.proxyAddr")
	if err == nil {
		testClusterName, err := getConfigMapString(configMap, "test.managementClusterName")
		if err != nil {
			return nil, microerror.Mask(err)
		}
		testIdentityFile, err := getConfigMapString(configMap, "test.identityFile")
		if err != nil {
			return nil, microerror.Mask(err)
		}
		testVersion, err := getConfigMapString(configMap, "test.teleportVersion")
		if err != nil {
			testVersion = config.TeleportVersion
		}

		config.TestInstance = &TeleportInstance{
			ProxyAddr:             testProxyAddr,
			TeleportVersion:       testVersion,
			ManagementClusterName: testClusterName,
			IdentityFile:          testIdentityFile,
			Enabled:               true,
		}
	}

	return config, nil
}

func getConfigMapString(configMap *corev1.ConfigMap, key string) (string, error) {
	if s, ok := configMap.Data[key]; ok {
		return s, nil
	}
	if b, ok := configMap.BinaryData[key]; ok {
		return string(b), nil
	}
	return "", fmt.Errorf("malformed Config Map: required key %q not found", key)
}
