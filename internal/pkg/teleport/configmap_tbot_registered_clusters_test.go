package teleport

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

var testLog = ctrl.Log.WithName("test")

var aggregateKey = client.ObjectKey{
	Name:      key.TbotRegisteredClustersConfigmapName,
	Namespace: key.TeleportBotNamespace,
}

func newTbotTeleport() *Teleport {
	return New(key.TeleportBotNamespace, &config.Config{
		AppName:               test.AppName,
		ManagementClusterName: test.ManagementClusterName,
	}, token.NewGenerator())
}

func seedClient(t *testing.T, objects ...runtime.Object) client.Client {
	t.Helper()
	c, err := test.NewFakeK8sClient(objects)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}
	return c
}

func resourceVersion(t *testing.T, ctx context.Context, c client.Client) string {
	t.Helper()
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, aggregateKey, cm); err != nil {
		t.Fatalf("failed to get the ConfigMap: %v", err)
	}
	return cm.ResourceVersion
}

func Test_SetTbotRegisteredClusters_CreatesConfigMap(t *testing.T) {
	ctx := context.TODO()
	ctrlClient := seedClient(t)

	err := newTbotTeleport().SetTbotRegisteredClusters(ctx, testLog, ctrlClient, map[string]string{
		"golem-dingo":  "dingo",
		"golem-badger": "badger",
	})
	if err != nil {
		t.Fatalf("SetTbotRegisteredClusters returned error: %v", err)
	}

	outputs := test.ReadTbotRegisteredClusters(t, ctx, ctrlClient)
	if len(outputs) != 2 || outputs["golem-dingo"] != "dingo" || outputs["golem-badger"] != "badger" {
		t.Errorf("expected both clusters recorded, got %v", outputs)
	}
}

// Entries for clusters no longer in the list are removed.
func Test_SetTbotRegisteredClusters_ReplacesStaleEntries(t *testing.T) {
	ctx := context.TODO()
	ctrlClient := seedClient(t, test.NewTbotRegisteredClustersConfigMap(map[string]string{
		"golem-dingo":  "dingo",
		"golem-orphan": "orphan",
	}))

	err := newTbotTeleport().SetTbotRegisteredClusters(ctx, testLog, ctrlClient, map[string]string{
		"golem-dingo": "dingo",
	})
	if err != nil {
		t.Fatalf("SetTbotRegisteredClusters returned error: %v", err)
	}

	outputs := test.ReadTbotRegisteredClusters(t, ctx, ctrlClient)
	if len(outputs) != 1 || outputs["golem-dingo"] != "dingo" {
		t.Errorf("expected only golem-dingo to remain, got %v", outputs)
	}
}

// The operator owns the whole ConfigMap: anything else in it is dropped.
func Test_SetTbotRegisteredClusters_OwnsWholeConfigMap(t *testing.T) {
	ctx := context.TODO()
	cm := test.NewTbotRegisteredClustersConfigMapWithDoc(map[string]interface{}{
		"outputs":  map[string]string{"golem-dingo": "dingo"},
		"teleport": map[string]string{"proxyAddr": "test.teleport.giantswarm.io:443"},
	})
	cm.Data["other"] = "extra"
	ctrlClient := seedClient(t, cm)

	err := newTbotTeleport().SetTbotRegisteredClusters(ctx, testLog, ctrlClient, map[string]string{
		"golem-badger": "badger",
	})
	if err != nil {
		t.Fatalf("SetTbotRegisteredClusters returned error: %v", err)
	}

	doc := test.ReadTbotRegisteredClustersDoc(t, ctx, ctrlClient)
	if _, ok := doc["teleport"]; ok || len(doc) != 1 {
		t.Errorf("expected only the outputs key, got %v", doc)
	}
	got := &corev1.ConfigMap{}
	if err := ctrlClient.Get(ctx, aggregateKey, got); err != nil {
		t.Fatalf("failed to get the ConfigMap: %v", err)
	}
	if _, ok := got.Data["other"]; ok {
		t.Errorf("expected the extra data key to be removed, got %v", got.Data)
	}
	if got.Labels[key.ManagedByLabel] != key.TeleportOperatorLabelValue {
		t.Errorf("expected the managed-by label, got %v", got.Labels)
	}
	if got.Annotations[key.NotesAnnotation] == "" {
		t.Errorf("expected the notes annotation, got %v", got.Annotations)
	}
}

// Runs on every reconcile, so an unchanged list must not write.
func Test_SetTbotRegisteredClusters_DoesNotWriteWhenUnchanged(t *testing.T) {
	ctx := context.TODO()
	outputs := map[string]string{"golem-dingo": "dingo"}
	ctrlClient := seedClient(t)
	tp := newTbotTeleport()

	if err := tp.SetTbotRegisteredClusters(ctx, testLog, ctrlClient, outputs); err != nil {
		t.Fatalf("SetTbotRegisteredClusters returned error: %v", err)
	}
	before := resourceVersion(t, ctx, ctrlClient)
	if err := tp.SetTbotRegisteredClusters(ctx, testLog, ctrlClient, outputs); err != nil {
		t.Fatalf("SetTbotRegisteredClusters returned error: %v", err)
	}

	if after := resourceVersion(t, ctx, ctrlClient); before != after {
		t.Errorf("expected no write for an unchanged list, resourceVersion moved %s -> %s", before, after)
	}
}

// The ConfigMap exists even with no clusters, so a reference to it from Git
// always resolves.
func Test_SetTbotRegisteredClusters_CreatesEmptyDocumentWhenNoClusters(t *testing.T) {
	ctx := context.TODO()
	ctrlClient := seedClient(t)

	if err := newTbotTeleport().SetTbotRegisteredClusters(ctx, testLog, ctrlClient, map[string]string{}); err != nil {
		t.Fatalf("SetTbotRegisteredClusters returned error: %v", err)
	}

	if outputs := test.ReadTbotRegisteredClusters(t, ctx, ctrlClient); len(outputs) != 0 {
		t.Errorf("expected an empty outputs map, got %v", outputs)
	}
}
