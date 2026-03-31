package provisioner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/getdavit/davit/internal/firewall"
	"github.com/getdavit/davit/internal/osdetect"
)

// --- Step 1: System Update ---

func (p *Provisioner) stepSystemUpdate(opts Options) StepResult {
	start := time.Now()
	if opts.DryRun {
		return stepOK(StepSystemUpdate, "would run package update and upgrade", start)
	}
	if err := p.pm.Update(); err != nil {
		return stepError(StepSystemUpdate, "PROVISION_STEP_FAILED",
			"package index update failed: "+err.Error(), start)
	}
	if err := p.pm.Upgrade(); err != nil {
		return stepError(StepSystemUpdate, "PROVISION_STEP_FAILED",
			"package upgrade failed: "+err.Error(), start)
	}
	return stepOK(StepSystemUpdate, "system packages updated", start)
}

// --- Step 2: Install Core Utilities ---

var corePackages = map[osdetect.PkgMgrKind][]string{
	osdetect.PkgMgrApt:    {"curl", "git", "wget", "gnupg", "ca-certificates", "unzip", "htop", "vim"},
	osdetect.PkgMgrDnf:    {"curl", "git", "wget", "gnupg2", "ca-certificates", "unzip", "htop", "vim"},
	osdetect.PkgMgrYum:    {"curl", "git", "wget", "gnupg2", "ca-certificates", "unzip", "htop", "vim"},
	osdetect.PkgMgrZypper: {"curl", "git", "wget", "gpg2", "ca-certificates", "unzip", "htop", "vim"},
	osdetect.PkgMgrPacman: {"curl", "git", "wget", "gnupg", "ca-certificates", "unzip", "htop", "vim"},
	osdetect.PkgMgrApk:    {"curl", "git", "wget", "gnupg", "ca-certificates", "unzip", "htop", "vim"},
}

func (p *Provisioner) stepCoreUtils(opts Options) StepResult {
	start := time.Now()
	pkgs := corePackages[p.os.PkgMgr]
	if len(pkgs) == 0 {
		return stepSkipped(StepCoreUtils, "no package list for "+string(p.os.PkgMgr))
	}
	if opts.DryRun {
		return stepOK(StepCoreUtils, "would install: "+strings.Join(pkgs, ", "), start)
	}
	if err := p.pm.Install(pkgs...); err != nil {
		return stepError(StepCoreUtils, "PROVISION_STEP_FAILED",
			"install core utils: "+err.Error(), start)
	}
	return stepOK(StepCoreUtils, "core utilities installed", start)
}

// --- Step 3: Timezone ---

func (p *Provisioner) stepTimezone(opts Options) StepResult {
	start := time.Now()
	tz := opts.Timezone
	if tz == "" {
		tz = detectCurrentTZ()
	}
	if opts.DryRun {
		return stepOK(StepTimezone, "would set timezone to "+tz, start)
	}
	if p.os.InitSystem == osdetect.InitSystemd {
		if err := cmdRun("timedatectl", "set-timezone", tz); err != nil {
			return stepError(StepTimezone, "PROVISION_STEP_FAILED",
				"timedatectl set-timezone: "+err.Error(), start)
		}
	} else {
		target := "/usr/share/zoneinfo/" + tz
		if _, err := os.Stat(target); err != nil {
			return stepError(StepTimezone, "PROVISION_STEP_FAILED",
				"timezone file not found: "+target, start)
		}
		_ = os.Remove("/etc/localtime")
		if err := os.Symlink(target, "/etc/localtime"); err != nil {
			return stepError(StepTimezone, "PROVISION_STEP_FAILED",
				"symlink /etc/localtime: "+err.Error(), start)
		}
	}
	_ = p.db.SetSystemInfo("timezone", tz)
	return stepOK(StepTimezone, "timezone set to "+tz, start)
}

func detectCurrentTZ() string {
	data, err := os.ReadFile("/etc/timezone")
	if err == nil {
		if tz := strings.TrimSpace(string(data)); tz != "" {
			return tz
		}
	}
	return "UTC"
}

// --- Step 4: SSH Hardening ---

const sshBlockBegin = "# BEGIN DAVIT MANAGED BLOCK — do not edit manually"
const sshBlockEnd = "# END DAVIT MANAGED BLOCK"

const sshHardeningBlock = `
# BEGIN DAVIT MANAGED BLOCK — do not edit manually
PermitRootLogin no
PasswordAuthentication no
ChallengeResponseAuthentication no
X11Forwarding no
AllowTcpForwarding no
PrintMotd yes
MaxAuthTries 3
LoginGraceTime 30
# END DAVIT MANAGED BLOCK
`

func (p *Provisioner) stepSSHHardening(opts Options) StepResult {
	start := time.Now()
	sshdConfig := "/etc/ssh/sshd_config"

	if _, err := os.Stat(sshdConfig); err != nil {
		return stepSkipped(StepSSHHardening, "sshd_config not found; openssh-server not installed")
	}

	if isSSHBlockPresent(sshdConfig) {
		return stepSkipped(StepSSHHardening, "davit SSH block already present in sshd_config")
	}

	if !hasAuthorizedKey() {
		return stepWarn(StepSSHHardening, "WARN_NO_SSH_KEY",
			"no SSH public key found; skipping password auth hardening to avoid lockout",
			start)
	}

	if opts.DryRun {
		return stepOK(StepSSHHardening, "would append davit managed block to "+sshdConfig, start)
	}

	f, err := os.OpenFile(sshdConfig, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return stepError(StepSSHHardening, "PROVISION_STEP_FAILED",
			"open sshd_config: "+err.Error(), start)
	}
	_, writeErr := f.WriteString(sshHardeningBlock)
	f.Close()
	if writeErr != nil {
		return stepError(StepSSHHardening, "PROVISION_STEP_FAILED",
			"write sshd_config: "+writeErr.Error(), start)
	}

	if err := cmdRun("sshd", "-t"); err != nil {
		removeSSHBlock(sshdConfig)
		return stepError(StepSSHHardening, "SSH_VALIDATION_FAILED",
			"sshd -t validation failed; block removed: "+err.Error(), start)
	}

	if p.os.InitSystem == osdetect.InitSystemd {
		_ = cmdRun("systemctl", "restart", "sshd")
	} else {
		_ = cmdRun("service", "ssh", "restart")
	}

	return stepOK(StepSSHHardening, "SSH hardened and validated", start)
}

func isSSHBlockPresent(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), sshBlockBegin)
}

func hasAuthorizedKey() bool {
	paths := []string{"/root/.ssh/authorized_keys"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, home+"/.ssh/authorized_keys")
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				return true
			}
		}
	}
	return false
}

func removeSSHBlock(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	inBlock := false
	for _, line := range lines {
		if strings.Contains(line, sshBlockBegin) {
			inBlock = true
			continue
		}
		if strings.Contains(line, sshBlockEnd) {
			inBlock = false
			continue
		}
		if !inBlock {
			out = append(out, line)
		}
	}
	_ = os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

// --- Step 5: Firewall ---

func (p *Provisioner) stepFirewall(opts Options) StepResult {
	start := time.Now()
	if opts.DryRun {
		return stepOK(StepFirewall, "would configure firewall (22,80,443/tcp; 443/udp; deny inbound)", start)
	}

	fw, kind, err := firewall.Detect(p.pm)
	if err != nil {
		return stepError(StepFirewall, "PROVISION_STEP_FAILED",
			"detect/install firewall: "+err.Error(), start)
	}

	if err := fw.Enable(); err != nil {
		return stepError(StepFirewall, "PROVISION_STEP_FAILED",
			"enable firewall: "+err.Error(), start)
	}

	// UFW-specific: set default deny inbound, allow outbound
	if u, ok := fw.(*firewall.UFW); ok {
		_ = u.SetDefaultDeny()
		_ = u.SetDefaultAllow()
	}

	rules := []struct {
		port  int
		proto string
	}{
		{22, "tcp"},
		{80, "tcp"},
		{443, "tcp"},
		{443, "udp"},
	}
	for _, r := range rules {
		if err := fw.AllowPort(r.port, r.proto); err != nil {
			return stepError(StepFirewall, "PROVISION_STEP_FAILED",
				fmt.Sprintf("allow %d/%s: %v", r.port, r.proto, err), start)
		}
	}

	_ = p.db.SetSystemInfo("firewall_kind", string(kind))
	return stepOK(StepFirewall, "firewall configured ("+string(kind)+")", start)
}

// --- Step 6: fail2ban ---

const fail2banConfig = `[sshd]
enabled  = true
port     = ssh
filter   = sshd
logpath  = %(sshd_log)s
maxretry = 5
bantime  = 3600
findtime = 600

[caddy-http-auth]
enabled  = false
`

func (p *Provisioner) stepFail2ban(opts Options) StepResult {
	start := time.Now()
	if opts.DryRun {
		return stepOK(StepFail2ban, "would install fail2ban and write davit jail config", start)
	}

	if !p.pm.IsInstalled("fail2ban") {
		if err := p.pm.Install("fail2ban"); err != nil {
			return stepError(StepFail2ban, "PROVISION_STEP_FAILED",
				"install fail2ban: "+err.Error(), start)
		}
	}

	if err := os.MkdirAll("/etc/fail2ban/jail.d", 0755); err != nil {
		return stepError(StepFail2ban, "PROVISION_STEP_FAILED",
			"create jail.d: "+err.Error(), start)
	}

	if err := os.WriteFile("/etc/fail2ban/jail.d/davit.conf", []byte(fail2banConfig), 0644); err != nil {
		return stepError(StepFail2ban, "PROVISION_STEP_FAILED",
			"write davit.conf: "+err.Error(), start)
	}

	if p.os.InitSystem == osdetect.InitSystemd {
		_ = cmdRun("systemctl", "enable", "--now", "fail2ban")
	} else {
		_ = cmdRun("service", "fail2ban", "start")
	}

	return stepOK(StepFail2ban, "fail2ban installed and configured", start)
}

// --- Step 7: Docker ---

func (p *Provisioner) stepDocker(opts Options) StepResult {
	start := time.Now()

	if _, err := exec.LookPath("docker"); err == nil {
		return stepSkipped(StepDocker, "docker already installed")
	}

	if opts.DryRun {
		return stepOK(StepDocker, "would install Docker Engine from official repositories", start)
	}

	switch p.os.PkgMgr {
	case osdetect.PkgMgrApt:
		if err := installDockerApt(p.os.ID); err != nil {
			return stepError(StepDocker, "PROVISION_STEP_FAILED", "install docker (apt): "+err.Error(), start)
		}
	case osdetect.PkgMgrDnf, osdetect.PkgMgrYum:
		if err := installDockerDnf(); err != nil {
			return stepError(StepDocker, "PROVISION_STEP_FAILED", "install docker (dnf): "+err.Error(), start)
		}
	case osdetect.PkgMgrPacman:
		if err := p.pm.Install("docker", "docker-compose"); err != nil {
			return stepError(StepDocker, "PROVISION_STEP_FAILED", "install docker (pacman): "+err.Error(), start)
		}
	case osdetect.PkgMgrApk:
		if err := p.pm.Install("docker", "docker-cli-compose"); err != nil {
			return stepError(StepDocker, "PROVISION_STEP_FAILED", "install docker (apk): "+err.Error(), start)
		}
		_ = cmdRun("rc-update", "add", "docker", "boot")
	default:
		return stepError(StepDocker, "PROVISION_STEP_FAILED",
			"unsupported package manager for docker: "+string(p.os.PkgMgr), start)
	}

	if p.os.InitSystem == osdetect.InitSystemd {
		_ = cmdRun("systemctl", "enable", "--now", "docker")
	} else {
		_ = cmdRun("service", "docker", "start")
	}

	if err := cmdRun("docker", "run", "--rm", "hello-world"); err != nil {
		return stepError(StepDocker, "PROVISION_STEP_FAILED",
			"docker hello-world verification failed: "+err.Error(), start)
	}

	return stepOK(StepDocker, "Docker Engine installed and verified", start)
}

func installDockerApt(distroID string) error {
	cmds := [][]string{
		{"apt-get", "install", "-y", "-qq", "debian-keyring", "debian-archive-keyring", "apt-transport-https", "curl"},
		{"bash", "-c", "curl -fsSL https://download.docker.com/linux/" + distroID + "/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg"},
	}
	for _, cmd := range cmds {
		if err := cmdRun(cmd[0], cmd[1:]...); err != nil {
			return err
		}
	}

	arch, _ := cmdOutput("dpkg", "--print-architecture")
	arch = strings.TrimSpace(arch)
	codename, _ := cmdOutput("bash", "-c", ". /etc/os-release && echo $VERSION_CODENAME")
	codename = strings.TrimSpace(codename)

	if err := os.MkdirAll("/etc/apt/keyrings", 0755); err != nil {
		return err
	}
	repo := fmt.Sprintf(
		"deb [arch=%s signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/%s %s stable\n",
		arch, distroID, codename)
	if err := os.WriteFile("/etc/apt/sources.list.d/docker.list", []byte(repo), 0644); err != nil {
		return err
	}
	if err := cmdRun("apt-get", "update", "-qq"); err != nil {
		return err
	}
	return cmdRun("apt-get", "install", "-y", "-qq",
		"docker-ce", "docker-ce-cli", "containerd.io",
		"docker-buildx-plugin", "docker-compose-plugin")
}

func installDockerDnf() error {
	if err := cmdRun("dnf", "config-manager", "--add-repo",
		"https://download.docker.com/linux/fedora/docker-ce.repo"); err != nil {
		return err
	}
	return cmdRun("dnf", "install", "-y", "-q",
		"docker-ce", "docker-ce-cli", "containerd.io",
		"docker-buildx-plugin", "docker-compose-plugin")
}

// --- Step 8: Caddy ---

func (p *Provisioner) stepCaddy(opts Options) StepResult {
	start := time.Now()

	if _, err := exec.LookPath("caddy"); err == nil {
		return stepSkipped(StepCaddy, "caddy already installed")
	}

	if opts.DryRun {
		return stepOK(StepCaddy, "would install Caddy from official repositories", start)
	}

	switch p.os.PkgMgr {
	case osdetect.PkgMgrApt:
		if err := installCaddyApt(); err != nil {
			return stepError(StepCaddy, "PROVISION_STEP_FAILED", "install caddy (apt): "+err.Error(), start)
		}
	case osdetect.PkgMgrDnf, osdetect.PkgMgrYum:
		if err := installCaddyDnf(); err != nil {
			return stepError(StepCaddy, "PROVISION_STEP_FAILED", "install caddy (dnf): "+err.Error(), start)
		}
	case osdetect.PkgMgrPacman:
		if err := p.pm.Install("caddy"); err != nil {
			return stepError(StepCaddy, "PROVISION_STEP_FAILED", "install caddy (pacman): "+err.Error(), start)
		}
	case osdetect.PkgMgrApk:
		if err := p.pm.Install("caddy"); err != nil {
			return stepError(StepCaddy, "PROVISION_STEP_FAILED", "install caddy (apk): "+err.Error(), start)
		}
	default:
		return stepError(StepCaddy, "PROVISION_STEP_FAILED",
			"unsupported package manager for caddy: "+string(p.os.PkgMgr), start)
	}

	email := opts.Email
	if email == "" {
		email = "admin@example.com"
	}
	caddyFile := fmt.Sprintf("{\n    admin localhost:2019\n    email %s\n}\n", email)
	if err := os.MkdirAll("/etc/caddy", 0755); err != nil {
		return stepError(StepCaddy, "PROVISION_STEP_FAILED", "mkdir /etc/caddy: "+err.Error(), start)
	}
	if err := os.WriteFile("/etc/caddy/Caddyfile", []byte(caddyFile), 0644); err != nil {
		return stepError(StepCaddy, "PROVISION_STEP_FAILED", "write Caddyfile: "+err.Error(), start)
	}

	if p.os.InitSystem == osdetect.InitSystemd {
		_ = cmdRun("systemctl", "enable", "--now", "caddy")
	} else {
		_ = cmdRun("service", "caddy", "start")
	}

	return stepOK(StepCaddy, "Caddy installed and started", start)
}

func installCaddyApt() error {
	steps := [][]string{
		{"apt-get", "install", "-y", "-qq", "debian-keyring", "debian-archive-keyring", "apt-transport-https", "curl"},
		{"bash", "-c", "curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg"},
		{"bash", "-c", "curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list"},
		{"apt-get", "update", "-qq"},
		{"apt-get", "install", "-y", "-qq", "caddy"},
	}
	for _, s := range steps {
		if err := cmdRun(s[0], s[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func installCaddyDnf() error {
	if err := cmdRun("dnf", "install", "-y", "-q", "dnf-command(copr)"); err != nil {
		return err
	}
	if err := cmdRun("dnf", "copr", "enable", "-y", "@caddy/caddy"); err != nil {
		return err
	}
	return cmdRun("dnf", "install", "-y", "-q", "caddy")
}

// --- Step 9: Directory Structure ---

var davitDirs = []struct {
	path string
	mode os.FileMode
}{
	{"/etc/davit", 0755},
	{"/var/lib/davit", 0755},
	{"/var/lib/davit/apps", 0755},
	{"/var/log/davit", 0755},
	{"/run/davit", 0755},
}

func (p *Provisioner) stepDirStructure(opts Options) StepResult {
	start := time.Now()
	if opts.DryRun {
		return stepOK(StepDirStructure, "would create /etc/davit, /var/lib/davit, /var/log/davit, /run/davit", start)
	}
	for _, d := range davitDirs {
		if err := os.MkdirAll(d.path, d.mode); err != nil {
			return stepError(StepDirStructure, "PROVISION_STEP_FAILED",
				"mkdir "+d.path+": "+err.Error(), start)
		}
	}
	motd := "╔══════════════════════════════════════════════╗\n" +
		"║          This server is managed by Davit     ║\n" +
		"║          Run: davit tui   to get started     ║\n" +
		"╚══════════════════════════════════════════════╝\n"
	if err := os.MkdirAll("/etc/motd.d", 0755); err == nil {
		_ = os.WriteFile("/etc/motd.d/davit", []byte(motd), 0644)
	}
	return stepOK(StepDirStructure, "davit directories created", start)
}

// --- Step 10: DB Init ---

func (p *Provisioner) stepDBInit(opts Options) StepResult {
	start := time.Now()
	if opts.DryRun {
		return stepOK(StepDBInit, "would initialise SQLite state database", start)
	}
	if err := p.db.SetSystemInfo("db_initialized", "true"); err != nil {
		return stepError(StepDBInit, "STATE_DB_ERROR", "set system_info: "+err.Error(), start)
	}
	return stepOK(StepDBInit, "state database initialised", start)
}

// --- Step 11: Daemon systemd unit ---

const systemdUnit = `[Unit]
Description=Davit Git Watcher and Webhook Receiver
After=network.target docker.service caddy.service

[Service]
Type=simple
ExecStart=/usr/local/bin/davit daemon start
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=davit-daemon

[Install]
WantedBy=multi-user.target
`

func (p *Provisioner) stepDaemonUnit(opts Options) StepResult {
	start := time.Now()
	if opts.DryRun {
		return stepOK(StepDaemonUnit, "would install davit-daemon systemd unit", start)
	}
	if p.os.InitSystem != osdetect.InitSystemd {
		return stepSkipped(StepDaemonUnit, "systemd not present; daemon unit skipped")
	}
	unitPath := "/etc/systemd/system/davit-daemon.service"
	if err := os.WriteFile(unitPath, []byte(systemdUnit), 0644); err != nil {
		return stepError(StepDaemonUnit, "PROVISION_STEP_FAILED",
			"write systemd unit: "+err.Error(), start)
	}
	_ = cmdRun("systemctl", "daemon-reload")
	_ = cmdRun("systemctl", "enable", "davit-daemon")
	return stepOK(StepDaemonUnit, "davit-daemon systemd unit installed", start)
}

// --- internal helpers ---

func cmdRun(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w\n%s", name, err, buf.String())
	}
	return nil
}

func cmdOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	return buf.String(), cmd.Run()
}
