package output

// ErrorCode is a machine-readable error identifier (SCREAMING_SNAKE_CASE).
type ErrorCode string

const (
	ErrUnknownCommand      ErrorCode = "UNKNOWN_COMMAND"
	ErrAgentForbidden      ErrorCode = "AGENT_FORBIDDEN"
	ErrUnsupportedOS       ErrorCode = "UNSUPPORTED_OS"
	ErrProvisionStepFailed ErrorCode = "PROVISION_STEP_FAILED"
	ErrSSHValidationFailed ErrorCode = "SSH_VALIDATION_FAILED"
	ErrWarnNoSSHKey        ErrorCode = "WARN_NO_SSH_KEY"
	ErrAppAlreadyExists    ErrorCode = "APP_ALREADY_EXISTS"
	ErrAppNotFound         ErrorCode = "APP_NOT_FOUND"
	ErrAppTypeUnknown      ErrorCode = "APP_TYPE_UNKNOWN"
	ErrGitUnreachable      ErrorCode = "GIT_UNREACHABLE"
	ErrGitCloneFailed      ErrorCode = "GIT_CLONE_FAILED"
	ErrGitPullFailed       ErrorCode = "GIT_PULL_FAILED"
	ErrDockerBuildFailed   ErrorCode = "DOCKER_BUILD_FAILED"
	ErrDockerStartFailed   ErrorCode = "DOCKER_START_FAILED"
	ErrContainerExited     ErrorCode = "CONTAINER_EXITED"
	ErrHealthCheckTimeout  ErrorCode = "HEALTH_CHECK_TIMEOUT"
	ErrPortExhausted       ErrorCode = "PORT_EXHAUSTED"
	ErrCaddyAPIUnreachable ErrorCode = "CADDY_API_UNREACHABLE"
	ErrCaddyConfigFailed   ErrorCode = "CADDY_CONFIG_FAILED"
	ErrCertIssueFailed     ErrorCode = "CERT_ISSUE_FAILED"
	ErrStateDBError        ErrorCode = "STATE_DB_ERROR"
	ErrConfigInvalid       ErrorCode = "CONFIG_INVALID"
	ErrInternalError       ErrorCode = "INTERNAL_ERROR"
)

// ExitCode returns the OS exit code for a given error code.
func ExitCode(code ErrorCode) int {
	switch code {
	case ErrUnknownCommand:
		return 1
	case ErrAgentForbidden:
		return 2
	case ErrUnsupportedOS:
		return 10
	case ErrProvisionStepFailed:
		return 11
	case ErrSSHValidationFailed:
		return 12
	case ErrWarnNoSSHKey:
		return 13
	case ErrAppAlreadyExists:
		return 20
	case ErrAppNotFound:
		return 21
	case ErrAppTypeUnknown:
		return 22
	case ErrGitUnreachable:
		return 23
	case ErrGitCloneFailed:
		return 24
	case ErrGitPullFailed:
		return 25
	case ErrDockerBuildFailed:
		return 30
	case ErrDockerStartFailed:
		return 31
	case ErrContainerExited:
		return 32
	case ErrHealthCheckTimeout:
		return 33
	case ErrPortExhausted:
		return 34
	case ErrCaddyAPIUnreachable:
		return 40
	case ErrCaddyConfigFailed:
		return 41
	case ErrCertIssueFailed:
		return 42
	case ErrStateDBError:
		return 50
	case ErrConfigInvalid:
		return 51
	default:
		return 99
	}
}

// DavitError is the structured error type returned by all Davit operations.
type DavitError struct {
	Code    ErrorCode      `json:"error_code"`
	Message string         `json:"message"`
	Context map[string]any `json:"context,omitempty"`
	DocsURL string         `json:"docs_url,omitempty"`
}

func (e *DavitError) Error() string {
	return string(e.Code) + ": " + e.Message
}

// NewError constructs a DavitError.
func NewError(code ErrorCode, message string, ctx map[string]any) *DavitError {
	return &DavitError{
		Code:    code,
		Message: message,
		Context: ctx,
		DocsURL: "https://github.com/getdavit/davit/wiki/errors/" + string(code),
	}
}
