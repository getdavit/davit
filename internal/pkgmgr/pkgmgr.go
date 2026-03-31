// Package pkgmgr provides a unified interface over Linux package managers.
package pkgmgr

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/getdavit/davit/internal/osdetect"
)

// PackageManager is the interface every distro-specific implementation satisfies.
type PackageManager interface {
	Update() error
	Upgrade() error
	Install(packages ...string) error
	IsInstalled(pkg string) bool
	Purge(packages ...string) error
}

// New returns the correct PackageManager for the detected OS profile.
func New(kind osdetect.PkgMgrKind) (PackageManager, error) {
	switch kind {
	case osdetect.PkgMgrApt:
		return &Apt{}, nil
	case osdetect.PkgMgrDnf:
		return &Dnf{}, nil
	case osdetect.PkgMgrYum:
		return &Yum{}, nil
	case osdetect.PkgMgrZypper:
		return &Zypper{}, nil
	case osdetect.PkgMgrPacman:
		return &Pacman{}, nil
	case osdetect.PkgMgrApk:
		return &Apk{}, nil
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", kind)
	}
}

// run executes a command and returns combined stdout+stderr on failure.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %v: %w\n%s", name, args, err, buf.String())
	}
	return nil
}

// isInstalled checks for a binary or package by probing PATH and dpkg/rpm/etc.
func binExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// --- apt ---

type Apt struct{}

func (a *Apt) Update() error {
	return run("apt-get", "update", "-qq")
}

func (a *Apt) Upgrade() error {
	return run("apt-get", "upgrade", "-y", "-qq",
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold")
}

func (a *Apt) Install(packages ...string) error {
	args := append([]string{"install", "-y", "-qq"}, packages...)
	return run("apt-get", args...)
}

func (a *Apt) IsInstalled(pkg string) bool {
	err := exec.Command("dpkg", "-s", pkg).Run()
	return err == nil
}

func (a *Apt) Purge(packages ...string) error {
	args := append([]string{"purge", "-y", "-qq"}, packages...)
	return run("apt-get", args...)
}

// --- dnf ---

type Dnf struct{}

func (d *Dnf) Update() error {
	return run("dnf", "makecache", "-q")
}

func (d *Dnf) Upgrade() error {
	return run("dnf", "upgrade", "-y", "-q")
}

func (d *Dnf) Install(packages ...string) error {
	args := append([]string{"install", "-y", "-q"}, packages...)
	return run("dnf", args...)
}

func (d *Dnf) IsInstalled(pkg string) bool {
	err := exec.Command("rpm", "-q", pkg).Run()
	return err == nil
}

func (d *Dnf) Purge(packages ...string) error {
	args := append([]string{"remove", "-y"}, packages...)
	return run("dnf", args...)
}

// --- yum ---

type Yum struct{}

func (y *Yum) Update() error {
	return run("yum", "makecache", "-q")
}

func (y *Yum) Upgrade() error {
	return run("yum", "update", "-y", "-q")
}

func (y *Yum) Install(packages ...string) error {
	args := append([]string{"install", "-y", "-q"}, packages...)
	return run("yum", args...)
}

func (y *Yum) IsInstalled(pkg string) bool {
	err := exec.Command("rpm", "-q", pkg).Run()
	return err == nil
}

func (y *Yum) Purge(packages ...string) error {
	args := append([]string{"remove", "-y"}, packages...)
	return run("yum", args...)
}

// --- zypper ---

type Zypper struct{}

func (z *Zypper) Update() error {
	return run("zypper", "--non-interactive", "refresh")
}

func (z *Zypper) Upgrade() error {
	return run("zypper", "--non-interactive", "update")
}

func (z *Zypper) Install(packages ...string) error {
	args := append([]string{"--non-interactive", "install"}, packages...)
	return run("zypper", args...)
}

func (z *Zypper) IsInstalled(pkg string) bool {
	err := exec.Command("rpm", "-q", pkg).Run()
	return err == nil
}

func (z *Zypper) Purge(packages ...string) error {
	args := append([]string{"--non-interactive", "remove"}, packages...)
	return run("zypper", args...)
}

// --- pacman ---

type Pacman struct{}

func (p *Pacman) Update() error {
	return run("pacman", "-Sy", "--noconfirm")
}

func (p *Pacman) Upgrade() error {
	return run("pacman", "-Su", "--noconfirm")
}

func (p *Pacman) Install(packages ...string) error {
	args := append([]string{"-S", "--noconfirm", "--needed"}, packages...)
	return run("pacman", args...)
}

func (p *Pacman) IsInstalled(pkg string) bool {
	err := exec.Command("pacman", "-Q", pkg).Run()
	return err == nil
}

func (p *Pacman) Purge(packages ...string) error {
	args := append([]string{"-Rns", "--noconfirm"}, packages...)
	return run("pacman", args...)
}

// --- apk (Alpine) ---

type Apk struct{}

func (a *Apk) Update() error {
	return run("apk", "update")
}

func (a *Apk) Upgrade() error {
	return run("apk", "upgrade")
}

func (a *Apk) Install(packages ...string) error {
	args := append([]string{"add", "--no-cache"}, packages...)
	return run("apk", args...)
}

func (a *Apk) IsInstalled(pkg string) bool {
	err := exec.Command("apk", "info", "-e", pkg).Run()
	return err == nil
}

func (a *Apk) Purge(packages ...string) error {
	args := append([]string{"del"}, packages...)
	return run("apk", args...)
}

// Ensure binExists is used (suppress unused warning).
var _ = binExists
