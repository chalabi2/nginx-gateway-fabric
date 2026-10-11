# Akash gateway recovery candidate

This fork packages recovery fixes on NGINX Gateway Fabric 2.7.2. It is an optional
candidate maintained under `chalabi2`, not an official NGINX or Akash release.
Install both images together. Provider v0.16.5-rc2 remains a separate component.

## Changes

- Backport [NGF #6039](https://github.com/nginx/nginx-gateway-fabric/pull/6039):
  retry failed configuration applies with backoff. Local corrections also retry
  failures on initial Agent connections, avoid recording an apply when no Agent
  received it, and preserve failure status across graph rebuilds and queued updates.
- Backport [NGF #5968](https://github.com/nginx/nginx-gateway-fabric/pull/5968):
  remember EndpointSlice ownership so deletion rebuilds the affected upstream.
- Upgrade NGINX Agent from 3.12.0 to 3.12.1, which contains
  [Agent #1947](https://github.com/nginx/agent/pull/1947). Nonregular files such as
  FIFOs return an error instead of blocking configuration processing. This does
  not add a general timeout for every stalled Agent acknowledgement.

Both runtime base images are pinned to the upstream 2.7.2 manifest digests.
The controller binary retains runtime version 2.7.2; the OCI image tag and revision
identify the fork. NGINX modules, CRDs, and the Helm chart are unchanged.

## Images

Candidate tag: `2.7.2-akash.1-rc1`.

```text
ghcr.io/chalabi2/nginx-gateway-fabric:2.7.2-akash.1-rc1
ghcr.io/chalabi2/nginx-gateway-fabric/nginx:2.7.2-akash.1-rc1
```

The [Akash gateway images workflow](../../.github/workflows/akash-images.yml)
builds Linux amd64 and arm64 images. Its `akash-gateway-images` artifact records
immutable digests. Use those digests when promoting a tested candidate.
Only signed `v2.7.2-akash.*` candidate tags should be published; increment the
candidate tag for changes rather than replacing an existing tag.

## Use with an existing NGF 2.7.2 Helm installation

The [values overlay](../../deploy/akash/values.yaml) changes only the two images.
Retain the provider's existing GatewayClass, Gateway, networking, TLS, snippets,
service, and scheduling settings. Akash's SnippetsFilters require snippets to
already be enabled. This is not a migration guide from older NGF versions.

Set the intended Kubernetes context explicitly. Adjust the release and namespace
if this provider uses different names.

```bash
export KUBE_CONTEXT=your-provider-context
export NGF_RELEASE=nginx-gateway
export NGF_NAMESPACE=nginx-gateway

helm --kube-context "$KUBE_CONTEXT" get values "$NGF_RELEASE" \
  -n "$NGF_NAMESPACE" -o yaml > ngf-values-before.yaml
helm --kube-context "$KUBE_CONTEXT" history "$NGF_RELEASE" -n "$NGF_NAMESPACE"

helm --kube-context "$KUBE_CONTEXT" upgrade "$NGF_RELEASE" \
  oci://ghcr.io/nginx/charts/nginx-gateway-fabric \
  --version 2.7.2 -n "$NGF_NAMESPACE" --reuse-values \
  -f deploy/akash/values.yaml --wait --timeout 5m
```

Record the previous successful Helm revision for rollback. A Gateway can reference
its own NginxProxy, overriding the chart's image. Inspect those references and
update the image in the owning NginxProxy when needed. The daemonSet form is:

```yaml
spec:
  kubernetes:
    daemonSet:
      container:
        image:
          repository: ghcr.io/chalabi2/nginx-gateway-fabric/nginx
          tag: 2.7.2-akash.1-rc1
          pullPolicy: IfNotPresent
```

For a deployment, use `spec.kubernetes.deployment.container.image` instead.
For GitOps or manifest installations, set the controller Deployment image and
this NginxProxy image in the managed manifests. Editing the generated NGINX
Deployment or DaemonSet directly will be overwritten by NGF reconciliation.

Changing the dataplane image replaces NGINX pods. A provider with a single
serving dataplane pod can have an interruption during that rollout.

## Verify and roll back

After the Helm controller rollout, also wait for the generated NGINX workload
rollout. For the Akash DaemonSet layout:

```bash
kubectl --context "$KUBE_CONTEXT" -n akash-gateway rollout status \
  daemonset/akash-gateway-nginx --timeout=5m
kubectl --context "$KUBE_CONTEXT" -n akash-gateway get pods \
  -o custom-columns=NAME:.metadata.name,IMAGE:.spec.containers[*].image,IMAGEID:.status.containerStatuses[*].imageID
kubectl --context "$KUBE_CONTEXT" -n akash-gateway exec \
  daemonset/akash-gateway-nginx -c nginx -- nginx-agent --version
kubectl --context "$KUBE_CONTEXT" -n akash-gateway get gateway akash-gateway -o yaml
```

Confirm the desired image has reached the generated workload before treating its
rollout as complete. Check Gateway `Programmed=True` and HTTPRoute `Accepted=True`
and `ResolvedRefs=True`, then request an existing and newly created hello-world
URI. Conditions and Ready pods alone do not establish working HTTP traffic.
Keep requests to an existing URI running during the test.

To roll back a Helm-managed image change:

```bash
helm --kube-context "$KUBE_CONTEXT" rollback "$NGF_RELEASE" \
  "$PREVIOUS_REVISION" -n "$NGF_NAMESPACE" --wait --timeout 5m
```

Also restore any Gateway-specific NginxProxy image override you changed and wait
for the generated dataplane rollout. This candidate does not change CRDs.

## Validation

The source changes passed race-enabled controller, Agent, state, and WAF poller
unit tests. Local integration used Kubernetes 1.32.5, Gateway API CRDs 1.5.1,
NGINX OSS, and the downloaded provider v0.16.5-rc2 on a local testnet.

| Reproduction | Upstream 2.7.2 | Candidate |
| --- | --- | --- |
| Failed configuration apply, then remove the fault | Original URI remained 404 for 90 seconds | Same URI recovered in 33 seconds without restart or manifest change |
| Delete one EndpointSlice | 30 of 60 requests reached its removed backend | 0 of 60 |
| FIFO in an included configuration file | Agent blocked and queued route status was absent | Agent reported an error and processed the queued route |

Six lease cases covered hello-world, custom hostnames and HTTP options, replica
updates, private backends, persistent data across pod replacement, and a new lease
during controller restart. Deleting SnippetsFilters also recovered the original
URIs. Existing hello-world traffic had 3,299 requests and zero failures.

The functional suite used a 5-second hostname-operator prune interval. The failed
apply comparison used default timers. Python fixtures used 0.25 CPU under amd64
emulation; an initial 0.1 CPU fixture run timed out and is not counted as passing.
The hello-world SDL retained its original 0.5 CPU allocation.

These are single-node local tests, not an HA, NGINX Plus, WAF, or production
qualification. They reproduce specific bugs but do not identify the cause of a
past provider incident. See [validation.json](validation.json) for the source
and test artifact hashes. The historical integration run covered amd64; arm64
images require their own deployment qualification.

## Build another candidate

Use a new signed tag on `akash/v2.7.2`, for example `v2.7.2-akash.1-rc2`, and update
the image overlay to match before tagging. Pushing the tag runs the fork's image
workflow with `GITHUB_TOKEN`; NGINX's internal vault credentials are not required.
Run the workflow on that tag to retry a failed build. Verify anonymous image pulls
before handing a candidate to providers; newly created GHCR packages may require
public visibility to be enabled in the package settings.
