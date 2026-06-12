package state

import "time"

// App represents a registered application in the state store.
type App struct {
	Name          string
	RepoURL       string
	Branch        string
	Domain        string
	ContainerPort int
	InternalPort  int
	ComposeFile   string
	BuildContext  string
	DeployKeyPath string
	Status        string // created|running|stopped|removed
	CreatedAt     time.Time
	RemovedAt     *time.Time

	// Watch settings for Git automation
	WatchEnabled      bool
	WatchPollInterval int    // seconds, 0 = disabled
	WatchUseWebhook   bool   // if true, use webhook instead of polling
	LastCommitSHA     string // last seen commit SHA
	LastCheckedAt     time.Time // last time we checked for changes
}

// Deployment is a record of a single deploy operation.
type Deployment struct {
	ID            int64
	AppName       string
	CommitSHA     string
	CommitMessage string
	Status        string // ok|failed
	ErrorCode     string
	DurationMS    int64
	DeployedAt    time.Time
}

// AgentKey is an SSH key registered for agent access.
type AgentKey struct {
	Fingerprint string
	Label       string
	PublicKey   string
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

// EnvVar is a stored (encrypted) environment variable for an app.
type EnvVar struct {
	AppName   string
	Key       string
	UpdatedAt time.Time
}

// OperationLog is a single entry in the append-only audit log.
type OperationLog struct {
	ID         int64
	Operation  string
	Subject    string
	Status     string
	ErrorCode  string
	Message    string
	InvokedBy  string
	DurationMS int64
	LoggedAt   time.Time
}