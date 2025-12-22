package storage

import (
	"github.com/rs/zerolog/log"
)

var storageInstance Storage

// NewStorage creates a new storage instance based on the mode and configuration
func NewStorage(mode StorageMode, databaseURL string) (Storage, error) {
	if storageInstance != nil {
		return storageInstance, nil
	}

	var err error

	// Determine effective storage mode
	// If database mode is requested but no DATABASE_URL, fall back to memory
	effectiveMode := mode
	if mode == StorageModeDatabase && databaseURL == "" {
		log.Warn().Msg("STORAGE_MODE=database but DATABASE_URL is not set, falling back to memory mode")
		effectiveMode = StorageModeMemory
	}

	switch effectiveMode {
	case StorageModeDatabase:
		log.Info().Msg("Initializing database storage (PostgreSQL + TimescaleDB)")
		storageInstance, err = NewDatabaseStorage(databaseURL)
		if err != nil {
			log.Error().Err(err).Msg("Failed to initialize database storage, falling back to memory mode")
			storageInstance = NewMemoryStorage()
		}
	default:
		storageInstance = NewMemoryStorage()
	}

	return storageInstance, nil
}

// GetStorage returns the current storage instance
func GetStorage() Storage {
	return storageInstance
}

// CloseStorage closes the storage connection
func CloseStorage() error {
	if storageInstance != nil {
		return storageInstance.Close()
	}
	return nil
}

