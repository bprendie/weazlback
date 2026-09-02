# WeazlBack: Sovereign Backup

### Real Encryption, Full Control, Zero Cloud Bros

```text
 __      __                   .__ ___.                  __
/  \    /  \ ____ _____  ____ |  |\_ |__ _____    ____ |  | __
\   \/\/   // __ \\__  \ \___ \|  | | __ \\__  \ _/ ___\|  |/ /
 \        /\  ___/ / __ \_/    >  |_| \_\ \/ __ \\  \___|    <
  \__/\  /  \___  >____  /_____ \____/___  (____  /\___  >__|_ \
       \/       \/     \/      \/        \/     \/     \/     \/
SIGNAL // IMMUTABLE // BARE METAL

```

Cloud backup is just some cloud bro charging you rent to scan your files. Weazlback is the exploit. It’s a gnarly, sovereign disaster-recovery TUI built strictly for Omarchy Quattro.

We’re cutting the cord entirely. You get encrypted local and SFTP repos powered by Restic, a passphrase-encrypted local vault, and live Bubble Tea progress meters grinding in the terminal. No Electron bloat. No telemetry harshing the mellow. Just content-addressed Restic Restore Points—not block-device snapshots—locked under paranoid local encryption, complete with selective file recovery, versioned app manifests, and password-locked recovery kits.

## Install

Run the script to stage the binaries, the Omarchy widget, and the user services. It doesn't babysit your vault. Start `weazlback`, forge the sovereign vault, and map your targets from **Destinations**.

```sh
./scripts/install.sh
```

## TUI-First Mechanics

GUIs are a crutch for tourists. Weazlback is terminal-native and keyboard-driven down to the bare metal.

* **Navigation:** `Tab` shifts focus to the navigation rail; `Shift+Tab` drops you back into the payload.
* **Session State:** The `weazlback-session` backend keeps one vault-owning process alive in a `tmux` session. Hit `q` to detach the visible terminal and let the backup grind without taxing the gig.
* **Cancellation:** `Ctrl+C` is your explicit kill switch, but the widget demands absolute confirmation before aborting the job.
* **Lockdown:** An explicit quit-and-lock terminates the process and wipes the in-memory key clean.

Selective recovery lets you search or browse a Restore Point, extract it into private staging, verify the vector, and explicitly confirm it before Weazlback overwrites your live metal.

## The Omarchy Quattro Widget

The schema-v1 bar widget flies the floppy glyph ``. Left-click cracks open the compact tactical panel; right-click drops into Travel Mode.

Routine backups grind Core and Home concurrently with separate progress lanes. Security is absolute: the vault passphrase pipes straight to the binary over `stdin`—it never touches `argv` or the environment. The key stays alive only for the current Omarchy shell session, until you hit **Lock vault**, reload the shell, or log out. Widget status is sanitized local JSON; true Restic diagnostics are vaulted ciphertext under `~/.weazlback/logs` (50 retained).

An hourly user timer enforces the perimeter. It checks if your last healthy backup is seven days old, respects the 59% battery floor, preserves your stay-awake state, and absolutely refuses to prune unattended.

### The Sandbox and Uninstallation

The installer stages user-owned plugin files, bounces the shell to rescan, enables the widget, and records the binary path. It calls exactly what it needs: `restic`, `ssh-agent`/`ssh-add`, `sudo` (only after visible UI auth), `pkexec`, `notify-send`, and `tmux`.

Nuke the plugin interface without deleting your vaults or working binaries:

```sh
omarchy plugin disable io.github.bprendie.weazlback
omarchy plugin remove io.github.bprendie.weazlback
```

To rip out the timer and widget while preserving the vault, encrypted logs, repositories, recovery media, and working binaries, run:

```sh
weazlback-uninstall
```

## Forge the Binary and Run Drills

```sh
go test ./...
go build -o ./weazlback ./cmd/weazlback
go build -o ./weazlback-restore ./cmd/weazlback-restore
./weazlback doctor
./weazlback init --name local --repository /mnt/weazlback/repository
./weazlback backup --profile core
./weazlback heavy inspect
./weazlback backup --profile heavy
./weazlback list
./weazlback files --snapshot latest --query api-keys
./weazlback restore --snapshot latest --target /private/staging --include /home/me/file
./weazlback check
./weazlback recovery export --output /mnt/weazlback/weazlback-recovery.wzrk
./weazlback inventory --output ./weazlback-inventory.json
./weazlback applications --output /secure/path/applications.json
./weazlback packages refresh
./weazlback packages refresh --build-missing-aur
./weazlback packages schedule --days 30
./weazlback benchmark --engine borg --fixture all
./weazlback benchmark --engine restic --fixture all
```

To run the retained benchmark harness on Omarchy: `omarchy pkg add borg restic`.

## Package Capsule

Fresh metal shouldn't spend its first hour begging mirrors for packages or compiling an AUR leviathan. The Package Capsule is a dedicated encrypted Restore Point holding curated Arch artifacts and the ledger needed to validate them. Routine backups never crawl Pacman, Yay, or Paru caches.

Open **Profiles** and press `P` to harvest cached artifacts, download missing official packages, validate architecture and build flags, hash package signatures, and write `profile:packages` into the active Restic repo. Press `A` only after reading the warning: that path clones and executes missing AUR PKGBUILDs *now* so a fresh-system recovery doesn't compile them later. `S` sets an independent 30-day refresh reminder.

Every capsule records installed and artifact versions, Arch install reason, `.PKGINFO`, `.BUILDINFO`, dependencies, providers, conflicts, Flatpaks, hashes, and exceptions. An incompatible architecture or `-march=native` artifact is aggressively rejected. A missing artifact stays a visible fallback item—it is never dressed up as recovery-ready.

On fresh metal, recovery inventories packages already supplied by the current Omarchy ISO before installing anything. Arch's own version rules decide the delta. Every selected artifact is extracted into private staging, rehash-checked, and signature-verified. Pacman gets one coordinated local transaction—never a cache wildcard and never a forced overwrite.

Package Capsule extraction and installation run beside filesystem recovery. Fresh System Recovery exposes separate lanes for **Filesystem**, **Package Capsule**, and **Applications** so a slow mirror or AUR fallback can't hide data progress.

### Turbo Recovery Foundation

Fresh recovery records a private mode-0600 schema-v2 journal. Completion includes a per-filesystem `syncfs`; bytes merely accepted by Linux's page cache are not called durable.

Turbo can be selected explicitly in the Fresh Recovery TUI or via `--engine turbo`. Standard remains the default. Turbo budgets max 70% of `MemAvailable` while respecting cgroup limits. On Btrfs it uses a private, compression-disabled, same-filesystem staging subvolume for atomic landing. Any cross-device placement or staging failure journals out before Restic Standard takes the wheel.

Turbo's reader is a deliberately narrow bridge to pinned Restic v0.19.1 code. No repacked backups, no home-grown crypto. The TUI reports logical output rate separately from actual wire rate. Don't cook the scoreboard.

Compare official Standard and embedded Turbo with medians:

```sh
weazlback benchmark --engine restic --fixture all --trials 3
weazlback benchmark --engine turbo --fixture all --trials 3 --connections 4
```

Turbo is promoted per workload only after it beats Standard without durability regressions.

## Prime Vector Nugs: The Heavy Lane

Heavy data is a separate, deduplicated lane exclusively for VM and container assets. Weazlback is merciless here: before firing a Heavy backup, it sweeps roots for writable open files. If a VM or container is live, it hard-refuses the capture. Stop the workload and retry. There is no unsafe override.

Restores use sparse-file extraction. Pruning retention demands explicit confirmation:

```sh
weazlback prune --profile heavy          # preview only
weazlback prune --profile heavy --apply  # requires PRUNE <repository ID>
```

## Vault Rule: No Lifeguard on Duty

There is no strength policy, and there is zero lost-passphrase recovery. You lose your passphrase, you permanently nuke your access to all encrypted backups and `.wzrk` recovery kits. Back up your metal.

## The Recovery Kit (.wzrk)

The recovery USB is not a bloated copy of your backups—it’s the raw bootstrap bundle that lets a fresh Omarchy install find, unlock, and restore the prime data.

The USB packs:

* `weazlback-restore`: Starts recovery even if Weazlback isn't installed.
* `weazlback-recovery.wzrk`: Holds every configured repository location, pinned SSH host key, and repository secret inside a second encrypted envelope.
* The raw `weazlback` binary, offline docs, and SHA-256 checksums.
* A verified Restic binary.

**The `.wzrk` file does not contain your vault passphrase.** A stolen USB is a brick without it. To cut a new kit to an existing writable folder:

```sh
weazlback recovery prepare --target /mnt/WEAZLBACK-RECOVERY
```

### Break-Glass Destruction

If a repo is compromised, the recovery folder provides a cryptographic destruction path:

```sh
./weazlback-restore --recovery ./weazlback-recovery.wzrk --nuke-repository
```

This demands the vault passphrase and the exact phrase `NUKE <repository ID>`. The default scorches the repository data and its keys. Append `--nuke-keys-only` to intentionally strand inaccessible ciphertext.

## Restoration Field Guide

**Restore Mode** handles deleted files and point-in-time rewinds while the machine is alive. **Fresh System Recovery** rebuilds a clean Omarchy installation from bare metal. Same encrypted repository. Different blast radius.

### Restore Mode: The Rig Still Boots

Open Weazlback from the widget, focus the navigation rail with `Tab`, and press `R`. The entire TUI switches into the restore workspace; press `B` from its dashboard to return to backup operations. The widget's **Restore files** action lands in the same persistent `tmux` backend, so closing the terminal does not vaporize an active transaction.

The dashboard exposes five recovery tools:

* **Browse history (`f` or `Enter`):** Walk one Restore Point as a filesystem tree. `Right` or `Enter` descends into a directory; `Left` climbs toward `/`; `↑` and `↓` move the cursor.
* **Search (`/`):** Fuzzy-search filenames across known timestamps. Results show the same path once with its available version dates, instead of burying you in duplicate filenames.
* **Path mode (`/p PATH`):** Jump straight to a known location such as `/p ~/Pictures/wallpapers`. An exact directory opens the tree there; a partial path filters inside the selected Restore Point.
* **Bundle restore (`g`):** Restore System Config, Personal Files, Heavy data, or a selected combination around a requested time.
* **Applications (`a`):** Reconcile the selected machine's package and service manifest without touching filesystem bundles.

The browser always identifies the source machine and exact Restore Point at the top. Use `i` to cycle machine identities, `[` and `]` to move through time, and `A` when a filename may exist on another rig. `H` expands a path search across its catalogued history. Deleted paths are marked in orange; yellow is selection, not danger.

Press `Space` to add files or directories to the restore basket. Every basket item remains pinned to the machine and Restore Point where you selected it, even if you keep browsing through time. Press `e` to execute the basket, then choose where the payload lands:

* **Original path:** Put it back where it lived, mapping the source home into the current user's home.
* **Private staging:** Extract and validate it without touching live files.
* **Alternate directory:** Redirect the payload somewhere you control.

Existing live objects are preserved for rollback before replacement. Weazlback stages, validates, journals, and then places the batch as one transaction—you do not approve a thousand files one at a time. If placement fails, the encrypted resumable journal records exactly where the grind stopped.

### Bundle Restore: Reset the Clock

**Safe Overlay** is the daily-driver option. It restores the selected bundle, preserves rollback copies of conflicts, and leaves unrelated live files alone.

**Exact Rewind** makes selected boundaries match the chosen point in time, including removing paths proven absent then. That is deliberately destructive. Weazlback shows the deletion queue, warns that missing or corrupt files are possible, offers to capture a quick current-state backup, and requires an explicit `YES` acknowledgement before it fires. If you are merely recovering a lost file, use the basket—not the orbital cannon.

Restore Points are composed honestly. You choose the anchor time; Core, Home, and Heavy resolve to their nearest healthy points, and the final plan discloses every actual timestamp before anything mutates the machine.

### Fresh System Recovery: Bare Metal

Install Omarchy, mount the USB, and verify the payload before touching the vault:

```sh
cd /mnt/WEAZLBACK-RECOVERY
sha256sum -c SHA256SUMS
./weazlback-restore
```

Prepared media verifies `SHA256SUMS` automatically. Mismatches hard-stop before the `.wzrk` envelope is opened. Don't freestyle around a failed checksum.

The guided recovery flow asks for these decisions in order:

1. **Recovery kit and vault passphrase.** This unlocks the `.wzrk` envelope in memory. The passphrase is never stored on the USB.
2. **Destination.** Choose any local or SSH repository embedded in the kit. An SSH target needs storage and SSH/SFTP access; Restic runs from the recovery media, not on the target.
3. **Source machine.** Pick the laptop or desktop whose history you intend to restore. Machine identities share repository deduplication without bleeding manifests into one another.
4. **Target identity.** Keep the fresh installation's identity, generate a new independent identity, or explicitly adopt the source identity when this is replacement hardware.
5. **Restore Point and action.** Restore a bundle, applications only, one selected path, or build the optional encrypted history catalog.
6. **Hostname.** Keep the fresh hostname, inject a new one, or restore the original for applications such as Chromium that bind state to the machine name.
7. **Final plan.** Review the precise Core, Home, Heavy, package, service, identity, and hostname operations before authorizing the run.

Recovery scopes are intentionally blunt and legible:

* **Core:** Settings, dotfiles, widgets, Weazl apps, application manifests, services, and hostname—the fastest path back to a usable rig.
* **Home:** Core plus normal home files. This is the recommended fresh-system restore.
* **Everything:** Applications in parallel with Core, Home, and the Heavy lane containing VMs and container assets.
* **Applications:** Reconciles the chosen identity's package and service manifest without placing filesystem bundles.
* **Selective:** Browses one Restore Point immediately and restores one file or directory through the same rollback-preserving transaction engine. Building the cross-time catalog is optional.

Filesystem placement, Package Capsule reconciliation, and online application fallback
run in parallel. Slow AUR builds do not hold your home directory hostage, and the
interface shows independent progress lanes. Package conflicts, unavailable services,
and manual-review items are exceptions—not permission to pretend the restore was
clean. The final journal tells you what landed locally, what fell back online, and
what still needs human judgment.

For a zero-mutation drill, use the explicit inspection flags:

```sh
# Unlock, verify repository access, and print the exact plan
./weazlback-restore --recovery ./weazlback-recovery.wzrk --plan-only

# Decrypt and validate Core into private staging only
./weazlback-restore --recovery ./weazlback-recovery.wzrk --stage-only
```

A recovery kit you've never bootstrapped is a theory, not a backup strategy.

### Browser Compatibility After Hostname Changes

Core, Home, and Everything recovery automatically inspect restored browser
profiles when the source and target hostnames differ. Weazlback removes only
marker-validated transient Chromium `Singleton*` locks and Mozilla
`.parentlock`/`lock` entries. Cookies, sessions, credentials, history,
extensions, preferences, and every other profile file remain untouched.

Close browsers before recovery. A running matching browser is never killed;
its locks are skipped and the restore reports an exception. Native and Flatpak
profiles are supported for Chromium, Chrome, Brave, Edge, Vivaldi, Opera,
Firefox, LibreWolf, Waterfox, Floorp, Zen, Mullvad Browser, and their documented
variants. Unknown, custom, ambiguous, symlinked, or foreign-owned roots are
left alone.

Inspect or repair stale locks without repository access:

```sh
weazlback browser repair          # count-only dry run
weazlback browser repair --apply  # exact validated unlink
```

The command prints sanitized counts, not profile paths. New backups omit
validated transient locks; older Restore Points remain compatible and are
repaired after placement when required.

## Flow State Tuning

Don't guess your throughput—measure it. Run `weazlback tune` after a Core backup. It benchmarks 2, 4, and 10 repository connections.

For SSH targets, tuning streams 100 MiB of cryptographic random data into an ephemeral remote probe file, measures sustained bandwidth, deletes the probe, and recommends a 79% aggregate upload ceiling.

```sh
# Pin repository concurrency
weazlback tune --connections 10

# Pin both values
weazlback tune --connections 10 --upload-limit-mib 79

# Return SSH destination to unlimited upload
weazlback tune --connections 10 --upload-limit-mib 0
```

## Appendix: Turbo Recovery Under the Hood

Standard recovery is the stock car: conservative and proven. Turbo is the forced-induction path for fresh metal with a fast local or SFTP source, enough memory, and a destination filesystem that can take the hit.

### The boost path

```text
encrypted Restic packs
        │
        ▼
authenticated pack-ordered readers ── actual source / wire meter
        │
        ▼
bounded decrypt + sparse writers ───── logical output meter
        │
        ▼
private Btrfs landing subvolume
        │
        ▼
Core over Home composition ─────────── applications run in parallel
        │
        ▼
metadata audit → syncfs → durable
```

The embedded reader uses pinned Restic v0.19.1 BSD code. It parses indexes, authenticates objects, and preserves sparse zero regions. On a qualifying Btrfs target, Turbo disables compression during the critical landing to avoid a cross-device copy. Optional recompression happens *after* the durability milestone.

### Honest gauges

Turbo shows two rates to avoid dyno fraud:

* **Source/wire rate:** Encrypted data actually read from the repo.
* **Output rate:** Authenticated logical plaintext materialized on the target.

### Measured physical result

An anonymized fresh-metal drill restored roughly 250 GB from fast local USB-C onto a four-core ultraportable with 32 GB RAM and a PCIe Gen3 NVMe:

| Path | Total wall time | Relative result |
| --- | --- | --- |
| Earlier Standard recovery baseline | ~17m30s | 1.00x |
| Turbo Everything recovery | **7m51s** | **2.23x effective throughput** |
| Reduction | **~9m39s** | **55.1% less wall time** |

During the run, encrypted input held near 425 MB/s, logical output held near 550 MB/s, and CPU stayed around 80%. Turbo earns its name by exposing the mechanics and falling back cleanly—not by cooking the stopwatch.
