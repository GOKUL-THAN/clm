package deploy

// Shared bootstrap fragments for gateway provisioning.
//
// Gateways are deployed onto whatever Linux the customer runs, so the bootstrap
// cannot assume Debian. Both the Lightsail and EC2 provisioners use the helpers
// here so their scripts cannot drift apart.

// SupportedLinuxFlavours is the set of distributions the bootstrap handles,
// grouped by the package manager it will detect.
var SupportedLinuxFlavours = map[string][]string{
	"apt": {"ubuntu", "debian"},
	"dnf": {"rhel", "rocky", "almalinux", "fedora", "amzn2023"},
	"yum": {"centos", "amzn2", "rhel7", "rhel8"},
}

// packageManagerPreamble returns shell that identifies the distribution and
// defines pkg_update / pkg_install wrappers over apt-get, dnf, or yum.
//
// Written as POSIX-compatible shell with no here-doc so it can be embedded in a
// larger script safely. It deliberately does not fail the whole bootstrap when
// the distribution is unrecognised — it reports the ID it saw, which is far more
// useful than "apt-get: command not found" from a script that assumed Debian.
func packageManagerPreamble() string {
	return `
# --- distribution detection -------------------------------------------------
DISTRO_ID="unknown"
DISTRO_VERSION=""
if [ -r /etc/os-release ]; then
    . /etc/os-release
    DISTRO_ID="${ID:-unknown}"
    DISTRO_VERSION="${VERSION_ID:-}"
fi
echo "=== detected distribution: ${DISTRO_ID} ${DISTRO_VERSION} ==="

PKG=""
if command -v apt-get > /dev/null 2>&1; then
    PKG="apt"
elif command -v dnf > /dev/null 2>&1; then
    PKG="dnf"
elif command -v yum > /dev/null 2>&1; then
    PKG="yum"
fi

if [ -z "$PKG" ]; then
    echo "FATAL: no supported package manager (apt-get, dnf, yum) on ${DISTRO_ID}." >&2
    echo "Supported: Ubuntu, Debian, RHEL, CentOS, Rocky, AlmaLinux, Amazon Linux." >&2
    exit 1
fi
echo "=== using package manager: ${PKG} ==="

pkg_update() {
    case "$PKG" in
        apt) DEBIAN_FRONTEND=noninteractive apt-get update -y ;;
        dnf) dnf makecache -y || true ;;
        yum) yum makecache -y || true ;;
    esac
}

pkg_install() {
    case "$PKG" in
        apt) DEBIAN_FRONTEND=noninteractive apt-get install -y "$@" ;;
        dnf) dnf install -y "$@" ;;
        yum) yum install -y "$@" ;;
    esac
}
# --- end distribution detection ---------------------------------------------
`
}

// basePackages installs the tools the gateway bootstrap needs. curl and jq are
// named identically across all supported package managers; ca-certificates is
// apt-only, since the RPM families ship it in the base image.
func basePackages() string {
	return `
pkg_update
pkg_install curl jq
if [ "$PKG" = "apt" ]; then
    pkg_install ca-certificates
fi
`
}
