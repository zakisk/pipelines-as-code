package reconciler

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/keys"
	pacv1alpha1 "github.com/openshift-pipelines/pipelines-as-code/pkg/apis/pipelinesascode/v1alpha1"
	paclisters "github.com/openshift-pipelines/pipelines-as-code/pkg/generated/listers/pipelinesascode/v1alpha1"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/kubeinteraction"
	"github.com/openshift-pipelines/pipelines-as-code/pkg/params"
	queuepkg "github.com/openshift-pipelines/pipelines-as-code/pkg/queue"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	tektonfake "github.com/tektoncd/pipeline/pkg/client/clientset/versioned/fake"
	"go.uber.org/zap"
	"gotest.tools/v3/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/system"
)

func TestPeriodicRequeueCadence(t *testing.T) {
	t.Setenv(system.NamespaceEnvKey, "pipelines-as-code")
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		repo := &pacv1alpha1.Repository{ObjectMeta: metav1.ObjectMeta{Namespace: "ci", Name: "app"}}
		indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, nil)
		assert.NilError(t, indexer.Add(repo))
		pr := &tektonv1.PipelineRun{ObjectMeta: metav1.ObjectMeta{
			Namespace:       repo.Namespace,
			Name:            "build",
			UID:             "build-uid",
			ResourceVersion: "1",
			Annotations: map[string]string{
				keys.State:      kubeinteraction.StateStarted,
				keys.Repository: repo.Name,
			},
		}}
		run := params.New()
		client := tektonfake.NewSimpleClientset(pr)
		run.Clients.Tekton = client
		run.Clients.Log = zap.NewNop().Sugar()
		runs := run.Clients.Tekton.TektonV1().PipelineRuns(pr.Namespace)
		impl := controller.NewContext(ctx, &fakeReconciler{}, controller.ControllerOptions{Logger: run.Clients.Log})
		queue := impl.WorkQueue()
		defer queue.ShutDown()
		r := &Reconciler{
			run:          run,
			repoLister:   paclisters.NewRepositoryLister(indexer),
			qm:           queuepkg.NewManager(run.Clients.Log),
			enqueueAfter: impl.EnqueueAfter,
		}
		drain := func() {
			t.Helper()
			synctest.Wait()
			assert.Equal(t, queue.Len(), 1)
			key, shutdown := queue.Get()
			assert.Assert(t, !shutdown)
			assert.Equal(t, key, types.NamespacedName{Namespace: pr.Namespace, Name: pr.Name})
			queue.Done(key)
		}

		// Drain both informer events without reconciling. The first event alone
		// must schedule the 45-minute tick even when another event arrives earlier.
		const interval = 45 * time.Minute
		const patchAfter = 5 * time.Minute
		handle := checkStateAndEnqueue(impl, run.Clients.Log)
		handle(pr)
		drain()
		time.Sleep(patchAfter)
		handle(pr)
		drain()
		for _, untilTick := range []time.Duration{interval - patchAfter, interval, interval} {
			time.Sleep(untilTick - time.Nanosecond)
			synctest.Wait()
			assert.Equal(t, queue.Len(), 0)
			time.Sleep(time.Nanosecond)
			drain()
			assert.NilError(t, r.ReconcileKind(ctx, pr))
		}

		// A tick scheduled while active can still arrive after completion.
		pr.Annotations[keys.State] = kubeinteraction.StateCompleted
		pr.Status.SetCondition(&apis.Condition{Type: apis.ConditionSucceeded, Status: corev1.ConditionTrue})
		pr, err := runs.Update(ctx, pr, metav1.UpdateOptions{})
		assert.NilError(t, err)
		time.Sleep(interval)
		drain()
		assert.NilError(t, r.ReconcileKind(ctx, pr))
		time.Sleep(interval)
		synctest.Wait()
		assert.Equal(t, queue.Len(), 0)
		for _, action := range client.Actions() {
			assert.Assert(t, !action.Matches("patch", "pipelineruns"))
		}
	})
}
