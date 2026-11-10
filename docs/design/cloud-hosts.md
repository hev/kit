# Cloud hosts

Reviewed against public documentation on 2026-11-09. Boat, executable scripts and the AWS adapter are implemented. AWS live
validation remains pending permissions. Boot times and adapter sizes below are engineering
estimates, not measurements. Prices exclude disks, network, tax, licences and
minimum plan fees unless specified; obtain a regional quote before deployment.

## Lifecycle and transport

A cloud host is a pool with one machine per order. The dispatcher serializes
create/resume admissions, counts provider-labelled machines, persists a local
FIFO when the pool or class is full, and starts the FIFO head when capacity
returns. Each receiver runs the normal dispatch supervisor. Records are fetched
before deletion; notification happens after fetch, with a digest marker making
retries safe. Provider auto-stop bounds costs if the dispatcher disappears.
Independent dispatchers must use distinct host labels: admission locks are local.

CloudProvider in internal/dispatch/provider.go defines create, wait-ready,
transport, execute, copy-out, stop, resume, delete and list-by-host-label.
MachineSpec includes image/template, size, region, labels and auto-stop.
Credentials are resolved at use time from environment names in config; records
and queued requests carry no resolved credentials.

Two transport kinds:

- **ssh**: provision a machine, authorize a public key and return an SSH address
  and authenticated host key. Use the normal Unix dispatch RPC over pinned SSH.
  Refresh address and key on resume. Boat and command hosts use this path.
  The existing Windows receiver also supports SSH; Windows cloud bootstrapping
  is future adapter work (the current cloud bootstrap expects Ubuntu).
- **exec**: a future API transport implements commands and copy-out without SSH.
  ECS/EKS tasks and Cloudflare can implement this contract. No new provider for
  these services is implemented. The port retains Boat's old HTTP execution
  methods for regression coverage and runtime environment bootstrap. Normal
  dispatch uses SSH; the accepting RPC after a cold resume uses the injected
  API environment before runtime files are rebuilt.

Templates contain hev, the harness, git, tmux, Python and runtime credentials
integration. Command templates currently use Boat's /home/user layout and
arrange capture startup themselves. Reads never resume a stopped VM. Rework
deliberately resumes, checks pool/class capacity, and replaces the watcher token.

## Provider matrix

Line estimates cover provider-specific production code beyond the shared
lifecycle, excluding tests, SDKs, image building and infrastructure.

| Cloud | Transport and auth | Boot estimate; image story | Labels; Windows | Compute ballpark; adapter estimate | Main risks |
| --- | --- | --- | --- | --- | --- |
| Boat | SSH; API bearer and public key from named env, SSH agent | Seconds to tens of seconds; saved disk snapshots, resume/fork | Native labels; Ubuntu only, Wine is not a Windows receiver | Small 2-vCPU/4-GB $0.018/h, per-second billing plus plan; 200–350 lines | Plan/start quotas, snapshot permissions, key/IP changes on resume |
| AWS EC2 + Nitro Sandboxes | SSH to guest; IAM temporary credentials on parent; attestation-gated guest secrets | No measured boot time; warm parent, prepared guest image/rootfs | EC2 tags plus guest inventory; Nitro Windows guest support not established by launch docs, ordinary EC2 Windows separate | Parent EC2 billed normally, no extra Nitro Sandbox fee; 400–800 lines plus parent setup | Parent pool, guest networking, VMM/kernel compatibility, attestation policy, idle parent cost |
| AWS ECS/EKS + Nitro Sandboxes | exec; IAM task/pod identity, API command/file bridge needed | Seconds to minutes with scheduling/image pull; OCI images, EBS/EFS/S3 | Task tags / pod labels; do not assume Windows Nitro guests | Underlying compute/storage and relevant ECS/EKS charges; no extra Nitro Sandbox fee; 400–700 lines | Streaming/file bridge, shutdown integration, control-plane complexity |
| Hetzner Cloud | SSH; scoped project token from named env | Tens of seconds; images, snapshots, cloud-init | Native label selectors; Windows installation/licensing custom, not standard image path | EU CX23 about €0.0088/h in June 2026 table, IPv4 extra; 150–300 lines | Powered-off servers remain billable, delete to end compute billing; capacity, host-key provenance, no native per-run TTL |
| GCP Compute Engine | SSH; short-lived OAuth/workload identity, OS Login or provisioned key | Tens of seconds to minutes; custom images, instance templates, disk snapshots | Native labels; Windows Server images make kit's Windows receiver useful | E2-standard-2 about $0.067/h in published base-region table; 250–450 lines | IAM/OS Login, zone quota, operation polling, disks/IP charges after stop, Windows licences |
| Azure VMs | SSH; Entra service principal/federated token from named env | Usually minutes; Compute Gallery, managed images, disk snapshots | Resource tags; first-class Windows Server, useful for kit's Windows receiver | Budget $0.04–0.12/h for small Linux VM, regional quote required; 300–500 lines | Deallocate versus stop billing, async operations, disks/NICs/IPs, Windows licences |
| Cloudflare Sandbox / Containers | exec; scoped API token and Worker binding | Warm starts seconds; OCI image, explicit persistent storage | Application-owned IDs/metadata rather than VM selectors; Linux containers, no Windows receiver | CPU/memory/disk meters, budget cents per small active container-hour depending on use; 200–400 lines | API bridge, sleep/eviction, ephemeral files, image/network limits, no SSH/systemd assumption |

Boat sources: [SSH and authenticated host keys](https://docs.boat.dev/ssh-access),
[pricing](https://docs.boat.dev/pricing), [snapshots](https://docs.boat.dev/snapshots),
[limits](https://docs.boat.dev/limits).
Its [documentation index](https://docs.boat.dev/llms.txt) links current API endpoints.

AWS sources: [9 November 2026 announcement](https://aws.amazon.com/about-aws/whats-new/2026/11/nitro-sandboxes-eks-ecs-ec2/)
and [EKS implementation discussion](https://aws.amazon.com/blogs/containers/aws-nitro-sandboxes-hardware-enforced-workload-isolation-on-amazon-eks/).
The announcement describes lightweight Nitro Hypervisor VMs on customers' EC2
instances, attestation-based release of secrets, native Amazon Linux and
Firecracker support, and no additional Nitro Sandbox charge. Pods/tasks retain
their identity, networking and storage integrations.

Hetzner sources: [API and labels](https://docs.hetzner.cloud/),
[official price table](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/),
[billing FAQ](https://docs.hetzner.com/cloud/billing/faq/).

GCP sources: [instances/images](https://cloud.google.com/compute/docs/instances),
[pricing](https://cloud.google.com/products/compute/pricing/general-purpose),
[Windows images](https://cloud.google.com/compute/docs/instances/windows).

Azure sources: [VM overview](https://learn.microsoft.com/en-us/azure/virtual-machines/overview),
[Linux pricing](https://azure.microsoft.com/en-us/pricing/details/virtual-machines/linux/),
[states and billing](https://learn.microsoft.com/en-us/azure/virtual-machines/states-billing).

Cloudflare sources: [Sandbox SDK](https://developers.cloudflare.com/sandbox/),
[pricing](https://developers.cloudflare.com/containers/platform/pricing/),
[GA announcement](https://developers.cloudflare.com/changelog/post/2026-04-13-containers-sandbox-ga/).

## Nitro recommendation

Start with EC2 + Firecracker: keep a warm kit parent host, allocate one isolated
guest per run, return guest SSH transport, and reuse supervisor, fetch-back
and cleanup. The adapter follows the EC2 user guide; live acceptance remains pending. It avoids building an exec/file bridge first and permits control
of guest images and attestation policy. The parent remains trusted infrastructure;
guest isolation does not secure a compromised parent control plane. Never bake
credentials into guest images.

Choose ECS/EKS when the operator already owns that platform and wants its
scheduler, IAM identity and storage integration. AWS supports these integrations
with a small deployment change; kit still needs API execution, file transport
and recovery. Prefer provider-managed tasks there to adding an SSH daemon.

## Recommended build order

1. Boat live acceptance; command-backed Hetzner pilot to exercise the seam.
2. Native Hetzner, including safe TTL/delete reconciliation.
3. Azure for Windows demand; GCP for identity and regional capacity.
4. EC2 + Firecracker Nitro prototype with explicit parent/guest inventory.
5. Cloudflare exec transport; ECS/EKS after that transport is proven, or sooner
   when an operator already owns a cluster and command/file bridge.

AWS now implements instance lifecycle and a parent-local Nitro guest controller.
Neither AWS mode has live acceptance yet.

## EC2 guide findings (2026-11-09)

[The current EC2 guide](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/amazon-ec2-nitro-sandboxes.html)
uses EnclaveOptions.Enabled at parent launch and Firecracker's `enclave` JSON
object for guests. Guest lifecycle has no separate EC2 Sandbox API. An empty
object selects confidential mode; direct_mode shares memory with the parent.
The guest driver model is virtio-pci, rather than ordinary KVM assumptions.
Supported types are those supporting Nitro Enclaves; consult the guide's exact
family/size table before selecting a parent. For example m5.xlarge is supported,
m5.large and m5.metal are excluded. No GPU passthrough is available.
Memory is fully donated, never overcommitted; vCPU/memory sizing is immutable
after launch. CPU overcommit is opt-in and the aggregate vCPU limit is five
times the parent count. Devices consume a parent-side slot budget. The guide
does not establish a measured guest boot latency; the earlier subsecond estimate
is not acceptance evidence. Parent billing continues while it is idle.
