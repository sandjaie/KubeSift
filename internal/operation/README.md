# operation

`operation` prepares, reviews, and executes mutations against an explicit target. The desktop adapter and the opt-in integration tests call it. Reviewed workload actions share one session coordinator for preparation slots, action-bound tokens, target locks, and terminal results. A token for the wrong action is not consumed and is not revealed. Transport goes through `http.go` on a guarded connection. Raw error bodies stay inside the package.

Scale keeps its existing response shape through compatibility methods. The Phase 1A marker experiment has its own scope and lifecycle, described below. Later actions reuse the same prepare, review, and single dispatch pattern.

## Workload actions

Scale compares the displayed parent and its Scale subresource, requires a server dry-run, and issues a bounded review. The only live update is a minimal Scale carrying the original UID and resourceVersion. A permission hint is advisory. Completion feedback stays in memory, with separate states for acceptance, rejection, and an uncertain outcome.

Restart checks the parent strategy and dry-runs a fixed Pod template annotation patch. The review retains hashes and safe evidence. It does not retain the parent manifest.

Delete reviews one supported workload, then sends a conditional `DeleteOptions` body with background propagation and the server grace period. A later check reads the original route with the captured UID and keeps that observation separate from the result of the delete request. Impact evidence is computed before the token is issued.

Rollback verifies retained ReplicaSet or ControllerRevision ownership and reads the captured workload. It dry-runs a conditional replacement of only the Pod template. Confirm checks the source and the parent again, then sends that same reviewed patch once.

Node reviews use the same coordinator and canonical Node lock. Drain inventories all namespaces, verifies owners and storage, and waits for each admitted Pod UID to disappear. A connection close or explicit cancellation stops the worker. Results retain scheduling state and bounded Pod progress.

## YAML actions

Edit captures one discovered object and reviews a conditional patch with a Kubernetes dry-run. The baseline and the patch stay in memory. They do not enter operation history. Drafts share the coordinator's limits and expiry. Retained edit results include the route, the identity, and the outcome.

Create prepares one named YAML resource through exact API discovery and a mandatory strict dry-run POST. The editor shows the normalized proposal and the server preview. Only the proposed bytes are retained with the review. Confirm sends that POST once. A dry-run identity, or another object that happens to have the same name, is not recorded as the created object. Name inspection after a request is explicit and does not adopt an observed UID.

Apply captures an existing identity, starts from identity-only desired YAML, and uses strict Kubernetes server-side apply. Force is available only when an earlier structured ownership conflict was reported for the same draft text. The coordinator rechecks UID and resourceVersion before the single dispatch. Different API versions are not compared as if they were the same document. Field paths and values are omitted from retained results. Response differences go back only to the editor that submitted the draft. A lost response or an unexpected creation is not replayed and is not rolled back.

## Marker experiment

The Phase 1A path prepares, reviews, and executes a write to the `ks-marker` ConfigMap. Opaque five-minute tokens bind connection generation, namespace, UID, resourceVersion, and marker value. At most 128 reviews are retained, and a marker value is at most 256 bytes. Execution consumes the token before any network work, revalidates identity through the guarded connection, and sends one update with UID and resourceVersion preconditions. An unknown write outcome requires inspection and a new review. The desktop adapter further restricts this path to its validated local fixture configuration.
