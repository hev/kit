# Capture and native host roles

`hev daemon` runs capture by default. `HEV_NATIVE_HOST_CONFIG` additionally selects a verified native-host profile in the same managed daemon. In the default `HEV_CAPTURE_ROLE=required` mode, either role exiting stops both roles. Existing unconfigured behavior is unchanged.

`HEV_CAPTURE_ROLE=held` is an explicit capability selection. It requires a verified native-host profile and the platform's specific unsupported trace-write capability. It is rejected on platforms that support capture. Configuration, credentials, network errors, and errors that merely contain similar text cannot select this mode.

Held mode never starts capture or opens its configuration, log or status files. It writes one `daemon-role/v1` record to the managed service's stderr with `capture: held`, `reason: unsupported_trace_writes`, `native: starting`, and the verified profile digests. This record is not health, readiness, capacity, registration or launch authority. Native-host capability and runtime errors remain fatal; unsupported platform guards remain effective.

The OS service adapter must retain stderr durably and expose held capture as a capability gap. This option does not install a service, change the selected service account/session, or provide a standalone native-host daemon. Platform acceptance of filesystem security, transport and owned process lifecycle is required separately.
