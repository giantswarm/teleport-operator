package teleport

import (
	"context"
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

// readOutputs returns the `outputs` map stored in the aggregate tbot ConfigMap.
func readOutputs(t *testing.T, ctx context.Context, c client.Client) map[string]string {
	t.Helper()
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{
		Name:      key.TbotOutputsConfigmapName,
		Namespace: key.TeleportBotNamespace,
	}, cm); err != nil {
		t.Fatalf("failed to get aggregate ConfigMap: %v", err)
	}
	var doc struct {
		Outputs map[string]string `yaml:"outputs"`
	}
	if err := yaml.Unmarshal([]byte(cm.Data["values"]), &doc); err != nil {
		t.Fatalf("failed to unmarshal values: %v\ncontent:\n%s", err, cm.Data["values"])
	}
	return doc.Outputs
}

func newTbotTeleport() *Teleport {
	return New(key.TeleportBotNamespace, &config.Config{
		AppName:               test.AppName,
		ManagementClusterName: test.ManagementClusterName,
	}, token.NewGenerator())
}

// EnsureTbotOutput must create the aggregate ConfigMap when it does not exist
// yet and record the cluster's output under `outputs`.
func Test_EnsureTbotOutput_CreatesConfigMapWithEntry(t *testing.T) {
	ctrlClient, err := test.NewFakeK8sClient([]runtime.Object{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	if err := newTbotTeleport().EnsureTbotOutput(ctx, log, ctrlClient, "golem-dingo", "dingo"); err != nil {
		t.Fatalf("EnsureTbotOutput returned error: %v", err)
	}

	outputs := readOutputs(t, ctx, ctrlClient)
	if len(outputs) != 1 || outputs["golem-dingo"] != "dingo" {
		t.Errorf("expected outputs {golem-dingo: dingo}, got %v", outputs)
	}
}

// A second cluster must be added alongside the first, not replace it. This is
// the whole point of the aggregate ConfigMap.
func Test_EnsureTbotOutput_AddsSecondClusterWithoutLosingTheFirst(t *testing.T) {
	ctrlClient, err := test.NewFakeK8sClient([]runtime.Object{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.TODO()
	log := ctrl.Log.WithName("test")
	tele := newTbotTeleport()

	if err := tele.EnsureTbotOutput(ctx, log, ctrlClient, "golem-dingo", "dingo"); err != nil {
		t.Fatalf("first EnsureTbotOutput returned error: %v", err)
	}
	if err := tele.EnsureTbotOutput(ctx, log, ctrlClient, "golem-badger", "badger"); err != nil {
		t.Fatalf("second EnsureTbotOutput returned error: %v", err)
	}

	outputs := readOutputs(t, ctx, ctrlClient)
	if len(outputs) != 2 || outputs["golem-dingo"] != "dingo" || outputs["golem-badger"] != "badger" {
		t.Errorf("expected both clusters present, got %v", outputs)
	}
}

// Removing one cluster on teardown must not disturb the others. This is the
// regression that matters most: a stale read here would silently break every
// other cluster's tbot output.
func Test_RemoveTbotOutput_KeepsOtherClusters(t *testing.T) {
	ctrlClient, err := test.NewFakeK8sClient([]runtime.Object{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.TODO()
	log := ctrl.Log.WithName("test")
	tele := newTbotTeleport()

	for reg, cl := range map[string]string{"golem-dingo": "dingo", "golem-badger": "badger"} {
		if err := tele.EnsureTbotOutput(ctx, log, ctrlClient, reg, cl); err != nil {
			t.Fatalf("EnsureTbotOutput(%s) returned error: %v", reg, err)
		}
	}

	if err := tele.RemoveTbotOutput(ctx, log, ctrlClient, "golem-dingo"); err != nil {
		t.Fatalf("RemoveTbotOutput returned error: %v", err)
	}

	outputs := readOutputs(t, ctx, ctrlClient)
	if len(outputs) != 1 || outputs["golem-badger"] != "badger" {
		t.Errorf("expected only golem-badger to remain, got %v", outputs)
	}
}

// Tearing down a cluster when no aggregate ConfigMap exists must not conjure an
// empty one into being.
func Test_RemoveTbotOutput_DoesNotCreateConfigMapWhenAbsent(t *testing.T) {
	ctrlClient, err := test.NewFakeK8sClient([]runtime.Object{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	if err := newTbotTeleport().RemoveTbotOutput(ctx, log, ctrlClient, "golem-dingo"); err != nil {
		t.Fatalf("RemoveTbotOutput returned error: %v", err)
	}

	cm := &corev1.ConfigMap{}
	err = ctrlClient.Get(ctx, client.ObjectKey{
		Name:      key.TbotOutputsConfigmapName,
		Namespace: key.TeleportBotNamespace,
	}, cm)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected no aggregate ConfigMap to exist, got err=%v cm=%v", err, cm.Data)
	}
}

// The create path runs on every reconcile, so an unchanged entry must not write
// to the object. Writing every time would churn a shared object and its
// resourceVersion for no reason.
func Test_EnsureTbotOutput_DoesNotWriteWhenUnchanged(t *testing.T) {
	ctrlClient, err := test.NewFakeK8sClient([]runtime.Object{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.TODO()
	log := ctrl.Log.WithName("test")
	tele := newTbotTeleport()

	if err := tele.EnsureTbotOutput(ctx, log, ctrlClient, "golem-dingo", "dingo"); err != nil {
		t.Fatalf("first EnsureTbotOutput returned error: %v", err)
	}
	nn := client.ObjectKey{Name: key.TbotOutputsConfigmapName, Namespace: key.TeleportBotNamespace}
	before := &corev1.ConfigMap{}
	if err := ctrlClient.Get(ctx, nn, before); err != nil {
		t.Fatalf("failed to get ConfigMap: %v", err)
	}

	if err := tele.EnsureTbotOutput(ctx, log, ctrlClient, "golem-dingo", "dingo"); err != nil {
		t.Fatalf("second EnsureTbotOutput returned error: %v", err)
	}

	after := &corev1.ConfigMap{}
	if err := ctrlClient.Get(ctx, nn, after); err != nil {
		t.Fatalf("failed to get ConfigMap: %v", err)
	}
	if before.ResourceVersion != after.ResourceVersion {
		t.Errorf("expected no write for an unchanged entry, resourceVersion moved %s -> %s",
			before.ResourceVersion, after.ResourceVersion)
	}
}

// An entry someone added to `outputs` by hand must survive our writes. We own
// the ConfigMap, but we should not silently discard what we did not put there.
func Test_EnsureTbotOutput_PreservesUnknownEntries(t *testing.T) {
	seeded, err := marshalTbotOutputs(map[string]string{"hand-written": "somewhere"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.TbotOutputsConfigmapName,
			Namespace: key.TeleportBotNamespace,
		},
		Data: map[string]string{"values": seeded},
	}
	ctrlClient, err := test.NewFakeK8sClient([]runtime.Object{cm})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ctx := context.TODO()
	log := ctrl.Log.WithName("test")
	tele := newTbotTeleport()

	if err := tele.EnsureTbotOutput(ctx, log, ctrlClient, "golem-dingo", "dingo"); err != nil {
		t.Fatalf("EnsureTbotOutput returned error: %v", err)
	}
	if err := tele.RemoveTbotOutput(ctx, log, ctrlClient, "golem-dingo"); err != nil {
		t.Fatalf("RemoveTbotOutput returned error: %v", err)
	}

	outputs := readOutputs(t, ctx, ctrlClient)
	if len(outputs) != 1 || outputs["hand-written"] != "somewhere" {
		t.Errorf("expected the hand-written entry to survive, got %v", outputs)
	}
}

// Every cluster reconciler writes this one object, so a losing write must retry
// against a fresh read. If it retried with the stale object it would erase the
// entry the winning writer had just added.
func Test_EnsureTbotOutput_RetriesOnConflictWithoutLosingConcurrentWrite(t *testing.T) {
	seeded, err := marshalTbotOutputs(map[string]string{"golem-badger": "badger"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.TbotOutputsConfigmapName,
			Namespace: key.TeleportBotNamespace,
		},
		Data: map[string]string{"values": seeded},
	}
	base := clientfake.NewClientBuilder().WithObjects(cm).Build()

	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	// The first Update loses the race: another reconciler has already added its
	// own cluster, so our write is rejected with a conflict.
	conflicts := 0
	racing := interceptor.NewClient(base, interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if conflicts == 0 {
				conflicts++
				live := &corev1.ConfigMap{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
					return err
				}
				winner, err := marshalTbotOutputs(map[string]string{
					"golem-badger": "badger",
					"golem-otter":  "otter",
				})
				if err != nil {
					return err
				}
				live.Data["values"] = winner
				if err := c.Update(ctx, live); err != nil {
					return err
				}
				return apierrors.NewConflict(
					schema.GroupResource{Resource: "configmaps"}, obj.GetName(),
					errors.New("simulated concurrent write"))
			}
			return c.Update(ctx, obj, opts...)
		},
	})

	if err := newTbotTeleport().EnsureTbotOutput(ctx, log, racing, "golem-dingo", "dingo"); err != nil {
		t.Fatalf("EnsureTbotOutput returned error: %v", err)
	}
	if conflicts != 1 {
		t.Fatalf("expected the interceptor to inject exactly one conflict, got %d", conflicts)
	}

	outputs := readOutputs(t, ctx, racing)
	for reg, want := range map[string]string{"golem-badger": "badger", "golem-otter": "otter", "golem-dingo": "dingo"} {
		if outputs[reg] != want {
			t.Errorf("expected %s=%s to be present, got outputs %v", reg, want, outputs)
		}
	}
}

// Two clusters reconciling for the first time can both find no ConfigMap and
// both try to create it. The loser gets AlreadyExists, which is not a conflict,
// so it must still merge into what the winner created.
func Test_EnsureTbotOutput_RecoversFromCreateRace(t *testing.T) {
	base := clientfake.NewClientBuilder().Build()
	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	creates := 0
	racing := interceptor.NewClient(base, interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if creates == 0 {
				creates++
				winner, err := marshalTbotOutputs(map[string]string{"golem-otter": "otter"})
				if err != nil {
					return err
				}
				if err := c.Create(ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      key.TbotOutputsConfigmapName,
						Namespace: key.TeleportBotNamespace,
					},
					Data: map[string]string{"values": winner},
				}); err != nil {
					return err
				}
				return apierrors.NewAlreadyExists(
					schema.GroupResource{Resource: "configmaps"}, obj.GetName())
			}
			return c.Create(ctx, obj, opts...)
		},
	})

	if err := newTbotTeleport().EnsureTbotOutput(ctx, log, racing, "golem-dingo", "dingo"); err != nil {
		t.Fatalf("EnsureTbotOutput returned error: %v", err)
	}

	outputs := readOutputs(t, ctx, racing)
	if outputs["golem-otter"] != "otter" || outputs["golem-dingo"] != "dingo" {
		t.Errorf("expected both the winner's and our entry, got %v", outputs)
	}
}
