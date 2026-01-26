package a2a

import "errors"

// Agent Card validation errors.
var (
	// ErrMissingName indicates the agent card is missing a name.
	ErrMissingName = errors.New("agent card: missing name")
	// ErrMissingDescription indicates the agent card is missing a description.
	ErrMissingDescription = errors.New("agent card: missing description")
	// ErrMissingURL indicates the agent card is missing a URL.
	ErrMissingURL = errors.New("agent card: missing url")
	// ErrMissingVersion indicates the agent card is missing a version.
	ErrMissingVersion = errors.New("agent card: missing version")
)

// A2A protocol errors.
var (
	// ErrAgentNotFound indicates the requested agent was not found.
	ErrAgentNotFound = errors.New("a2a: agent not found")
	// ErrRemoteUnavailable indicates the remote agent is unavailable.
	ErrRemoteUnavailable = errors.New("a2a: remote agent unavailable")
	// ErrAuthFailed indicates authentication failed.
	ErrAuthFailed = errors.New("a2a: authentication failed")
	// ErrInvalidMessage indicates the message format is invalid.
	ErrInvalidMessage = errors.New("a2a: invalid message format")
)

// A2A message validation errors.
var (
	// ErrMessageMissingID indicates the message is missing an ID.
	ErrMessageMissingID = errors.New("a2a message: missing id")
	// ErrMessageInvalidType indicates the message has an invalid type.
	ErrMessageInvalidType = errors.New("a2a message: invalid type")
	// ErrMessageMissingFrom indicates the message is missing a sender.
	ErrMessageMissingFrom = errors.New("a2a message: missing from")
	// ErrMessageMissingTo indicates the message is missing a recipient.
	ErrMessageMissingTo = errors.New("a2a message: missing to")
	// ErrMessageMissingTimestamp indicates the message is missing a timestamp.
	ErrMessageMissingTimestamp = errors.New("a2a message: missing timestamp")
)

// A2A client errors.
var (
	// ErrTaskNotReady indicates the async task is still processing.
	ErrTaskNotReady = errors.New("a2a: task not ready")
	// ErrTaskNotFound indicates the task was not found.
	ErrTaskNotFound = errors.New("a2a: task not found")
)
