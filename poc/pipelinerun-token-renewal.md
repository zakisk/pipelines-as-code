# Keeping a GitHub token valid while a PipelineRun waits

A private-repository PipelineRun can spend more than an hour waiting for a PaC concurrency slot or a cluster worker. Its first clone then reads an expired GitHub App installation token. This proof of concept exercises the scheduling part of a possible fix in the PaC watcher. It revisits active PipelineRuns every **45 minutes**. **Secret expiry tracking and token renewal are proposed work.** This explores implementing renewal inside PaC.

## From a GitHub event to the clone credential

On a GitHub push or pull request, PaC matches the Repository and PipelineRun templates. With git-auth Secret creation enabled, PaC stores a scoped GitHub App installation token in each run’s Secret before creating the PipelineRun. The clone task reads that credential when it starts, so time spent queued counts against the token’s lifetime.

## How the 45-minute requeue works

The watcher receives PipelineRun add, update and delete events. In `checkStateAndEnqueue`, it checks for PaC's existing state annotation and enqueues the run immediately for normal processing. For an active PipelineRun it also calls Knative's `EnqueueAfter(pr, 45 * time.Minute)`. PaC chooses the delay. Users do not need to annotate their templates to enable or configure it. [Controller scheduling](../pkg/reconciler/controller.go)

`EnqueueAfter` schedules a single check. If a PipelineRun stays queued for 90 minutes without any updates, the controller schedules the first check at creation, and `ReconcileKind` schedules the 90-minute check when it handles the 45-minute check. Each check reads the same PipelineRun. If it has finished, has a stop or cancellation request, or is being deleted, PaC skips scheduling the next check. [Reconciler](../pkg/reconciler/reconciler.go)

### Do annotation changes reset the 45-minute timer?

**No.** Suppose PaC creates a PipelineRun at **10:00** and schedules a check for **10:45**. At **10:05**, Tekton Results adds an annotation. PaC queues the updated run for processing immediately and asks Knative to check it again in 45 minutes, at **10:50**. Knative already has an earlier check for that same PipelineRun, so it keeps **10:45**. [Delayed queue implementation](../vendor/k8s.io/client-go/util/workqueue/delaying_queue.go)

Even if Results and Chains patch the run five times before 10:45, PaC can process those changes as they arrive and still have **one delayed check due at 10:45**. Each `Requested delayed PipelineRun requeue` log entry records a request. In the proposed renewal flow, PaC reads the Secret’s expiry on each eligible reconcile and mints a new token only when renewal is due. [Scheduling test](../pkg/reconciler/periodic_queue_test.go)

## Proposed 45-minute renewal

GitHub installation tokens expire after **one hour**. The proposed renewal point is **15 minutes before GitHub's returned expiry**, normally 45 minutes after issuance. Using the actual expiry matters because PaC can mint the token before creating the Secret, and the watcher may restart partway through its lifetime. [GitHub installation token lifecycle](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)

1. **Record expiry at creation.** Store GitHub's expiry alongside the exact scoped token in the git-auth Secret. Use a PaC-owned annotation such as `pipelinesascode.tekton.dev/github-token-expires-at` with an RFC 3339 UTC timestamp. Include the same timestamp when creating the PipelineRun so operators can see when its credential expires. The Secret is authoritative because its token and expiry are written together. Today's `GetAppToken` returns only the token string. The underlying transport exposes the expiry via `Expiry()`. [Token acquisition](../pkg/provider/github/parse_payload.go) · [Transport expiry](../vendor/github.com/bradleyfalzon/ghinstallation/v2/transport.go)
2. **Check eligibility first.** Update credentials only for a **pending** or **in-progress** GitHub App run with a PaC-generated git-auth Secret. These correspond to PaC's `queued` and `started` states. Check the live run for cancellation, graceful cancellation or stop, deletion, success and failure before acting. A GitHub status or stale `started` annotation alone cannot establish that the run is still active. Verify that the Secret belongs to this run's UID. Leave user-managed credentials alone. [PaC states](../pkg/kubeinteraction/labels.go) · [Tekton stop conditions](../pkg/queue/admission.go)
3. **Check whether renewal is due.** Read the Secret's expiry and compute `renewAt = expiresAt - 15m`. Before that deadline, schedule the remaining delay. If the PipelineRun already shows the same expiry, this path makes no writes. If its annotation is stale, repair it as described in step 5. A Results patch five minutes after issuance leaves about 40 minutes until renewal. If the watcher restarts after the deadline, an eligible reconcile should attempt renewal immediately.
4. **Replace the credential in place.** When due, request a fresh token for the same GitHub installation, authorized repositories and permissions. Update the token-bearing fields (`.git-credentials` and `git-provider-token`) and the new expiry in one Kubernetes Secret update. Keep the Secret name and ownership so existing workspace references still work. Use the Secret's resource version for the write. On conflict, reread and recheck eligibility and expiry. On failure, retry without advancing the stored deadline. On success, schedule from the replacement token's expiry.

5. **Mirror the committed expiry on the PipelineRun.** If the active run's annotation differs from the Secret, patch only PaC's expiry annotation, conditional on the PipelineRun's current `resourceVersion`. If the patch returns a conflict, reread the run and Secret, recheck eligibility and ownership, and retry with bounded backoff. A successful Secret update followed by a failed PipelineRun patch leaves a valid token in place. The retry copies its expiry without minting another token.

For example, a token issued at **10:00** expires at **11:00**. If the run is still queued at **10:45**, the watcher replaces it and records the new expiry, normally **11:45**. A clone starting at **11:10** reads the replacement. If the run was cancelled at **10:44**, the 10:45 wake-up skips renewal. A task that already copied the old credential into an environment variable or file keeps that value until it explicitly reads fresh credentials.

### Write contention at Kueue admission

On a busy OpenShift Pipelines cluster, Kueue admission can coincide with Tekton status updates and annotation patches from controllers such as Results and Chains. Each change advances the PipelineRun's `resourceVersion` and can wake PaC. Suppose PaC reads the run, Results patches an annotation, then PaC submits its expiry patch using the version it read. Kubernetes can return HTTP **409 Conflict** with the message `the object has been modified; please apply your changes to the latest version and try again`. The stale version precondition causes this conflict even when the controllers change different annotations. Independent patches to distinct keys can otherwise both succeed. [Kubernetes API update semantics](https://kubernetes.io/docs/reference/using-api/api-concepts/#updates-to-existing-resources) · [API server error text](https://github.com/kubernetes/apiserver/blob/master/pkg/registry/generic/registry/store.go)

Renewing on every ordinary reconcile would add a GitHub token request, a Secret write and a PipelineRun patch during that burst. PaC's own expiry patch could wake it again. Checking the deadline on each reconcile keeps the common admission path read-only and still renews a quiet run. For a token issued at **10:00**, admission at **10:05** can finish well before the **10:45** renewal. Admission at **10:45** can overlap it. A timer reserves no quiet window, and a busy worker may process the key late. The expiry check limits how often PaC writes. The narrow patch and conflict retry handle concurrent writers whenever renewal actually runs.

## Verify the scheduling PoC

From the repository root, run the virtual-clock test. It exercises the actual Knative queue with a fake Tekton client, injects an informer event at minute 5, checks ticks at minutes 45, 90 and 135, and verifies that completion stops recurrence. It exercises scheduling only.

```sh
go test ./pkg/reconciler -run '^TestPeriodicRequeueCadence$' -count=1 -v
```
