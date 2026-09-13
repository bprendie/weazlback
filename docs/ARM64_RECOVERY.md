# Universal recovery and ARM package preparation

Weazlback builds Linux x64 and ARM64 from one source tree. A universal recovery
kit detects the destination CPU and selects its native Weazlback, restore, and
Restic executables. Snapdragon hardware qualification is still required; successful
cross-compilation is not a real-device acceptance result.

## Build and prepare the kit

```sh
./scripts/build-universal.sh
# Use the matching newly built binary to prepare existing mounted media:
./build/recovery-bundle/linux-amd64/weazlback recovery prepare \
  --binary-dir ./build/recovery-bundle --target /mnt/WEAZLBACK-RECOVERY
```

`./scripts/release.sh` packages both architectures and installation assets in a
`linux-universal` archive. Installing that archive installs the native pair and
retains the universal bundle for later kit preparation/refresh.

In the TUI, **k → Prepare USB / recovery folder** uses the matching installed
universal bundle and includes both architectures. Preparation stops if that bundle
is missing or fails validation. For a development build, set
`WEAZLBACK_BUNDLE_DIR` to the complete matching bundle directory.

Media stores immutable `payloads/kit-*` generations and an atomically replaced
`CURRENT` pointer. Both architectures, runtime assets, and the encrypted recovery
envelope are verified before a new generation becomes active. Failed refreshes
leave the previous generation available. Existing legacy kits remain readable.
Refresh universal media with a complete matched bundle, never just a native pair.

After booting the clean Snapdragon installation:

```sh
cd /mnt/WEAZLBACK-RECOVERY
./weazlback-turbo-restore
```

The launcher selects ARM64 and requests Turbo automatically. The normal guided
restore still reviews the source snapshot and destination before applying changes.
Missing application packages may download from the destination's repositories.
Btrfs fast landing remains a Turbo prerequisite; qualification/fallback is reported.

## Independent ARM package preparation

Profiles → **X: Other architectures → ARM64** opens the architecture package
manager. It offers target export, source inventory export, availability inspection,
preparation, explicit native source builds, verification, and import.

Normal System Snapshots only capture native packages. They never invoke foreign
package preparation. ARM capsules are optional accelerators, with a separate date
and source-inventory digest. They are retained by routine pruning; explicit manual
snapshot deletion can still remove them and is detected when resuming recovery.

1. Export the target environment on the Snapdragon installation:

   ```sh
   weazlback packages architectures export-target --output /path/snapdragon-target
   ```

   This exports effective repository configuration, a platform fingerprint, and
   public package-verification material. It does not export private signing keys.
   Transfer this directory to the preparation machine. Re-export it when the image
   or repository configuration changes. Treat it as trusted configuration from your
   target: repository entries may contain private URLs.

2. Export the source application inventory on x64:

   ```sh
   weazlback packages architectures export-source --output /path/source-apps.json
   ```

3. Inspect availability using an isolated ARM database and cache:

   ```sh
   weazlback packages architectures inspect \
     --target-dir /path/snapdragon-target --source /path/source-apps.json
   ```

4. Prepare and store an encrypted ARM capsule:

   ```sh
   weazlback packages architectures prepare \
     --target-dir /path/snapdragon-target --source /path/source-apps.json \
     --machine SOURCE_MACHINE_ID
   ```

   `--snapshot CORE_SNAPSHOT_ID` (or `latest`) can replace `--source` to read the
   chosen source machine's application inventory directly from the repository.
   `--destination ID` selects the repository. New exports include the source machine ID; `--machine` overrides it for older inventories.
   The source machine ID binds the
   capsule to the correct backup history; do not substitute the builder's identity.

Downloads use isolated configuration, database, cache, and public keyring paths.
Pacman may request sudo, but preparation does not change the host repositories or
install ARM archives onto x64. Cached packages are reused, and dependency archives
are retained in the capsule.

For packages unavailable from target repositories, adding `--build-missing-aur`
explicitly executes AUR build recipes. This is allowed only on a native machine
matching the exported target environment. Inspect/review recipes first. Builds may
install build dependencies on that ARM machine. General cross-compilation is not
performed. Existing build directories are reused without silently updating recipes.

To prepare on ARM and import later on x64, add `--dry-run --output /path/arm-capsule`
to preparation, transfer the directory, and run:

```sh
weazlback packages architectures verify \
  --target-dir /path/snapdragon-target --input /path/arm-capsule
weazlback packages architectures import \
  --target-dir /path/snapdragon-target --input /path/arm-capsule
```

## Migration behavior

- CPU architecture and repository/platform compatibility are checked separately.
- Recovery prefers matching target capsules, discloses inventory/date differences,
  and downloads missing native repository packages. Obsolete official artifacts
  are bypassed when their version differs from the target repository version.
- Existing target packages are held in place. A transaction requiring a baseline
  upgrade is reported for review; update the custom target image deliberately and
  create a fresh restore plan when necessary.
- Hardware/session configuration and native runtimes are retained from the target.
  Other portable settings and user data can restore. Foreign ELF binaries and
  native dependency environments are withheld and recorded for review/rebuilding.
- Weazlback's native runtime is restored from the verified kit and registered after
  filesystem placement. Source x64 binaries cannot overwrite the ARM installation.
- Source services are not blindly enabled on the new hardware; missing services
  are listed for review. VM/container images restore as data without a promise of
  native execution. Unavailable apps and failed native build artifacts never trigger
  an automatic AUR build during recovery.
- Resume pins the target fingerprint, source inventory, package versions, and
  capsule snapshot. Changed plans or removed pinned capsules require a new workspace.
  Failed package operations can retry without repeating completed file placement.

Use the final exception report to distinguish recovered data from applications or
services still needing attention. A real Snapdragon clean-install/Turbo/reboot drill
is the remaining release qualification step.
