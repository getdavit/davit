package osdetect

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// InitSystem represents the process supervisor on the server.
type InitSystem string

const (
	InitSystemd  InitSystem = "systemd"
	InitOpenRC   InitSystem = "openrc"
	InitSysVinit InitSystem = "sysvinit"
)

// PkgMgrKind identifies a Linux package manager.
type PkgMgrKind string

const (
	PkgMgrApt    PkgMgrKind = "apt"
	PkgMgrDnf    PkgMgrKind = "dnf"
	PkgMgrYum    PkgMgrKind = "yum"
	PkgMgrZypper PkgMgrKind = "zypper"
	PkgMgrPacman PkgMgrKind = "pacman"
	PkgMgrApk    PkgMgrKind = "apk"
)

// Profile holds all detected OS characteristics used by the provisioner.
type Profile struct {
	ID         string     // e.g. "ubuntu"
	IDLike     string     // e.g. "debian"
	VersionID  string     // e.g. "24.04"
	PrettyName string     // e.g. "Ubuntu 24.04 LTS"
	Arch       string     // normalized: "amd64" | "arm64"
	PkgMgr     PkgMgrKind
	InitSystem InitSystem
}

// Detect reads /etc/os-release and the runtime environment to build a Profile.
// Returns an error if the OS is not supported.
func Detect() (*Profile, error) {
	fields, err := readOSRelease()
	if err != nil {
		return nil, fmt.Errorf("read /etc/os-release: %w", err)
	}

	p := &Profile{
		ID:         strings.ToLower(fields["ID"]),
		IDLike:     strings.ToLower(fields["ID_LIKE"]),
		VersionID:  fields["VERSION_ID"],
		PrettyName: strings.Trim(fields["PRETTY_NAME"], `"`),
		Arch:       normalizeArch(runtime.GOARCH),
	}

	p.PkgMgr, err = detectPkgMgr(p.ID, p.IDLike)
	if err != nil {
		return nil, err
	}

	p.InitSystem = detectInitSystem()
	return p, nil
}

func readOSRelease() (map[string]string, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fields := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"`)
		fields[key] = val
	}
	return fields, scanner.Err()
}

func detectPkgMgr(id, idLike string) (PkgMgrKind, error) {
	combined := id + " " + idLike

	rules := []struct {
		keywords []string
		kind     PkgMgrKind
		binary   string
	}{
		{[]string{"debian", "ubuntu", "raspbian", "linuxmint", "mint", "pop", "elementary", "kali"}, PkgMgrApt, "apt-get"},
		{[]string{"fedora", "rhel", "rocky", "alma", "centos"}, PkgMgrDnf, "dnf"},
		// yum is a fallback for older RHEL/CentOS 7
		{[]string{"opensuse", "sles"}, PkgMgrZypper, "zypper"},
		{[]string{"arch", "manjaro", "endeavour"}, PkgMgrPacman, "pacman"},
		{[]string{"alpine"}, PkgMgrApk, "apk"},
	}

	for _, rule := range rules {
		for _, kw := range rule.keywords {
			if strings.Contains(combined, kw) {
				if _, err := exec.LookPath(rule.binary); err == nil {
					return rule.kind, nil
				}
			}
		}
	}

	// Fallback: probe by binary presence
	for _, pair := range []struct {
		bin  string
		kind PkgMgrKind
	}{
		{"apt-get", PkgMgrApt},
		{"dnf", PkgMgrDnf},
		{"yum", PkgMgrYum},
		{"zypper", PkgMgrZypper},
		{"pacman", PkgMgrPacman},
		{"apk", PkgMgrApk},
	} {
		if _, err := exec.LookPath(pair.bin); err == nil {
			return pair.kind, nil
		}
	}

	return "", fmt.Errorf("unsupported OS: could not determine package manager (id=%q id_like=%q)", id, idLike)
}

func detectInitSystem() InitSystem {
	if _, err := os.Stat("/run/systemd/private"); err == nil {
		return InitSystemd
	}
	if _, err := exec.LookPath("openrc"); err == nil {
		return InitOpenRC
	}
	return InitSysVinit
}

func normalizeArch(goarch string) string {
	switch goarch {
	case "arm64":
		return "arm64"
	default:
		return "amd64"
	}
}
