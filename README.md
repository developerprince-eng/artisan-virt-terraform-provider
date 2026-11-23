# ![Terraform Provider for Artisan Virt](https://img.shields.io/badge/Terraform_Provider_for_Artisan_Virt_%7C_Artisan_Virt-%23203072.svg?style=for-the-badge)

Manage an Artisan Virt fleet (hypervisor nodes, clusters, KVM virtual machines, host bridges and
backups) with Terraform. The provider talks to the control plane administrator API (`/api/v1`); the API
is described by the OpenAPI document the control plane serves at `/openapi.yaml` (Swagger UI at
`/swagger`).

```hcl
terraform {
  required_providers {
    artisanvirt = {
      source  = "developerprince-eng/artisanvirt"
      version = "~> 0.1"
    }
  }
}

provider "artisanvirt" {
  endpoint = "https://artisan.example.com" # or ARTISANVIRT_ENDPOINT
  # token  = "..."                         # or ARTISANVIRT_TOKEN
}

resource "artisanvirt_vm" "web" {
  name       = "web01"
  vcpus      = 2
  memory_mib = 4096
  disks      = [{ size_gib = 20 }]
}
```

## Installing

Every release is published four ways. Use the Terraform Registry once the provider is listed there
(see the naming note under [Publishing](#publishing)); until then, use GitHub Releases.

| Source | Access | Use it for |
|---|---|---|
| [Terraform Registry](https://registry.terraform.io/providers/developerprince-eng/artisanvirt) | public, once listed | normal use: `terraform init` downloads and verifies the provider |
| [GitHub Releases](https://github.com/developerprince-eng/artisan-virt-terraform-provider/releases) | public | offline / air-gapped machines, pinning a downloaded copy |
| [GitHub Packages](https://github.com/developerprince-eng/artisan-virt-terraform-provider/pkgs/container/artisan-virt-terraform-provider) (ghcr.io image) | public | CI / container images: `COPY --from` the provider into a Terraform image |
| GitLab package registry | private, token required | project members and internal build machines |

All of them carry the same signed files: one zip per platform, `SHA256SUMS`, and `SHA256SUMS.sig`. The
signature is made with key `6B0B6A4CD2811E216A9D898721806376C7816FDA`, whose public key is
[`artisanvirt-signing.pub.asc`](artisanvirt-signing.pub.asc) in this repository.

### From the Terraform Registry (recommended)

```hcl
terraform {
  required_providers {
    artisanvirt = {
      source  = "developerprince-eng/artisanvirt"
      version = "~> 0.1"
    }
  }
}
```

`terraform init` downloads the provider and checks its signature. Pre-releases (`v1.2.0-rc.1`) are only
installed when the version is pinned exactly, e.g. `version = "1.2.0-rc.1"`.

### From GitHub Releases

Each [GitHub release](https://github.com/developerprince-eng/artisan-virt-terraform-provider/releases) carries
the zips and checksums; the release page repeats these steps with its own version filled in. Terraform
picks up a zip placed in its local plugin directory (the "implied local mirror") without contacting any
registry for this provider.

Linux and macOS:

```sh
VERSION=0.1.0
PLATFORM=linux_amd64   # linux_arm64, darwin_arm64, darwin_amd64, freebsd_amd64, ...
NAME=terraform-provider-artisanvirt
BASE=https://github.com/developerprince-eng/artisan-virt-terraform-provider/releases/download/v${VERSION}
curl -fLO "$BASE/${NAME}_${VERSION}_${PLATFORM}.zip"
curl -fLO "$BASE/${NAME}_${VERSION}_SHA256SUMS"
curl -fLO "$BASE/${NAME}_${VERSION}_SHA256SUMS.sig"

# Verify the signature, then the zip's checksum.
gpg --import artisanvirt-signing.pub.asc        # once; the file is in this repository
gpg --verify ${NAME}_${VERSION}_SHA256SUMS.sig ${NAME}_${VERSION}_SHA256SUMS
grep "_${PLATFORM}.zip" ${NAME}_${VERSION}_SHA256SUMS | sha256sum -c -   # macOS: shasum -a 256 -c -

DIR=~/.terraform.d/plugins/registry.terraform.io/developerprince-eng/artisanvirt
mkdir -p "$DIR" && mv ${NAME}_${VERSION}_${PLATFORM}.zip "$DIR/"
```

Windows (PowerShell):

```powershell
$Version  = "0.1.0"
$Platform = "windows_amd64"   # or windows_arm64, windows_386
$Name = "terraform-provider-artisanvirt"
$Base = "https://github.com/developerprince-eng/artisan-virt-terraform-provider/releases/download/v$Version"
$Zip  = "${Name}_${Version}_${Platform}.zip"
$Sums = "${Name}_${Version}_SHA256SUMS"
Invoke-WebRequest "$Base/$Zip"  -OutFile $Zip
Invoke-WebRequest "$Base/$Sums" -OutFile $Sums
$Expected = ((Select-String -Path $Sums -SimpleMatch $Zip).Line -split '\s+')[0]
if ((Get-FileHash $Zip -Algorithm SHA256).Hash -ne $Expected) { throw "Checksum mismatch for $Zip" }
$Dir = "$env:APPDATA\terraform.d\plugins\registry.terraform.io\developerprince-eng\artisanvirt"
New-Item -ItemType Directory -Force $Dir | Out-Null
Move-Item $Zip $Dir -Force
```

(To check the signature on Windows as well, run the same `gpg --verify` with Gpg4win.)

With the [GitHub CLI](https://cli.github.com/), the download is one command:

```sh
gh release download v0.1.0 --repo developerprince-eng/artisan-virt-terraform-provider \
  --pattern '*_linux_amd64.zip' --pattern '*_SHA256SUMS' --pattern '*_SHA256SUMS.sig'
```

Then `terraform init` in your configuration. Keep the `required_providers` block from the registry
example above; the source address stays `developerprince-eng/artisanvirt`.

### From GitHub Packages (container image)

Each release also publishes `ghcr.io/developerprince-eng/artisan-virt-terraform-provider:<version>`
(and `:latest` for final versions). It is not a program to run: it holds the release zips in
Terraform's local-mirror layout under `/plugins`, plus `SHA256SUMS`, its signature and the public key
under `/checksums`. Copy it into the image your pipelines run Terraform in:

```dockerfile
FROM hashicorp/terraform:latest
COPY --from=ghcr.io/developerprince-eng/artisan-virt-terraform-provider:0.1.0 /plugins/ /root/.terraform.d/plugins/
```

`terraform init` in that image then installs `developerprince-eng/artisanvirt` from the copied files,
with no access to any registry needed for this provider. To pull the files onto a machine without
Docker, use any OCI tool, e.g. `crane export ghcr.io/developerprince-eng/artisan-virt-terraform-provider:0.1.0 - | tar -x`.

### From the GitLab package registry (private)

For project members and internal machines that should not depend on the public registry. The GitLab
project is private, so every download needs a token:

- a **deploy token** with the `read_package_registry` scope (*Settings → Repository → Deploy tokens*),
  best for build machines: `AUTH="DEPLOY-TOKEN: <token>"`; or
- a personal or project **access token** with `read_api`: `AUTH="PRIVATE-TOKEN: <token>"`.

Terraform's implied local mirror accepts the release zip as is:

```sh
AUTH="DEPLOY-TOKEN: <token>"        # or "PRIVATE-TOKEN: <token>"
VERSION=0.1.0                       # tag v0.1.0
PLATFORM=linux_amd64                # darwin_arm64, windows_amd64, ...
BASE=https://gitlab.developer.co.zw/api/v4/projects/<project-id>/packages/generic/terraform-provider-artisanvirt/v${VERSION}
DIR=~/.terraform.d/plugins/registry.terraform.io/developerprince-eng/artisanvirt   # %APPDATA%\terraform.d\plugins\... on Windows
mkdir -p "$DIR"
curl -fL --header "$AUTH" -o "$DIR/terraform-provider-artisanvirt_${VERSION}_${PLATFORM}.zip" \
  "$BASE/terraform-provider-artisanvirt_${VERSION}_${PLATFORM}.zip"
curl -fL --header "$AUTH" -O "$BASE/terraform-provider-artisanvirt_${VERSION}_SHA256SUMS"
(cd "$DIR" && grep "_${PLATFORM}.zip" "$OLDPWD/terraform-provider-artisanvirt_${VERSION}_SHA256SUMS" | sha256sum -c -)
terraform init
```

`<project-id>` is shown on the project's overview page. `terraform init` then uses the mirrored zip for
`developerprince-eng/artisanvirt` and the public registry for every other provider. Keep tokens out of
shell history and scripts in the repository (for example, read them from an environment variable set
by your secret store).

The GitLab release page and `release.json` links point to the same private URLs, so they also open only
for signed-in project members or with one of these tokens.

## Resources and data sources

| Resource | API | Notes |
|---|---|---|
| `artisanvirt_enrollment_token` | `/enrollment-tokens` | Single-use token for agent enrollment; `value` is sensitive. |
| `artisanvirt_node` | `/nodes/{id}/approve`, `/maintenance`, `/decommission` | Approves an enrolled host and waits for provisioning. |
| `artisanvirt_bridge` | `/nodes/{id}/bridges` | Isolated or NAT libvirt bridge. |
| `artisanvirt_cluster` | `/clusters` | Corosync/Pacemaker cluster; replaced on change. |
| `artisanvirt_cluster_member` | `/clusters/{id}/nodes` | Join/leave after preflight. |
| `artisanvirt_vm` | `/vms`, `/vms/{id}/actions/*` | Power state changes in place; hardware changes replace. |
| `artisanvirt_vm_snapshot` | `/vms/{id}/snapshots` | Internal snapshot. |
| `artisanvirt_vm_gpu` | `/vms/{id}/gpu` | PCI / SR-IOV passthrough to a stopped UEFI VM. |
| `artisanvirt_backup_target` | `/backup-targets` | NFS or filesystem repository. |
| `artisanvirt_backup_policy` | `/backup-policies` | Cron schedule, retention, selector; updated in place. |

Data sources: `artisanvirt_node`, `artisanvirt_nodes`, `artisanvirt_cluster`, `artisanvirt_clusters`,
`artisanvirt_vm`, `artisanvirt_vms`, `artisanvirt_backup_targets`, `artisanvirt_backups`.

Long-running operations return `202 Accepted` from the API; the provider waits for the queued task (or
the resource state) to finish and honours `timeouts { create/update/delete }` blocks. Full reference:
[`docs/`](docs/) (rendered by the Terraform Registry).

The control plane has no update endpoint for VMs, clusters, bridges, backup targets or tokens, so
changing their arguments plans a replacement. Read the plan before applying.

## Development

Requirements: Go 1.25.8+, Terraform 1.5+ (for the tests and doc generation), GNU make (optional).

```sh
make build      # go build
make test       # unit tests + Terraform-driven tests against an in-memory fake API
make docs       # regenerate docs/ from the schema and examples/ (commit the result)
make install    # install into ~/.terraform.d/plugins for local use
```

Without make: `go test ./...`, `go generate ./...`.

To try a local build against a real control plane, add a dev override to `~/.terraformrc`
(`%APPDATA%\terraform.rc` on Windows):

```hcl
provider_installation {
  dev_overrides {
    "developerprince-eng/artisanvirt" = "/path/to/go/bin"
  }
  direct {}
}
```

then `go install .` and run `terraform plan` (no `terraform init` needed for overridden providers).

## Publishing

The repository is pushed to both GitLab and GitHub, and each host publishes its own copy of every
version:

| Host | Pipeline | Publishes to |
|---|---|---|
| GitLab | [`.gitlab-ci.yml`](.gitlab-ci.yml) | GitLab Generic Package Registry + GitLab Releases (optionally HCP Terraform) |
| GitHub | [`.github/workflows/release.yml`](.github/workflows/release.yml) | GitHub Releases, which the **public Terraform Registry** ingests |

The GitHub copy lives at
[github.com/developerprince-eng/artisan-virt-terraform-provider](https://github.com/developerprince-eng/artisan-virt-terraform-provider);
the `Release` workflow publishes every tag to that repository's
[Releases](https://github.com/developerprince-eng/artisan-virt-terraform-provider/releases). Both
pipelines run GoReleaser with the same [`.goreleaser.yml`](.goreleaser.yml), so file names and layout
match on both hosts.

> **Terraform Registry naming rule.** The public Terraform Registry only accepts a public GitHub
> repository named `terraform-provider-<type>` (lowercase). `artisan-virt-terraform-provider` does not
> match, so releases there work for GitHub downloads but cannot be added to registry.terraform.io.
> To list the provider on the registry, rename the GitHub repository to `terraform-provider-artisanvirt`
> (*Settings → General → Repository name*; GitHub redirects the old URLs, including release downloads).
> The provider address `developerprince-eng/artisanvirt` stays the same either way.

### One-time setup

1. **Remotes.** Push the project to both hosts, for example:
   ```sh
   git remote add origin https://gitlab.developer.co.zw/developerprince/terraform-provider-artisanvirt.git
   git remote add github https://github.com/developerprince-eng/artisan-virt-terraform-provider.git
   git push origin main && git push github main
   ```
   The GitHub owner `developerprince-eng` is the provider namespace, so users write
   `source = "developerprince-eng/artisanvirt"`. If the owner ever changes, update `RegistryAddress` in
   `internal/provider/provider.go`, the `source` in `examples/`, `.goreleaser.yml` and this README.
2. **Signing key.** Create an RSA key (the Terraform Registry does not accept ECC keys):
   ```sh
   gpg --pinentry-mode loopback --full-generate-key                      # RSA and RSA, 4096 bits
   gpg --list-secret-keys --keyid-format long   # note the fingerprint
   gpg --armor --export <FINGERPRINT>                          # public key, for the registry
   gpg --armor --export-secret-keys <FINGERPRINT> | base64 -w0 # private key, for CI secrets
   ```
3. **GitHub secrets** in `developerprince-eng/artisan-virt-terraform-provider`
   (*Settings → Secrets and variables → Actions*):

   | Secret | Value |
   |---|---|
   | `GPG_PRIVATE_KEY` | the armored private key or its base64 |
   | `GPG_PASSPHRASE` | key passphrase, if any |
   | `GIT_TOKEN` | fine-grained token for this repository with *Contents: Read and write* (creates the release) |

   (`release.yml` reads `secrets.GIT_TOKEN`. Switching it to the built-in `secrets.GITHUB_TOKEN` also
   works, because the workflow grants `contents: write`, and removes the need for this secret.)
4. **Terraform Registry** (after renaming the repository, see the note above). Sign in to
   [registry.terraform.io](https://registry.terraform.io) with the `developerprince-eng` GitHub account,
   add the public key under *Settings → Signing Keys*, then *Publish → Provider* and pick the repository.
   Do this once the first GitHub release exists; later releases are picked up automatically.
5. **GitLab CI/CD variables** (*Settings → CI/CD → Variables*, Masked and Protected):

   | Variable | Value |
   |---|---|
   | `GPG_PRIVATE_KEY` | base64 of the armored secret key (or a *File* variable with the armored key) |
   | `GPG_FINGERPRINT` | the key fingerprint |
   | `GPG_PASSPHRASE` | key passphrase, if any |
   | `RELEASE_TOKEN` | project access token with `api` + `write_repository`, used by the manual `tag-release` job |
   | `TFC_TOKEN`, `TFC_ORGANIZATION` | optional: also publish to an HCP Terraform private registry (`TFC_HOSTNAME` for Terraform Enterprise) |

   The GPG variables are optional on GitLab: without them tags are published unsigned. Protected
   variables are only exposed to protected refs, so protect the tag pattern `v*` under
   *Settings → Repository → Protected tags*. Uploads use the job's `CI_JOB_TOKEN`; no extra token is
   needed for the package registry.

### Releasing a version

Pushing a branch only runs the tests. **Nothing is published until a version tag exists**, so a repository
with no tags shows no releases and no packages.

Add the changes to [CHANGELOG.md](CHANGELOG.md), push the default branch to both remotes, then tag the
same commit on both:

```sh
git tag v0.1.0
git push origin v0.1.0    # GitLab: package registry + GitLab release
git push github v0.1.0    # GitHub: GitHub release -> Terraform Registry
```

On GitHub you can instead open **Actions → Release → Run workflow** on `main`: choose a
`patch`/`minor`/`major` bump or type an exact version, and the workflow runs the tests, creates the tag
and publishes the release and package in one run. Push the new tag to GitLab afterwards if you want the
GitLab copy too: `git fetch github --tags && git push origin v0.1.1`.

On GitLab, run the manual **`tag-release`** job on the default-branch pipeline (it bumps the patch version
of the latest tag; set `BUMP=minor`/`BUMP=major`, or `VERSION=v1.4.0`), then mirror the tag:
`git fetch origin --tags && git push github v1.4.0`.

Each tag pipeline runs the tests first:

- **GitLab** (`build-and-release`) uploads the zips, `terraform-provider-artisanvirt_<version>_SHA256SUMS`,
  its `.sig`, the registry manifest and a `release.json` build manifest to
  `…/packages/generic/terraform-provider-artisanvirt/<tag>/`, creates a GitLab release linking to them,
  and points `…/stable/release.json` at it. Default-branch pushes publish an unsigned snapshot as
  `latest` (`edge` channel), as artisan-virt does.
- **GitHub** (`Release` workflow) creates the GitHub release with the same files plus install notes,
  and pushes the provider mirror image `ghcr.io/developerprince-eng/artisan-virt-terraform-provider:<version>`
  to GitHub Packages. Once the provider is listed, the Terraform Registry ingests the release within a
  few minutes.

Pre-release tags (`v1.2.0-rc.1`) become GitHub pre-releases, which Terraform only installs when a
configuration pins that exact version.
