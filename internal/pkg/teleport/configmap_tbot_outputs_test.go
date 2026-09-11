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
	Name:      key.TbotOutputsConfigmapName,
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
		t.Fatalf("failed to get the aggregate ConfigMap: %v", err)
	}
	return cm.ResourceVersion
}

func Test_SetTbotOutputs_CreatesConfigMap(t *testing.T) {
	ctx := context.TODO()
	ctrlClient := seedClient(t)

	err := newTbotTeleport().SetTbotOutputs(ctx, testLog, ctrlClient, map[string]string{
		"golem-dingo":  "dingo",
		"golem-badger": "badger",
	})
	if err != nil {
		t.Fatalf("SetTbotOutputs returned error: %v", err)
	}

	outputs := test.ReadTbotOutputs(t, ctx, ctrlClient)
	if len(outputs) != 2 || outputs["golem-dingo"] != "dingo" || outputs["golem-badger"] != "badger" {
		t.Errorf("expected both clusters recorded, got %v", outputs)
	}
}

// The document is a projection, so an entry for a cluster no longer in the
// desired set must disappear — including one orphaned while the operator was
// down, which an incremental merge could never clean up.
func Test_SetTbotOutputs_ReplacesStaleEntries(t *testing.T) {
	ctx := context.TODO()
	ctrlClient := seedClient(t, test.NewTbotOutputsConfigMap(map[string]string{
		"golem-dingo":  "dingo",
		"golem-orphan": "orphan",
	}))

	err := newTbotTeleport().SetTbotOutputs(ctx, testLog, ctrlClient, map[string]string{
		"golem-dingo": "dingo",
	})
	if err != nil {
		t.Fatalf("SetTbotOutputs returned error: %v", err)
	}

	outputs := test.ReadTbotOutputs(t, ctx, ctrlClient)
	if len(outputs) != 1 || outputs["golem-dingo"] != "dingo" {
		t.Errorf("expected only golem-dingo to remain, got %v", outputs)
	}
}

// We own the `outputs` key, not the whole file.
func Test_SetTbotOutputs_PreservesSiblingTopLevelKeys(t *testing.T) {
	ctx := context.TODO()
	ctrlClient := seedClient(t, test.NewTbotOutputsConfigMapWithDoc(map[string]interface{}{
		"outputs":  map[string]string{"golem-dingo": "dingo"},
		"teleport": map[string]string{"proxyAddr": "test.teleport.giantswarm.io:443"},
	}))

	err := newTbotTeleport().SetTbotOutputs(ctx, testLog, ctrlClient, map[string]string{
		"golem-badger": "badger",
	})
	if err != nil {
		t.Fatalf("SetTbotOutputs returned error: %v", err)
	}

	tp, ok := test.ReadTbotOutputsDoc(t, ctx, ctrlClient)["teleport"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected the sibling `teleport` key to survive")
	}
	if tp["proxyAddr"] != "test.teleport.giantswarm.io:443" {
		t.Errorf("expected proxyAddr preserved, got %v", tp)
	}
}

// Called on every reconcile of every cluster, so an unchanged projection must
// not write.
func Test_SetTbotOutputs_DoesNotWriteWhenUnchanged(t *testing.T) {
	ctx := context.TODO()
	outputs := map[string]string{"golem-dingo": "dingo"}
	ctrlClient := seedClient(t, test.NewTbotOutputsConfigMap(outputs))

	before := resourceVersion(t, ctx, ctrlClient)
	if err := newTbotTeleport().SetTbotOutputs(ctx, testLog, ctrlClient, outputs); err != nil {
		t.Fatalf("SetTbotOutputs returned error: %v", err)
	}

	if after := resourceVersion(t, ctx, ctrlClient); before != after {
		t.Errorf("expected no write for an unchanged projection, resourceVersion moved %s -> %s", before, after)
	}
}

// The stored document is compared semantically, not byte-for-byte, so an
// equivalent document indented differently is left alone. Byte comparison would
// rewrite the shared object on every reconcile, forever.
func Test_SetTbotOutputs_IgnoresFormattingDifferences(t *testing.T) {
	ctx := context.TODO()
	cm := test.NewTbotOutputsConfigMap(map[string]string{"golem-dingo": "dingo"})
	// Flow style: same document, different encoding. (Block style with 4-space
	// indent is exactly what yaml.v3 emits, so it would prove nothing here.)
	cm.Data["values"] = "outputs: {golem-dingo: dingo}\n"
	ctrlClient := seedClient(t, cm)

	before := resourceVersion(t, ctx, ctrlClient)
	err := newTbotTeleport().SetTbotOutputs(ctx, testLog, ctrlClient, map[string]string{"golem-dingo": "dingo"})
	if err != nil {
		t.Fatalf("SetTbotOutputs returned error: %v", err)
	}

	if after := resourceVersion(t, ctx, ctrlClient); before != after {
		t.Errorf("expected no write for an equivalent document, resourceVersion moved %s -> %s", before, after)
	}
}

// A management cluster with no Cluster CRs yet still gets the ConfigMap, so the
// reference declared in git resolves from the start.
func Test_SetTbotOutputs_CreatesEmptyDocumentWhenNoClusters(t *testing.T) {
	ctx := context.TODO()
	ctrlClient := seedClient(t)

	if err := newTbotTeleport().SetTbotOutputs(ctx, testLog, ctrlClient, map[string]string{}); err != nil {
		t.Fatalf("SetTbotOutputs returned error: %v", err)
	}

	if outputs := test.ReadTbotOutputs(t, ctx, ctrlClient); len(outputs) != 0 {
		t.Errorf("expected an empty outputs map, got %v", outputs)
	}
}
