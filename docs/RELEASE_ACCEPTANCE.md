# Weazlback v1 release acceptance

Automated gates produce evidence, but they do not substitute for the physical
restore gate. Record the date, machine, repository, elapsed backup/restore time,
placed-path count, package exceptions, and final repository check for every run.

## Fresh SSH target

1. In **Destinations**, choose **Add destination → SSH**.
2. Enter the target hostname, an existing setup username, and its temporary
   password. Confirm the displayed host-key fingerprint out of band.
3. Let Weazlback create the restricted account/key and encrypted repository. The
   bootstrap password is not retained; the private key and repository key remain
   only in the local vault and exported recovery kits.
4. Run Core + Home, then Heavy while its VM/container writers are stopped.
5. Run **Check repository**, prepare the recovery USB again, and verify
   `SHA256SUMS` on another machine.
6. Confirm that an intentionally forged host key hard-stops before repository
   authentication; restore the correct target and repeat Check.

## Physical clean-ISO matrix

Complete three consecutive fresh Omarchy ISO restores from the local repository
and three consecutive restores from SSH. For each repository, exercise Core,
Home, and Everything at least once across the three runs. Each run must:

- start only with the recovery folder and vault passphrase;
- verify media checksums and the pinned SSH host key where applicable;
- restore the selected filesystem scope and application manifest concurrently;
- retain correct modes, ownership, symlinks, sparse files, and hostname choice;
- record unavailable packages/services as resumable exceptions instead of losing
  successfully restored files;
- reboot into a usable Omarchy session with Weazl apps, widgets, configuration,
  and scheduled backup timer available;
- finish with a successful repository check and no secret in argv, plaintext
  logs, status, or crash output.

Any unexplained exception, manual filesystem repair, host-key bypass, or missing
application resets the consecutive-success count for that repository.

## Publication hold

Do not publish or submit to the marketplace until the six physical runs pass,
the seven-day unattended timer soak finishes, and the release archive is signed
with the chosen project signing identity. Marketplace submission remains a
separate explicit action.
