// Package firewall provides a unified interface over Linux firewall managers.
package firewall

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/getdavit/davit/internal/pkgmgr"
)

// Kind identifies which firewall implementation is active.
type Kind string

const (
	KindUFW       Kind = "ufw"
	KindFirewalld Kind = "firewalld"
	KindIptables  Kind = "iptables"
)

// FirewallRule describes a single allow/deny rule.
type FirewallRule struct {
	Port    int
	Proto   string
	Policy  string
	Service string
}

// Firewall is the interface every implementation satisfies.
type Firewall interface {
	Enable() error
	AllowPort(port int, proto string) error
	DenyPort(port int, proto string) error
	AllowService(name string) error
	Status() ([]FirewallRule, error)
	Reset() error
}

// Detect returns the active firewall implementation. If no firewall is running
// it installs and returns ufw using the supplied package manager.
func Detect(pm pkgmgr.PackageManager) (Firewall, Kind, error) {
	// 1. UFW active?
	if out, err := runOutput("ufw", "status"); err == nil {
		if strings.Contains(out, "Status: active") {
			return &UFW{}, KindUFW, nil
		}
	}

	// 2. firewalld active?
	if err := run("systemctl", "is-active", "--quiet", "firewalld"); err == nil {
		return &Firewalld{}, KindFirewalld, nil
	}

	// 3. iptables available?
	if err := run("iptables", "-L"); err == nil {
		return &Iptables{}, KindIptables, nil
	}

	// 4. Install ufw
	if err := pm.Install("ufw"); err != nil {
		return nil, "", fmt.Errorf("install ufw: %w", err)
	}
	return &UFW{}, KindUFW, nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	return cmd.Run()
}

func runOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// --- UFW ---

type UFW struct{}

func (u *UFW) Enable() error {
	return run("ufw", "--force", "enable")
}

func (u *UFW) AllowPort(port int, proto string) error {
	rule := fmt.Sprintf("%d/%s", port, proto)
	return run("ufw", "allow", rule)
}

func (u *UFW) DenyPort(port int, proto string) error {
	rule := fmt.Sprintf("%d/%s", port, proto)
	return run("ufw", "deny", rule)
}

func (u *UFW) AllowService(name string) error {
	return run("ufw", "allow", name)
}

func (u *UFW) Status() ([]FirewallRule, error) {
	// Return empty list — full parsing not required for v0.1
	return nil, nil
}

func (u *UFW) Reset() error {
	return run("ufw", "--force", "reset")
}

func (u *UFW) SetDefaultDeny() error {
	return run("ufw", "default", "deny", "incoming")
}

func (u *UFW) SetDefaultAllow() error {
	return run("ufw", "default", "allow", "outgoing")
}

// --- firewalld ---

type Firewalld struct{}

func (f *Firewalld) Enable() error {
	return run("systemctl", "enable", "--now", "firewalld")
}

func (f *Firewalld) AllowPort(port int, proto string) error {
	rule := fmt.Sprintf("--add-port=%d/%s", port, proto)
	return run("firewall-cmd", "--permanent", rule)
}

func (f *Firewalld) DenyPort(port int, proto string) error {
	rule := fmt.Sprintf("--remove-port=%d/%s", port, proto)
	return run("firewall-cmd", "--permanent", rule)
}

func (f *Firewalld) AllowService(name string) error {
	return run("firewall-cmd", "--permanent", fmt.Sprintf("--add-service=%s", name))
}

func (f *Firewalld) Status() ([]FirewallRule, error) {
	return nil, nil
}

func (f *Firewalld) Reset() error {
	return run("firewall-cmd", "--complete-reload")
}

func (f *Firewalld) Reload() error {
	return run("firewall-cmd", "--reload")
}

// --- iptables ---

type Iptables struct{}

func (i *Iptables) Enable() error {
	return nil // iptables is always active once rules are set
}

func (i *Iptables) AllowPort(port int, proto string) error {
	rule := fmt.Sprintf("%d", port)
	return run("iptables", "-A", "INPUT", "-p", proto, "--dport", rule, "-j", "ACCEPT")
}

func (i *Iptables) DenyPort(port int, proto string) error {
	rule := fmt.Sprintf("%d", port)
	return run("iptables", "-A", "INPUT", "-p", proto, "--dport", rule, "-j", "DROP")
}

func (i *Iptables) AllowService(name string) error {
	// Map common service names to ports
	portMap := map[string]struct {
		port  int
		proto string
	}{
		"ssh":   {22, "tcp"},
		"http":  {80, "tcp"},
		"https": {443, "tcp"},
	}
	if s, ok := portMap[name]; ok {
		return i.AllowPort(s.port, s.proto)
	}
	return fmt.Errorf("unknown service: %s", name)
}

func (i *Iptables) Status() ([]FirewallRule, error) {
	return nil, nil
}

func (i *Iptables) Reset() error {
	_ = run("iptables", "-F")
	_ = run("iptables", "-X")
	return nil
}
