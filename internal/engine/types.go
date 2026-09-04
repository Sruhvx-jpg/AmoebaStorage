package engine

import (
	"hash"
	"io"
	"time"
)

const (
	markerFileName   = ".amoeba"
	volumesDirname   = "volumes"
	metadatDirName   = "metadata"
	currentStackFile = "CURRENT"
)

var SpeciesRoster = []string{
	"proteus",
	"dubia",
	"chaos",
	"radiosa",
	"discoides",
	"vespertilio",
	"terricola",
	"limax",
}

// Various types for States

type VirtualVolume struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	MaxBytes  int64  `json:"max_bytes"`
	UsedBytes int64  `json:"used_bytes"`
	IsFull    bool   `json:"is_full"`
}

type MetadataEntry struct {
	ID                string    `json:"id"`
	Key               string    `json:"key"`
	ObjectPointer     string    `json:"object_pointer"`
	FileSize          int64     `json:"file_size"`
	ChecksumCRC32     uint32    `json:"checksum_crc32"`
	ReplicaID         int       `json:"replica_id"`         // 0 means no replice id or replication is off
	Autokill          bool      `json:"auto_cleanup"`       // false means file will not be auto killed
	IntegrityVerified bool      `json:"integrity_verified"` // Checked at end of ingestion pipeline
	LockStatus        bool      `json:"lock_status"`        // lock this file metadata entry if we are deleting it, keep default false
	CreatedAt         time.Time `json:"created_at"`
}

type UploadSession struct {
	ID            string
	Key           string
	VolumeSpecies *VirtualVolume
	ExpectedSize  int64
	ExpectedCRC32 uint32
	BytesWritten  int64
	Hasher        hash.Hash32
	TempPath      string
	FinalPath     string
	Autokill      bool
	LockStatus    bool
	StartTime     time.Time
	EndTime       time.Time
	ExpiresAt     time.Time
}

type UploadResult struct {
	Success bool
	Address string
	Err     error
}

type UploadTask struct {
	Key           string
	Size          int64
	Volume        *VirtualVolume
	Stream        io.Reader
	ExpectedCRC32 uint32
	Done          chan UploadResult
}
