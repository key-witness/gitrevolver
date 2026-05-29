package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	Version        = 1
	ConfigFile     = "config.yml"
	IdentitiesFile = "identities.yml"
	ProjectsFile   = "projects.yml"
	AgentsTemplate = "AGENTS.template.md"
	ClaudeTemplate = "CLAUDE.template.md"
)

type LibraryConfig struct {
	Version            int    `yaml:"version" json:"version"`
	DefaultProjectsDir string `yaml:"defaultProjectsDir,omitempty" json:"defaultProjectsDir,omitempty"`
	SecretBackend      string `yaml:"secretBackend" json:"secretBackend"`
}

type Identity struct {
	ID          string           `yaml:"-" json:"id"`
	DisplayName string           `yaml:"displayName" json:"displayName"`
	Git         GitIdentity      `yaml:"git" json:"git"`
	GitHub      GitHubIdentity   `yaml:"github" json:"github"`
	Vercel      VercelIdentity   `yaml:"vercel,omitempty" json:"vercel,omitempty"`
	Supabase    SupabaseIdentity `yaml:"supabase,omitempty" json:"supabase,omitempty"`
}

type GitIdentity struct {
	Name  string `yaml:"name" json:"name"`
	Email string `yaml:"email" json:"email"`
}

type GitHubIdentity struct {
	Username       string `yaml:"username" json:"username"`
	HostAlias      string `yaml:"hostAlias" json:"hostAlias"`
	SSHKeyPath     string `yaml:"sshKeyPath" json:"sshKeyPath"`
	APITokenSecret string `yaml:"apiTokenSecret,omitempty" json:"apiTokenSecret,omitempty"`
}

type VercelIdentity struct {
	Mode        string `yaml:"mode,omitempty" json:"mode,omitempty"`
	TokenSecret string `yaml:"tokenSecret,omitempty" json:"tokenSecret,omitempty"`
}

type SupabaseIdentity struct {
	TokenSecret string `yaml:"tokenSecret,omitempty" json:"tokenSecret,omitempty"`
}

type IdentityStore struct {
	Version    int                 `yaml:"version" json:"version"`
	Identities map[string]Identity `yaml:"identities" json:"identities"`
}

type Project struct {
	Slug     string          `yaml:"-" json:"slug"`
	Name     string          `yaml:"name" json:"name"`
	Aliases  []string        `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Identity string          `yaml:"identity" json:"identity"`
	Path     string          `yaml:"path" json:"path"`
	GitHub   ProjectGitHub   `yaml:"github,omitempty" json:"github,omitempty"`
	Vercel   ProjectVercel   `yaml:"vercel,omitempty" json:"vercel,omitempty"`
	Supabase ProjectSupabase `yaml:"supabase,omitempty" json:"supabase,omitempty"`
	Safety   SafetyPolicy    `yaml:"safety,omitempty" json:"safety,omitempty"`
}

type ProjectGitHub struct {
	Owner     string `yaml:"owner,omitempty" json:"owner,omitempty"`
	Repo      string `yaml:"repo,omitempty" json:"repo,omitempty"`
	RemoteSSH string `yaml:"remoteSsh,omitempty" json:"remoteSsh,omitempty"`
}

type ProjectVercel struct {
	Mode             string `yaml:"mode,omitempty" json:"mode,omitempty"`
	OrgID            string `yaml:"orgId,omitempty" json:"orgId,omitempty"`
	ProjectID        string `yaml:"projectId,omitempty" json:"projectId,omitempty"`
	ProductionBranch string `yaml:"productionBranch,omitempty" json:"productionBranch,omitempty"`
}

type ProjectSupabase struct {
	ProjectRef string `yaml:"projectRef,omitempty" json:"projectRef,omitempty"`
}

type SafetyPolicy struct {
	ProtectedBranches []string `yaml:"protectedBranches,omitempty" json:"protectedBranches,omitempty"`
	RequireConfirm    []string `yaml:"requireConfirm,omitempty" json:"requireConfirm,omitempty"`
}

type ProjectStore struct {
	Version  int                `yaml:"version" json:"version"`
	Projects map[string]Project `yaml:"projects" json:"projects"`
}

var getConfigDirFunc = func() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".gitrevolver"), nil
}

var slugPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func ValidateSlug(kind, slug string) error {
	if strings.TrimSpace(slug) == "" {
		return fmt.Errorf("%s slug cannot be empty", kind)
	}
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("%s slug %q contains unsupported characters; use letters, numbers, dots, underscores, or hyphens", kind, slug)
	}
	return nil
}

func GetConfigDir() (string, error) {
	return getConfigDirFunc()
}

func GetConfigPath() (string, error) {
	return configPath(ConfigFile)
}

func GetIdentitiesPath() (string, error) {
	return configPath(IdentitiesFile)
}

func GetProjectsPath() (string, error) {
	return configPath(ProjectsFile)
}

func GetTemplatePath(name string) (string, error) {
	return configPath(name)
}

func configPath(name string) (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func InitLibrary(defaultProjectsDir string) error {
	if defaultProjectsDir == "" {
		defaultProjectsDir = "~/code"
	}
	lib := &LibraryConfig{
		Version:            Version,
		DefaultProjectsDir: defaultProjectsDir,
		SecretBackend:      "platform",
	}
	if err := SaveLibraryConfig(lib); err != nil {
		return err
	}
	if err := SaveConfig(&IdentityStore{Version: Version, Identities: map[string]Identity{}}); err != nil {
		return err
	}
	return SaveProjects(&ProjectStore{Version: Version, Projects: map[string]Project{}})
}

func LoadLibraryConfig() (*LibraryConfig, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}
	var lib LibraryConfig
	if err := readYAML(path, &lib); err != nil {
		if os.IsNotExist(err) {
			return &LibraryConfig{Version: Version, DefaultProjectsDir: "~/code", SecretBackend: "platform"}, nil
		}
		return nil, err
	}
	if lib.Version == 0 {
		lib.Version = Version
	}
	if lib.SecretBackend == "" {
		lib.SecretBackend = "platform"
	}
	return &lib, nil
}

func SaveLibraryConfig(lib *LibraryConfig) error {
	if lib.Version == 0 {
		lib.Version = Version
	}
	path, err := GetConfigPath()
	if err != nil {
		return err
	}
	return writeYAML(path, lib)
}

// LoadConfig keeps the upstream identity API name while loading identities.yml.
func LoadConfig() (*IdentityStore, error) {
	path, err := GetIdentitiesPath()
	if err != nil {
		return nil, err
	}
	store := &IdentityStore{}
	if err := readYAML(path, store); err != nil {
		if os.IsNotExist(err) {
			return emptyIdentityStore(), nil
		}
		return nil, err
	}
	if store.Identities == nil {
		store.Identities = map[string]Identity{}
	}
	if store.Version == 0 {
		store.Version = Version
	}
	for slug, identity := range store.Identities {
		identity.ID = slug
		store.Identities[slug] = identity
	}
	return store, nil
}

func SaveConfig(store *IdentityStore) error {
	if store == nil {
		store = emptyIdentityStore()
	}
	if store.Version == 0 {
		store.Version = Version
	}
	if store.Identities == nil {
		store.Identities = map[string]Identity{}
	}
	path, err := GetIdentitiesPath()
	if err != nil {
		return err
	}
	return writeYAML(path, store)
}

func FindIdentityByAlias(alias string) (*Identity, error) {
	store, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	identity, ok := store.Identities[alias]
	if !ok {
		return nil, fmt.Errorf("identity %q not found", alias)
	}
	identity.ID = alias
	return &identity, nil
}

func AddIdentity(identity Identity) error {
	if err := ValidateSlug("identity", identity.ID); err != nil {
		return err
	}
	store, err := LoadConfig()
	if err != nil {
		return err
	}
	if _, ok := store.Identities[identity.ID]; ok {
		return fmt.Errorf("identity with alias %q already exists", identity.ID)
	}
	store.Identities[identity.ID] = identity
	return SaveConfig(store)
}

func UpdateIdentity(identity Identity) error {
	store, err := LoadConfig()
	if err != nil {
		return err
	}
	if _, ok := store.Identities[identity.ID]; !ok {
		return fmt.Errorf("identity %q not found", identity.ID)
	}
	store.Identities[identity.ID] = identity
	return SaveConfig(store)
}

func RemoveIdentity(alias string) error {
	store, err := LoadConfig()
	if err != nil {
		return err
	}
	if _, ok := store.Identities[alias]; !ok {
		return fmt.Errorf("identity %q not found", alias)
	}
	delete(store.Identities, alias)
	return SaveConfig(store)
}

func LoadProjects() (*ProjectStore, error) {
	path, err := GetProjectsPath()
	if err != nil {
		return nil, err
	}
	store := &ProjectStore{}
	if err := readYAML(path, store); err != nil {
		if os.IsNotExist(err) {
			return emptyProjectStore(), nil
		}
		return nil, err
	}
	if store.Projects == nil {
		store.Projects = map[string]Project{}
	}
	if store.Version == 0 {
		store.Version = Version
	}
	for slug, project := range store.Projects {
		project.Slug = slug
		store.Projects[slug] = project
	}
	return store, nil
}

func SaveProjects(store *ProjectStore) error {
	if store == nil {
		store = emptyProjectStore()
	}
	if store.Version == 0 {
		store.Version = Version
	}
	if store.Projects == nil {
		store.Projects = map[string]Project{}
	}
	path, err := GetProjectsPath()
	if err != nil {
		return err
	}
	return writeYAML(path, store)
}

func FindProject(slug string) (*Project, error) {
	store, err := LoadProjects()
	if err != nil {
		return nil, err
	}
	project, ok := store.Projects[slug]
	if !ok {
		return nil, fmt.Errorf("project %q not found", slug)
	}
	project.Slug = slug
	return &project, nil
}

func UpsertProject(project Project) error {
	if err := ValidateSlug("project", project.Slug); err != nil {
		return err
	}
	store, err := LoadProjects()
	if err != nil {
		return err
	}
	store.Projects[project.Slug] = project
	return SaveProjects(store)
}

func emptyIdentityStore() *IdentityStore {
	return &IdentityStore{Version: Version, Identities: map[string]Identity{}}
}

func emptyProjectStore() *ProjectStore {
	return &ProjectStore{Version: Version, Projects: map[string]Project{}}
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return nil
}

func writeYAML(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", path, err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".gitrevolver-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary config: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	return nil
}
