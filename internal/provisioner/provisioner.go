// Package provisioner implements the davit server provisioning sequence.
package provisioner

import (
	"time"

	"github.com/getdavit/davit/internal/osdetect"
	"github.com/getdavit/davit/internal/pkgmgr"
	"github.com/getdavit/davit/internal/state"
)

// StepName identifies a provisioning step.
type StepName string

const (
	StepSystemUpdate StepName = "system_update"
	StepCoreUtils    StepName = "install_core_utils"
	StepTimezone     StepName = "configure_timezone"
	StepSSHHardening StepName = "ssh_hardening"
	StepFirewall     StepName = "configure_firewall"
	StepFail2ban     StepName = "install_fail2ban"
	StepDocker       StepName = "install_docker"
	StepCaddy        StepName = "install_caddy"
	StepDirStructure StepName = "create_dir_structure"
	StepDBInit       StepName = "init_state_db"
	StepDaemonUnit   StepName = "install_daemon_unit"
)

// allSteps lists steps in execution order.
var allSteps = []StepName{
	StepSystemUpdate,
	StepCoreUtils,
	StepTimezone,
	StepSSHHardening,
	StepFirewall,
	StepFail2ban,
	StepDocker,
	StepCaddy,
	StepDirStructure,
	StepDBInit,
	StepDaemonUnit,
}

// StepResult is the structured output for one provisioning step.
type StepResult struct {
	Step       StepName `json:"step"`
	Status     string   `json:"status"` // ok|skipped|warn|error
	Message    string   `json:"message"`
	ErrorCode  string   `json:"error_code,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

// Options controls the provisioning run.
type Options struct {
	Timezone  string
	Email     string
	SkipSteps []StepName
	DryRun    bool
}

// Result is the overall summary returned after provisioning.
type Result struct {
	Status       string       `json:"status"`
	StepsTotal   int          `json:"steps_total"`
	StepsOK      int          `json:"steps_ok"`
	StepsSkipped int          `json:"steps_skipped"`
	StepsFailed  int          `json:"steps_failed"`
	DurationMS   int64        `json:"duration_ms"`
	Steps        []StepResult `json:"steps"`
}

// Provisioner orchestrates the provisioning sequence.
type Provisioner struct {
	db   *state.DB
	os   *osdetect.Profile
	pm   pkgmgr.PackageManager
	emit func(StepResult)
}

// New creates a Provisioner. emit is called for each step result as it
// completes (for streaming output). Pass nil to suppress streaming.
func New(db *state.DB, profile *osdetect.Profile, pm pkgmgr.PackageManager, emit func(StepResult)) *Provisioner {
	if emit == nil {
		emit = func(StepResult) {}
	}
	return &Provisioner{db: db, os: profile, pm: pm, emit: emit}
}

// Run executes the provisioning sequence and returns a summary.
func (p *Provisioner) Run(opts Options) (Result, error) {
	start := time.Now()
	skip := make(map[StepName]bool)
	for _, s := range opts.SkipSteps {
		skip[s] = true
	}

	stepFns := map[StepName]func(Options) StepResult{
		StepSystemUpdate: p.stepSystemUpdate,
		StepCoreUtils:    p.stepCoreUtils,
		StepTimezone:     p.stepTimezone,
		StepSSHHardening: p.stepSSHHardening,
		StepFirewall:     p.stepFirewall,
		StepFail2ban:     p.stepFail2ban,
		StepDocker:       p.stepDocker,
		StepCaddy:        p.stepCaddy,
		StepDirStructure: p.stepDirStructure,
		StepDBInit:       p.stepDBInit,
		StepDaemonUnit:   p.stepDaemonUnit,
	}

	res := Result{StepsTotal: len(allSteps)}

	for _, name := range allSteps {
		if skip[name] {
			sr := StepResult{Step: name, Status: "skipped", Message: "skipped by --skip-steps"}
			res.Steps = append(res.Steps, sr)
			res.StepsSkipped++
			p.emit(sr)
			continue
		}

		fn := stepFns[name]
		sr := fn(opts)
		res.Steps = append(res.Steps, sr)
		p.emit(sr)

		switch sr.Status {
		case "ok", "skipped", "warn":
			res.StepsOK++
		case "error":
			res.StepsFailed++
			res.Status = "error"
			res.DurationMS = time.Since(start).Milliseconds()
			return res, nil
		}
	}

	res.DurationMS = time.Since(start).Milliseconds()
	if res.Status == "" {
		res.Status = "ok"
	}

	if res.Status == "ok" && !opts.DryRun {
		_ = p.db.SetSystemInfo("provisioned_at", time.Now().UTC().Format(time.RFC3339))
		_ = p.db.SetSystemInfo("admin_email", opts.Email)
	}

	return res, nil
}

func ms(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

func stepOK(name StepName, msg string, start time.Time) StepResult {
	return StepResult{Step: name, Status: "ok", Message: msg, DurationMS: ms(start)}
}

func stepSkipped(name StepName, msg string) StepResult {
	return StepResult{Step: name, Status: "skipped", Message: msg}
}

func stepWarn(name StepName, code, msg string, start time.Time) StepResult {
	return StepResult{Step: name, Status: "warn", ErrorCode: code, Message: msg, DurationMS: ms(start)}
}

func stepError(name StepName, code, msg string, start time.Time) StepResult {
	return StepResult{Step: name, Status: "error", ErrorCode: code, Message: msg, DurationMS: ms(start)}
}
