# AWS policy sources

The adjacent policy is a us-east-1 starting policy for the instance and parent
lifecycle. Restrict subnet, security group and key-pair ARNs to your deployment
before production. The managed tag is `kit-managed=true`; dispatcher inventory
additionally uses `hev.dispatch=<host>`.

[EC2 Nitro Sandboxes user guide](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/amazon-ec2-nitro-sandboxes.html)
requires `RunInstances` with `EnclaveOptions.Enabled=true`. Guest creation is a
parent-local Firecracker operation using the Nitro Enclaves driver, not a separate
EC2 IAM API. The guide names no Sandbox IAM actions. Do not invent them. Attestation
and KMS release policies are outside this policy and implementation.

[EC2 service authorization reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_amazonec2.html)
defines action resource types and request/resource tag conditions. Read APIs use
wildcard resources; launch dependencies cannot all carry request tags. Instance
and volume creation require the managed request tag; lifecycle mutations require
the managed resource tag. Tagging is restricted to creation.

[Amazon Linux public parameters](https://docs.aws.amazon.com/systems-manager/latest/userguide/parameter-store-public-parameters-ami.html)
provides the AMI alias permission. No IAM roles, PassRole, secrets, KMS, S3 or
account-wide untagged termination permissions are granted.
