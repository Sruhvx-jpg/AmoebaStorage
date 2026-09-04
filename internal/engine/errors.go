package engine

import "errors"

var (
	ErrInvalidDirectory  = errors.New("data directory is not a valid directory")
	ErrDirectoryPolluted = errors.New("directory contains existing non-amoeba files")
	ErrPathEscaped       = errors.New("security violation: path traversal detected")
	ErrObjectNotFound    = errors.New("object not found")
	ErrEmptyKey           = errors.New("object key cannot be empty")
	ErrEngineClosed       = errors.New("storage engine is shut down")
	ErrAllVolumesFull     = errors.New("all virtual volumes are full")
	ErrObjectLocked       = errors.New("object is locked for modification or deletion")
	ErrChecksumMismatch   = errors.New("integrity check failed: checksum mismatch")
	ErrSizeMismatch       = errors.New("integrity check failed: size mismatch")
	ErrInvalidPayloadSize = errors.New("invalid payload size: must be greater than zero")
)
