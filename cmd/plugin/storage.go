package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "sync"

    "github.com/983033995/cliproxyapi-ofm-provider/internal/provider"
)

// A provider's public catalog/config is stored separately from its secret material.
// This is a local, owner-managed 0600 file, NOT an encrypted credential vault.
var configMu sync.Mutex

func configPath() (string, error) {
    path := strings.TrimSpace(os.Getenv("OFM_PROVIDERS_FILE"))
    if path != "" {
        if !filepath.IsAbs(path) { return "", errors.New("OFM_PROVIDERS_FILE must be absolute") }
        return filepath.Clean(path), nil
    }
    dir, err := os.UserConfigDir()
    if err != nil { return "", err }
    return filepath.Join(dir, "cliproxyapi", "ofm-providers.json"), nil
}

func secretsPath() (string, error) {
    path, err := configPath()
    if err != nil { return "", err }
    if custom := strings.TrimSpace(os.Getenv("OFM_SECRETS_FILE")); custom != "" {
        if !filepath.IsAbs(custom) { return "", errors.New("OFM_SECRETS_FILE must be absolute") }
        return filepath.Clean(custom), nil
    }
    return path + ".secrets.json", nil
}

func readConfigUnlocked() (provider.Config, error) {
    path, err := configPath()
    if err != nil { return provider.Config{}, err }
    raw, err := os.ReadFile(path)
    if errors.Is(err, os.ErrNotExist) {
        var defaultCatalog provider.Config
        if err := json.Unmarshal(catalogJSON, &defaultCatalog); err != nil { return provider.Config{}, err }
        return defaultCatalog, nil
    }
    if err != nil { return provider.Config{}, err }
    var config provider.Config
    if err := json.Unmarshal(raw, &config); err != nil { return provider.Config{}, fmt.Errorf("invalid provider config: %w", err) }
    return config, nil
}

func readConfig() (provider.Config, error) {
    configMu.Lock()
    defer configMu.Unlock()
    return readConfigUnlocked()
}

func writePrivate(path string, bytes []byte) error {
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0700); err != nil { return err }
    if st, err := os.Lstat(path); err == nil {
        if st.Mode() & os.ModeSymlink != 0 { return errors.New("refusing to write through symlink") }
    } else if !errors.Is(err, os.ErrNotExist) { return err }
    temp, err := os.CreateTemp(dir, ".ofm-*.tmp")
    if err != nil { return err }
    name := temp.Name()
    defer os.Remove(name)
    if err := temp.Chmod(0600); err != nil { temp.Close(); return err }
    if _, err := temp.Write(bytes); err != nil { temp.Close(); return err }
    if err := temp.Sync(); err != nil { temp.Close(); return err }
    if err := temp.Close(); err != nil { return err }
    return os.Rename(name, path)
}

func writeConfigUnlocked(config provider.Config) error {
    if _, err := provider.Validate(config); err != nil { return err }
    raw, err := json.MarshalIndent(config, "", "  ")
    if err != nil { return err }
    path, err := configPath()
    if err != nil { return err }
    return writePrivate(path, append(raw, '\n'))
}

func readSecretsUnlocked() (map[string]string, error) {
    path, err := secretsPath()
    if err != nil { return nil, err }
    st, err := os.Lstat(path)
    if errors.Is(err, os.ErrNotExist) { return map[string]string{}, nil }
    if err != nil { return nil, err }
    if !st.Mode().IsRegular() || st.Mode().Perm() & 0077 != 0 {
        return nil, errors.New("OFM secrets file must be a private regular file (0600)")
    }
    raw, err := os.ReadFile(path)
    if err != nil { return nil, err }
    secrets := map[string]string{}
    if err := json.Unmarshal(raw, &secrets); err != nil { return nil, errors.New("invalid OFM secrets file") }
    return secrets, nil
}

func writeSecretsUnlocked(secrets map[string]string) error {
    path, err := secretsPath()
    if err != nil { return err }
    raw, err := json.Marshal(secrets)
    if err != nil { return err }
    return writePrivate(path, append(raw, '\n'))
}

func credentialUnlocked(p provider.Entry, secrets map[string]string) (string, string) {
    if p.AuthKind == "none" { return "", "none" }
    // A credential entered through the management UI overrides a legacy env variable.
    if key := strings.TrimSpace(secrets[p.ID]); key != "" { return key, "local-file" }
    if p.KeyEnv != "" {
        if key := strings.TrimSpace(os.Getenv(p.KeyEnv)); key != "" { return key, "environment" }
    }
    return "", ""
}

func credentialFor(p provider.Entry) (string, error) {
    configMu.Lock()
    defer configMu.Unlock()
    secrets, err := readSecretsUnlocked()
    if err != nil { return "", err }
    key, _ := credentialUnlocked(p, secrets)
    if p.AuthKind == "none" || key != "" { return key, nil }
    return "", errors.New("upstream credential is not configured")
}
