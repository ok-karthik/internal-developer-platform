# 4. Platform APIs

CRDs the custom APIs tenants call. Phase 9 restored the earlier Crossplane experiment
(`git log -- archived/crossplane/`, archived then later deleted) and modernized it to
Crossplane v2 rather than leaving it in a directory named `archived`.

`awsbucket-xrd.yaml` + `awsbucket-composition.yaml` define `XAWSBucket` — a namespaced
Composite Resource (v2's `scope: Namespaced`, no separate Claim kind, unlike the archived
v1 shape) that fans out into two composed managed resources (an S3 `Bucket` and its
`BucketServerSideEncryptionConfiguration`) via a `function-patch-and-transform` pipeline.
Installed by `4-platform-engineering/2-cluster-services/composition/crossplane.yaml`, which
also installs Crossplane core, the provider, and the pipeline function.

**Why this exists alongside ACK's S3 support (Phase 3.2), not instead of it:** ACK's
`Bucket` CR is the **provider layer** — one CR, one AWS resource, no composition. This XRD
is the **composition layer** — one CR fanning out into several. `kro` composes too but
needs ACK underneath for anything AWS-shaped (it has no AWS provider of its own), which
keeps a kro-based platform AWS-locked; Crossplane has first-party providers for AWS, Azure
and GCP, which is the concrete reason it — not kro — is the correct Phase 9 pick if Phase
10 portability is ever pursued for real. See `PLAN.md` Phase 9 and Phase 10.1 for the full
argument; this directory is where that argument stops being prose.

**A tenant creates a bucket by applying an `XAWSBucket` directly in their own namespace**
— no claim indirection, matching every other per-namespace resource in this platform:

```yaml
apiVersion: platform.io/v1alpha1
kind: XAWSBucket
metadata:
  name: my-first-cloud-bucket
  namespace: team-a
spec:
  parameters:
    bucketName: team-a-my-idp-test-bucket
    region: eu-central-1
    isEncrypted: true
```

`XAWSBucket` is not yet in `team-a`'s `AppProject` `namespaceResourceWhitelist` — adding it
is a one-line YAML change in `1-platform-catalog/blueprints/team/gitops/platform/team/appproject.yaml.tmpl`
the moment this capability is offered for real, following the same pattern as every other
kind in that file.

**Only add the Component Matrix row back once this is actually offered as a golden-path
capability** (i.e. `catalog.yaml` gains a `provisioner: crossplane` entry, which — like
`provisioner: ack` before it — needs scaffolder support that is out of scope on this
branch). Until then this is a demonstrated capability, not a claimed one; see
`docs/incidents/` and this repo's general rule against Component Matrix rows for things
that are not wired end to end.
