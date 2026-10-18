# Fixed receiver RPC inside hevd

The first cohort prepares and enrolls original jobs locally with the maintained
Factory preparation path. This RPC does not prepare jobs, upload files or accept
programs, argv, environment or descriptor paths. The existing original receiver
validates start/inspect/ACK, issuance, enrollment and final DB/native admission.

A native-host/v1 config may opt into `remote`:

```json
{"listen":"100.64.0.1:19761","allowed_node_id":"STABLE_NODE_ID","allowed_user_id":"DECIMAL_USER_ID","tailscale":{"path":"ABS_RESOLVED_EXECUTABLE","digest":"sha256:HEX"},"runtime":{"directory":"ABS_PRIVATE_DIRECTORY","environment":["HOME=ABS_HOME","PATH=PINNED_PATH"],"files":[]}}
```

Supply actual checked runtime file pins; the example is not executable authority.
The listener binds only the configured Tailnet address on port 19761. Each request
runs fixed pinned `tailscale whois --json REMOTE_ADDR` under its explicit runtime,
matching Node.StableID and UserProfile.ID. The IDs authenticate an existing trusted
node/user, not health, capacity or launch permission. A missing/changed identity,
lookup failure or runtime pin failure holds the request.

POST `/native-receiver/v1` accepts only the bounded envelope
`{version:"native-receiver-rpc/v1",profile_id,config_digest,call}`. `call` is the
unchanged native-receiver/v1 start/inspect/ack object. The profile is selected
locally and reverified. The only executed command is the pinned profile supervisor
with fixed `--config PROFILE_CONFIG`; stdin is the original call. Receiver stdout
is returned unchanged. Stderr is discarded. No endpoint activation occurs during
static config verification.

`hev native-host receiver-rpc --config ABS_CLIENT_CONFIG` reads one original call
from stdin. Client config is strict private JSON:
`{version:"native-receiver-rpc-client/v1",address,server_node_id,server_user_id,profile_id,config_digest,tailscale,runtime}`.
The final two fields use the same FilePin/Runtime types as `remote`. The client
checks the actual connected server address with the same pinned WhoIs operation
before sending the envelope. It uses no ambient HTTP proxy or redirects.

Input, envelope and output are at most 64 KiB, with at most 16 concurrent server
requests, bounded header/body time and a 45 second total call. Any lost reply,
nonzero receiver exit, timeout or malformed transport response is unknown: inspect
the original, never blindly retry start or ACK. The daemon owns both its local
endpoint and remote listener lifetimes. Restart is not completion or replay
permission. Tailnet authentication does not replace the original fences, SQL
claim/binding, business/node/parent checks, postcommit Ready or original ACK.

Source checks do not establish network reachability, managed user-unit lifecycle,
model runtime closure or the final three-host coding cohort. Those remain exact
managed acceptance obligations.

Managed user tasks without inherited stdout/stderr redirection can use
`hev daemon --native-host-config ABS --capture-role held --log-root ABS_PRIVATE_DIR`
(on capture-capable systems the role remains `required`). The log root must already
be private and the native configuration must verify before a new exclusive log
is created. Each process gets a new checked private file; old logs are never
repaired, truncated or replaced. Capture role capability checks remain unchanged.

The controlled Windows candidate is limited to the native-proved amd64 kernel
10.0.26300. Its journal uses production Open and every existing protected DACL,
NTFS, no-reparse, durability and owned-workload check. Hosting additionally
requires a nonzero current user token session; Session0 remains held before
endpoint effects. This capability permits the selected managed-unit test, not
broad platform readiness. Default daemon behavior without an opted-in native
profile is unchanged. The prepared caller and owned-I/O facade must be composed
before controlled activation; unknown tree/lifetime outcomes remain charged.
