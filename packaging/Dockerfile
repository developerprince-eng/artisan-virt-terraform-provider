# Provider "mirror" image published to GitHub Packages (ghcr.io) by
# .github/workflows/release.yml. It contains no executable to run: it carries the
# signed release zips in Terraform's packed local-mirror layout, so CI images can
# install the provider without network access to any registry:
#
#   FROM hashicorp/terraform:<version>
#   COPY --from=ghcr.io/developerprince-eng/artisan-virt-terraform-provider:<version> \
#        /plugins/ /root/.terraform.d/plugins/
#
# Built from the repository root after `goreleaser release` has filled dist/.
FROM scratch

ARG VERSION
ARG NAME=terraform-provider-artisanvirt

# Terraform's implied local mirror: <dir>/registry.terraform.io/<namespace>/<type>/<zip>.
# The namespace must match RegistryAddress in internal/provider/provider.go.
COPY dist/${NAME}_${VERSION}_*.zip /plugins/registry.terraform.io/developerprince-eng/artisanvirt/

# Checksums, their signature and the signing key, for verification inside images.
COPY dist/${NAME}_${VERSION}_SHA256SUMS dist/${NAME}_${VERSION}_SHA256SUMS.sig artisanvirt-signing.pub.asc /checksums/
