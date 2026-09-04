package engine

import "errors"

var (
	// Directory & Anatomy Errors
	ErrInvalidDirectory  = errors.New("data directory is not a valid directory")
	ErrDirectoryPolluted = errors.New("directory contains existing non-amoeba files")
	ErrInspectDirectory  = errors.New("failed to inspect data directory")
	ErrCreateDirectory   = errors.New("failed to create directory")
	ErrMarkerFile        = errors.New("failed to create marker file")
	ErrCreateVolume      = errors.New("failed to provision volume")
	ErrResolvePath       = errors.New("failed to resolve absolute path")
	ErrPathEscaped       = errors.New("security violation: path traversal detected")

	// Object & Ingestion Errors
	ErrObjectNotFound     = errors.New("object not found")
	ErrEmptyKey           = errors.New("object key cannot be empty")
	ErrEngineClosed       = errors.New("storage engine is shut down")
	ErrAllVolumesFull     = errors.New("all virtual volumes are full")
	ErrObjectLocked       = errors.New("object is locked for modification or deletion")
	ErrChecksumMismatch   = errors.New("integrity check failed: checksum mismatch")
	ErrSizeMismatch       = errors.New("integrity check failed: size mismatch")
	ErrInvalidPayloadSize = errors.New("invalid payload size: must be greater than zero")

	// I/O & Storage Lifecycle Errors
	ErrGenerateID        = errors.New("failed to generate object id")
	ErrFileOpen          = errors.New("failed to open object file")
	ErrStreamWrite       = errors.New("stream write failed")
	ErrDiskSync          = errors.New("failed to sync object to disk")
	ErrSerializeMetadata = errors.New("failed to serialize metadata")
	ErrMetaStackWrite    = errors.New("failed to write meta stack file")
)
