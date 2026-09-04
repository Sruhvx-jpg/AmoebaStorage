package engine

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	rootDir        string
	volumeDir      string
	metadataDir    string
	shardingEnable bool
	retriesLimit   int

	// ghost conf
	// streamBufferSize int

	volMu     sync.RWMutex
	volume    []*VirtualVolume
	volumeidx uint64

	metaMu      sync.Mutex
	activeStack string
	index       map[string]*MetadataEntry

	wg sync.WaitGroup
}

// AmoebaBelly prepares the physical disk anatomy for the engine.
//
// The flow: Check if the directory exists. If it exists, check if it has empty
// storage and continue. If it doesn't exist, create one.
//
// Also guards against the edge case where an existing directory contains foreign
// files that have the exact same name as the sub-directories we want to create.
// Uses the .amoeba marker file so existing Amoeba stores mount safely on reboot
// while rejecting polluted foreign directories to prevent collisions.
//
// Once validated, it provisions the metadata/ directory and the volumes/ species
// directories (proteus, dubia, chaos...), returning the initialized VirtualVolume slice.
func AmoebaBelly(absRoot string, virtualVolumesCap int, virtualVolumeSizeCap int64) ([]*VirtualVolume, error) {
	info, err := os.Stat(absRoot)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %w", ErrInspectDirectory, err)
		}

		// Directory doesn't exist yet: create one fresh
		if err := os.MkdirAll(absRoot, 0o755); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrCreateDirectory, err)
		}
	} else {
		// Directory exists: must be a real directory, not a plain file
		if !info.IsDir() {
			return nil, ErrInvalidDirectory
		}

		// Read contents to guard against collisions with our sub-directories
		entries, err := os.ReadDir(absRoot)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInspectDirectory, err)
		}

		hasMarker := false
		hasRealFiles := false

		for _, entry := range entries {
			name := entry.Name()
			if name == markerFileName {
				hasMarker = true
				continue
			}
			// Ignore OS metadata files
			if name == ".DS_Store" || name == ".gitkeep" {
				continue
			}
			hasRealFiles = true
		}

		// If foreign files exist without our marker, reject to prevent collisions!
		if hasRealFiles && !hasMarker {
			return nil, fmt.Errorf("%w: %s contains foreign files", ErrDirectoryPolluted, absRoot)
		}
	}

	// Touch the .amoeba marker file to claim ownership of the belly
	markerPath := filepath.Join(absRoot, markerFileName)
	markerFile, err := os.OpenFile(markerPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMarkerFile, err)
	}
	markerFile.Close()

	// 1. Provision the metadata/ directory
	metadataDir := filepath.Join(absRoot, metadatDirName)
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateDirectory, err)
	}

	// 2. Provision the volumes/ directory
	volumesDir := filepath.Join(absRoot, volumesDirname)
	if err := os.MkdirAll(volumesDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateDirectory, err)
	}

	// 3. Provision each species volume from the roster
	var provisionedVolumes []*VirtualVolume
	maxBytesPerVol := virtualVolumeSizeCap * 1024 * 1024 // Convert MB to bytes

	// FUTURE MARK - 1
	// This code is marked to changed in future, as it is just for
	// virtual logical testing, also in future the volume folder
	// will have a refernce to the actually vols
	for i := 0; i < virtualVolumesCap; i++ {
		speciesName := SpeciesRoster[i%len(SpeciesRoster)]
		volPath := filepath.Join(volumesDir, speciesName)

		if err := os.MkdirAll(volPath, 0o755); err != nil {
			return nil, fmt.Errorf("%w %s: %w", ErrCreateVolume, speciesName, err)
		}

		provisionedVolumes = append(provisionedVolumes, &VirtualVolume{
			Name:      speciesName,
			Path:      volPath,
			MaxBytes:  maxBytesPerVol,
			UsedBytes: 0,
			IsFull:    false,
		})
	}

	return provisionedVolumes, nil
}

func New(rootDir string, shardingEnable bool, retriesLimit int, virtualVolumesCap int, virtualVolumeSizeCap int64) (*Engine, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolvePath, err)
	}

	volumes, err := AmoebaBelly(absRoot, virtualVolumesCap, virtualVolumeSizeCap)
	if err != nil {
		return nil, err
	}

	if retriesLimit <= 0 {
		retriesLimit = 3
	}

	return &Engine{
		rootDir:        absRoot,
		volumeDir:      filepath.Join(absRoot, volumesDirname),
		metadataDir:    filepath.Join(absRoot, metadatDirName),
		shardingEnable: shardingEnable,
		retriesLimit:   retriesLimit,
		volume:         volumes,
		volumeidx:      0,
		index:          make(map[string]*MetadataEntry),
	}, nil
}

func (e *Engine) assignVol(size int64) (*VirtualVolume, error) {
	e.volMu.Lock()
	defer e.volMu.Unlock()

	totalVol := len(e.volume)
	if totalVol == 0 {
		return nil, ErrAllVolumesFull
	}

	currVolIdx := e.volumeidx
	for nextIdx := 0; nextIdx < totalVol; nextIdx++ {
		destinationVolIdx := (currVolIdx + uint64(nextIdx)) % uint64(totalVol) // reset volume index to 0, if the volume index is out of bound
		vol := e.volume[destinationVolIdx]                                     // update the next index to be selected

		if vol.IsFull || (vol.UsedBytes+size > vol.MaxBytes) {
			continue
		}

		vol.UsedBytes += size
		if vol.UsedBytes >= vol.MaxBytes {
			vol.IsFull = true
		}

		e.volumeidx = (destinationVolIdx + 1) % uint64(totalVol)
		return vol, nil
	}

	return nil, ErrAllVolumesFull
}

// eatObject engulfs the stream, writes it directly into the species volume,
// computes CRC32C, verifies integrity, and returns only the essential values.
func (e *Engine) eatObject(vol *VirtualVolume, size int64, stream io.Reader, expectedHash uint32) (string, int64, uint32, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", 0, 0, fmt.Errorf("%w: %w", ErrGenerateID, err)
	}
	objectID := hex.EncodeToString(idBytes)
	objectPath := filepath.Join(vol.Path, objectID)

	file, err := os.OpenFile(objectPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", 0, 0, fmt.Errorf("%w: %w", ErrFileOpen, err)
	}
	defer file.Close()

	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	pipeDest := io.MultiWriter(file, hasher)

	buf := make([]byte, defaultStreamBufferSize)
	written, err := io.CopyBuffer(pipeDest, stream, buf)
	if err != nil {
		os.Remove(objectPath)
		return "", 0, 0, fmt.Errorf("%w: %w", ErrStreamWrite, err)
	}

	if err := file.Sync(); err != nil {
		os.Remove(objectPath)
		return "", 0, 0, fmt.Errorf("%w: %w", ErrDiskSync, err)
	}

	if written != size {
		os.Remove(objectPath)
		return "", 0, 0, ErrSizeMismatch
	}

	computedCRC := hasher.Sum32()
	if expectedHash != 0 && computedCRC != expectedHash {
		os.Remove(objectPath)
		return "", 0, 0, ErrChecksumMismatch
	}

	return objectID, written, computedCRC, nil
}

// registerObject writes the persistent stack.<UUID>.meta journal file in data/metadata/
// with retry backoff to guarantee metadata durability, and registers the entry in the in-memory index.
func (e *Engine) registerObject(vol *VirtualVolume, key string, objectID string, size int64, crc uint32) (string, error) {
	objectAddress := vol.Name + "/" + objectID

	entry := &MetadataEntry{
		ID:                objectID,
		Key:               key,
		ObjectPointer:     objectAddress,
		FileSize:          size,
		ChecksumCRC32:     crc,
		IntegrityVerified: true,
		LockStatus:        false,
		CreatedAt:         time.Now().UTC(),
	}

	metaBytes, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSerializeMetadata, err)
	}

	metaFileName := fmt.Sprintf("stack.%s.meta", objectID)
	metaFilePath := filepath.Join(e.metadataDir, metaFileName)

	maxRetries := e.retriesLimit
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := func() error {
			metaFile, err := os.OpenFile(metaFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			defer metaFile.Close()

			if _, err := metaFile.Write(metaBytes); err != nil {
				os.Remove(metaFilePath)
				return err
			}

			if err := metaFile.Sync(); err != nil {
				os.Remove(metaFilePath)
				return err
			}

			return nil
		}()

		if err == nil {
			// Commit to fast in-memory index
			e.metaMu.Lock()
			e.index[key] = entry
			e.metaMu.Unlock()
			return objectAddress, nil
		}

		lastErr = err
		// Exponential/linear backoff: 25ms, 50ms, 75ms...
		time.Sleep(time.Duration(attempt*25) * time.Millisecond)
	}

	return "", fmt.Errorf("%w after %d attempts: %w", ErrMetaStackWrite, maxRetries, lastErr)
}

func (e *Engine) Ingest(key string, size int64, stream io.Reader, expectedCRC32 uint32) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", ErrEmptyKey
	}

	if size <= 0 {
		return "", ErrInvalidPayloadSize
	}

	if stream == nil {
		return "", ErrObjectNotFound
	}

	destinationVol, err := e.assignVol(size)
	if err != nil {
		return "", err
	}

	e.wg.Add(1)
	defer e.wg.Done()

	objectID, written, computedCRC, err := e.eatObject(destinationVol, size, stream, expectedCRC32)
	if err != nil {
		e.volMu.Lock()
		destinationVol.UsedBytes -= size
		if destinationVol.UsedBytes < destinationVol.MaxBytes {
			destinationVol.IsFull = false
		}
		e.volMu.Unlock()
		return "", err
	}

	addr, err := e.registerObject(destinationVol, key, objectID, written, computedCRC)
	if err != nil {
		// Clean up the written object if metadata registration fails
		os.Remove(filepath.Join(destinationVol.Path, objectID))

		e.volMu.Lock()
		destinationVol.UsedBytes -= size
		if destinationVol.UsedBytes < destinationVol.MaxBytes {
			destinationVol.IsFull = false
		}
		e.volMu.Unlock()
		return "", err
	}

	return addr, nil
}
