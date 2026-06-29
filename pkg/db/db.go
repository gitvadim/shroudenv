package db

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"shroudenv/pkg/crypto"

	"github.com/gofrs/flock"
)

type EncryptedPayload struct {
	IV         string `json:"iv"`
	Ciphertext string `json:"ciphertext"`
}

type Environment struct {
	Name    string            `json:"name"`
	Secrets *EncryptedPayload `json:"secrets,omitempty"`
}

type Project struct {
	Name         string        `json:"name"`
	Environments []Environment `json:"environments"`
}

type Database struct {
	Salt     string    `json:"salt"` // Hex-encoded salt
	Projects []Project `json:"projects"`
}

// GetProject returns a pointer to the project if it exists.
func (db *Database) GetProject(name string) *Project {
	nameLower := strings.ToLower(name)
	for i := range db.Projects {
		if strings.ToLower(db.Projects[i].Name) == nameLower {
			return &db.Projects[i]
		}
	}
	return nil
}

// CreateProject adds a new project to the database if it doesn't already exist.
func (db *Database) CreateProject(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("project name cannot be empty")
	}
	if db.GetProject(name) != nil {
		return errors.New("project already exists")
	}
	db.Projects = append(db.Projects, Project{
		Name:         name,
		Environments: []Environment{},
	})
	return nil
}

// GetEnvironment returns a pointer to the environment if it exists inside the project.
func (p *Project) GetEnvironment(name string) *Environment {
	nameLower := strings.ToLower(name)
	for i := range p.Environments {
		if strings.ToLower(p.Environments[i].Name) == nameLower {
			return &p.Environments[i]
		}
	}
	return nil
}

// CreateEnvironment adds a new environment to the project.
func (db *Database) CreateEnvironment(projectName string, envName string) error {
	p := db.GetProject(projectName)
	if p == nil {
		return errors.New("project not found")
	}
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return errors.New("environment name cannot be empty")
	}
	if p.GetEnvironment(envName) != nil {
		return errors.New("environment already exists in this project")
	}
	p.Environments = append(p.Environments, Environment{
		Name:    envName,
		Secrets: nil,
	})
	return nil
}

// GetSecrets decrypts and returns the secrets map for this environment.
func (e *Environment) GetSecrets(key []byte) (map[string]string, error) {
	if e.Secrets == nil || e.Secrets.Ciphertext == "" {
		return make(map[string]string), nil
	}

	decryptedBytes, err := crypto.Decrypt(e.Secrets.IV, e.Secrets.Ciphertext, key)
	if err != nil {
		return nil, err
	}

	var secrets map[string]string
	if err := json.Unmarshal(decryptedBytes, &secrets); err != nil {
		return nil, errors.New("failed to parse decrypted secrets JSON: " + err.Error())
	}

	return secrets, nil
}

// SetSecrets encrypts and stores the secrets map for this environment.
func (e *Environment) SetSecrets(secrets map[string]string, key []byte) error {
	if secrets == nil {
		secrets = make(map[string]string)
	}

	plaintextBytes, err := json.Marshal(secrets)
	if err != nil {
		return err
	}

	iv, ciphertext, err := crypto.Encrypt(plaintextBytes, key)
	if err != nil {
		return err
	}

	e.Secrets = &EncryptedPayload{
		IV:         iv,
		Ciphertext: ciphertext,
	}
	return nil
}

// LoadDatabase loads the database from disk. If the file does not exist,
// it initializes a new Database struct with a random salt.
func LoadDatabase(path string) (*Database, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Initialize a fresh database with a 16-byte random salt
		saltBytes := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, saltBytes); err != nil {
			return nil, err
		}
		return &Database{
			Salt:     hex.EncodeToString(saltBytes),
			Projects: []Project{},
		}, nil
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var db Database
	if err := json.Unmarshal(fileBytes, &db); err != nil {
		return nil, err
	}

	// Double check salt exists, if not generate one (fallback for older DBs if any)
	if db.Salt == "" {
		saltBytes := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, saltBytes); err == nil {
			db.Salt = hex.EncodeToString(saltBytes)
		}
	}

	return &db, nil
}

// SaveDatabase writes the database to disk.
func SaveDatabase(path string, db *Database) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	fileBytes, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, fileBytes, 0600)
}

// LockFile represents a file lock on the database.
type LockFile struct {
	flock *flock.Flock
}

// LockExclusive acquires an exclusive write lock on the database.
// It blocks until the lock is acquired.
func LockExclusive(path string) (*LockFile, error) {
	lockPath := path + ".lock"
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f := flock.New(lockPath)
	if err := f.Lock(); err != nil {
		return nil, err
	}
	return &LockFile{flock: f}, nil
}

// LockShared acquires a shared read lock on the database.
// It blocks until the lock is acquired.
func LockShared(path string) (*LockFile, error) {
	lockPath := path + ".lock"
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f := flock.New(lockPath)
	if err := f.RLock(); err != nil {
		return nil, err
	}
	return &LockFile{flock: f}, nil
}

// Unlock releases the database lock.
func (l *LockFile) Unlock() error {
	if l == nil || l.flock == nil {
		return nil
	}
	err := l.flock.Unlock()
	l.flock = nil // prevent subsequent double-unlock calls
	return err
}

