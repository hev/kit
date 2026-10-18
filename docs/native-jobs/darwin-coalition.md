# Darwin coalition observation

`hev native-host observe-coalition` is a fixed read-only metadata operation. It
accepts one strict bounded JSON object on stdin:

```json
{"version":"native-host-coalition/v1","host_birth":{"boot_id":"BOOT-UUID","pid":123,"start":"SECONDS:MICROSECONDS"},"resource_cid":0}
```

The caller supplies an independently established original live process birth.
Zero CID permits initial observation only. Terminal producers must supply the
recorded nonzero resource CID and require the same original daemon birth.
Output contains only version, host_birth, resource_cid, tasks_started,
tasks_exited, os_build and observed_at. There is no readiness, permission,
capacity, treeGone or enumerated service-members assertion.

The adapter uses the private resource coalition ABI behind BSD proc_info336 and
coalition_info459. It is restricted to actual-tested Darwin arm64 kernel build
25G83; other builds/platforms fail held. The first16 bytes of resource usage
contain tasks_started/tasks_exited, copied under one coalition lock in inspected
Apple XNU source f6217f891ac0bb64f3d375211650a4c1ff8ca1ea. This inspected source is
not asserted to be the exact installed kernel source. Changes to the build
allowlist require native validation of this boundary.

Birth and coalition are rechecked around the sample. Missing/reused birth,
different coalition, syscall failure, invalid counters or unsupported kernel
fails held. The API does not signal, adopt or start any process, access
credentials/database, mutate journals or enable native-host support.

A count of one alone is insufficient: an escaped child can outlive its daemon,
and launchd can reuse the same coalition across a new daemon birth. An external
terminal producer must bind the count to the exact original live host,
serialized admission, managed executable/profile/unit identity and original
terminal evidence. Restart or unknown lineage remains Uncertain and charged.
Delegated launchd/XPC effects outside the owned resource lineage are not proved
absent by this sample. Native platform role admission requires the independently
reviewed managed-service composition and end-to-end proof.
