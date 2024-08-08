package teleport

import (
	"context"
	"reflect"

	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Teleport) getBotExtraConfig(clusterName string) v1alpha1.AppExtraConfig {
	return v1alpha1.AppExtraConfig{
		Kind:      "configMap",
		Name:      key.GetTbotConfigmapName(clusterName),
		Namespace: key.TeleportBotNamespace,
		Priority:  25}
}

func (t *Teleport) appendBotExtraConfig(appExtraConfigs []v1alpha1.AppExtraConfig, extraConfig v1alpha1.AppExtraConfig) []v1alpha1.AppExtraConfig {
	extraConfigs := []v1alpha1.AppExtraConfig{}
	if appExtraConfigs == nil {
		return append(extraConfigs, extraConfig)
	}
	shouldAppend := true
	for _, config := range appExtraConfigs {
		if reflect.DeepEqual(config, extraConfig) {
			shouldAppend = false
			break
		}
	}
	if shouldAppend {
		extraConfigs = append(appExtraConfigs, extraConfig)
	} else {
		extraConfigs = appExtraConfigs
	}
	return extraConfigs
}

func (t *Teleport) removeBotExtraConfig(appExtraConfigs []v1alpha1.AppExtraConfig, extraConfig v1alpha1.AppExtraConfig) []v1alpha1.AppExtraConfig {
	extraConfigs := []v1alpha1.AppExtraConfig{}
	for _, config := range appExtraConfigs {
		if reflect.DeepEqual(config, extraConfig) {
			continue
		}
		extraConfigs = append(extraConfigs, config)
	}
	return extraConfigs
}

func (t *Teleport) getBotApp(ctx context.Context, ctrlClient client.Client, clusterName string) (*v1alpha1.App, error) {
	app := &v1alpha1.App{}
	key := client.ObjectKey{Name: key.TeleportBotAppName, Namespace: key.TeleportBotNamespace}
	if err := ctrlClient.Get(ctx, key, app); err != nil {
		return app, err
	}
	return app, nil
}

func (t *Teleport) EnsureBotAppExtraConfig(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string) error {
	app, err := t.getBotApp(ctx, ctrlClient, clusterName)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "tbot: App not found", "app", app)
		}
		return err
	}

	appExtraConfigs := []v1alpha1.AppExtraConfig{}
	if app.Spec.ExtraConfigs != nil {
		appExtraConfigs = app.Spec.ExtraConfigs
	}

	extraConfig := t.getBotExtraConfig(clusterName)
	app.Spec.ExtraConfigs = t.appendBotExtraConfig(appExtraConfigs, extraConfig)

	if reflect.DeepEqual(appExtraConfigs, app.Spec.ExtraConfigs) {
		return nil
	}

	log.Info("tbot: Updating app", "app", app)
	if err := ctrlClient.Update(ctx, app); err != nil {
		if errors.IsConflict(err) {
			log.Error(err, "tbot: Conflict detected during app update", "app", app)
		}

		return microerror.Mask(err)
	}

	return nil
}

func (t *Teleport) DeleteBotAppExtraConfig(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string) error {
	app, err := t.getBotApp(ctx, ctrlClient, clusterName)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "tbot: App not found", "app", app)
		}
		return err
	}

	if app.Spec.ExtraConfigs == nil {
		return nil
	}
	appExtraConfigs := app.Spec.ExtraConfigs

	extraConfig := t.getBotExtraConfig(clusterName)
	app.Spec.ExtraConfigs = t.removeBotExtraConfig(appExtraConfigs, extraConfig)

	if reflect.DeepEqual(appExtraConfigs, app.Spec.ExtraConfigs) {
		return nil
	}

	log.Info("tbot: Updating app", "app", app)
	if err := ctrlClient.Update(ctx, app); err != nil {
		if errors.IsConflict(err) {
			log.Error(err, "tbot: Conflict detected during app update", "app", app)
		}

		return microerror.Mask(err)
	}

	return nil
}
