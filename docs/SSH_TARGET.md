# SSH target hardening

Use a dedicated, unprivileged `weazlback` account whose SFTP-visible directory is
limited to the repository root. Disable password login after setup, pin the host
key in Weazlback, and permit only the vaulted key. The account needs create,
read, rename, and delete access inside its repository: restic maintenance and the
break-glass **Nuke repository** action cannot work through a truly append-only
SFTP policy.

For stronger ransomware isolation, place provider snapshots or an append-only
copy behind a second credential that the client does not possess. That copy also
cannot be deleted by Weazlback; destroy it separately through the server/provider
control plane after using Nuke. Ciphertext may survive snapshots and SSD
remapping, so the promise is cryptographic destruction, not secure erase.

Rotate repository encryption with:

```sh
weazlback rotate repository-key
weazlback recovery prepare --target /mnt/WEAZLBACK-RECOVERY
```

The replacement key is added to the repository and vaulted before the old key is
removed. Refresh every offline recovery kit immediately after rotation.
