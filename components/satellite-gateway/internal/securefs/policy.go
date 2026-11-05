package securefs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

type Material string

const (
	Certificate Material = "certificate"
	PrivateKey  Material = "private-key"
	PublicKey   Material = "public-key"
	CSR         Material = "csr"
	CABundle    Material = "ca-bundle"
	Secret      Material = "secret"
)

var encodedPathEscape = regexp.MustCompile(`(?i)%2e|%2f|%5c|%00`)

type allowedRoot struct {
	path string
	root *os.Root
}

// Policy restricts sensitive file operations to administrator-approved roots.
// Tenant roots replace the global roots for that tenant.
type Policy struct {
	mu          sync.RWMutex
	roots       []allowedRoot
	globalRoots []allowedRoot
	tenantRoots map[string][]allowedRoot
	closed      bool
}

func NewPolicy(global []string, tenants map[string][]string) (*Policy, error) {
	if len(global) == 0 && len(tenants) == 0 {
		return nil, errors.New("file storage is default-deny: configure allowed_roots or tenant_roots")
	}

	policy := &Policy{tenantRoots: make(map[string][]allowedRoot, len(tenants))}
	opened := make(map[string]allowedRoot)
	closeOpened := func() {
		for _, root := range opened {
			_ = root.root.Close()
		}
	}
	openRoots := func(paths []string) ([]allowedRoot, error) {
		roots := make([]allowedRoot, 0, len(paths))
		for _, path := range paths {
			abs, err := canonicalRoot(path)
			if err != nil {
				return nil, err
			}
			entry, ok := opened[abs]
			if !ok {
				root, err := os.OpenRoot(abs)
				if err != nil {
					return nil, fmt.Errorf("opening approved file root %q: %w", abs, err)
				}
				info, err := root.Stat(".")
				if err != nil {
					_ = root.Close()
					return nil, fmt.Errorf("checking approved file root %q: %w", abs, err)
				}
				if !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
					_ = root.Close()
					return nil, fmt.Errorf("approved file root %q must be a directory not writable by group or others", abs)
				}
				if err := checkRootOwner(info); err != nil {
					_ = root.Close()
					return nil, fmt.Errorf("checking approved file root %q ownership: %w", abs, err)
				}
				entry = allowedRoot{path: abs, root: root}
				opened[abs] = entry
				policy.roots = append(policy.roots, entry)
			}
			roots = append(roots, entry)
		}
		slices.SortFunc(roots, func(a, b allowedRoot) int {
			return len(b.path) - len(a.path)
		})
		return roots, nil
	}

	var err error
	policy.globalRoots, err = openRoots(global)
	if err != nil {
		closeOpened()
		return nil, err
	}
	for tenant, paths := range tenants {
		tenant = strings.TrimSpace(tenant)
		if tenant == "" || tenant == "*" {
			closeOpened()
			return nil, errors.New("tenant_roots keys must be explicit tenant identifiers")
		}
		roots, err := openRoots(paths)
		if err != nil {
			closeOpened()
			return nil, fmt.Errorf("tenant %q: %w", tenant, err)
		}
		if len(roots) == 0 {
			closeOpened()
			return nil, fmt.Errorf("tenant %q has no approved file roots", tenant)
		}
		policy.tenantRoots[tenant] = roots
	}

	if len(policy.globalRoots) == 0 && len(policy.tenantRoots) == 0 {
		closeOpened()
		return nil, errors.New("file storage is default-deny: no approved file roots were configured")
	}
	return policy, nil
}

func canonicalRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00') {
		return "", errors.New("approved file root is empty or contains a null byte")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("approved file root %q must be absolute", path)
	}
	if hasTraversal(path) {
		return "", fmt.Errorf("approved file root %q contains path traversal", path)
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", fmt.Errorf("resolving existing approved file root %q: %w", clean, err)
	}
	if !samePath(clean, resolved) {
		return "", fmt.Errorf("approved file root %q contains a symbolic link; configure its canonical path %q", clean, resolved)
	}
	return clean, nil
}

func samePath(a, b string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func hasTraversal(path string) bool {
	if encodedPathEscape.MatchString(path) {
		return true
	}
	for _, component := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == ".." {
			return true
		}
	}
	return false
}

func (p *Policy) Resolve(path, tenant string) (string, string, error) {
	root, relative, err := p.resolve(path, tenant)
	if err != nil {
		return "", "", err
	}
	return root.path, relative, nil
}

// DerivedPath places a generated filename under the tenant's first approved
// root, or under base when that tenant uses the gateway-wide roots.
func (p *Policy) DerivedPath(base, relative, tenant string) (string, error) {
	if filepath.IsAbs(relative) || hasTraversal(relative) || strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("derived file name %q must be a non-empty relative path without traversal", relative)
	}
	if p == nil {
		return "", errors.New("file storage policy is not configured")
	}
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return "", errors.New("file storage policy is closed")
	}
	if roots, ok := p.tenantRoots[strings.TrimSpace(tenant)]; ok {
		base = roots[0].path
	}
	p.mu.RUnlock()
	path := filepath.Join(base, relative)
	if _, _, err := p.Resolve(path, tenant); err != nil {
		return "", err
	}
	return path, nil
}

func (p *Policy) ReadFile(path, tenant string, material Material) ([]byte, error) {
	root, relative, err := p.resolve(path, tenant)
	if err != nil {
		return nil, err
	}
	info, err := root.root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sensitive file %q must be a regular, non-symlink file", path)
	}
	if err := validateFilePermissions(info, material); err != nil {
		return nil, fmt.Errorf("checking sensitive file %q: %w", path, err)
	}

	file, err := root.root.Open(relative)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, fmt.Errorf("sensitive file %q changed while it was being opened", path)
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("closing sensitive file %q: %w", path, closeErr)
	}
	return data, nil
}

func (p *Policy) WriteFile(path, tenant string, material Material, data []byte, replace bool) error {
	root, relative, err := p.resolve(path, tenant)
	if err != nil {
		return err
	}
	if err := validateParent(root.root, relative); err != nil {
		return fmt.Errorf("checking destination %q: %w", path, err)
	}
	mode, err := permissions(material)
	if err != nil {
		return err
	}

	info, statErr := root.root.Lstat(relative)
	switch {
	case statErr == nil:
		if !replace {
			return fmt.Errorf("destination %q already exists; explicit replacement authorization is required", path)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("destination %q must be a regular, non-symlink file", path)
		}
		if err := validateFilePermissions(info, material); err != nil {
			return fmt.Errorf("checking existing destination %q: %w", path, err)
		}
	case errors.Is(statErr, os.ErrNotExist):
		replace = false
	case statErr != nil:
		return fmt.Errorf("checking destination %q: %w", path, statErr)
	}

	flags := os.O_WRONLY
	if !replace {
		flags |= os.O_CREATE | os.O_EXCL
	}
	file, err := root.root.OpenFile(relative, flags, mode)
	if err != nil {
		return fmt.Errorf("opening destination %q: %w", path, err)
	}
	created := !replace
	closeAndRemove := func() {
		_ = file.Close()
		if created {
			_ = root.root.Remove(relative)
		}
	}
	openedInfo, err := file.Stat()
	if err != nil {
		closeAndRemove()
		return fmt.Errorf("checking opened destination %q: %w", path, err)
	}
	if replace && !os.SameFile(info, openedInfo) {
		closeAndRemove()
		return fmt.Errorf("destination %q changed while it was being opened", path)
	}
	if replace {
		if err := file.Truncate(0); err != nil {
			closeAndRemove()
			return fmt.Errorf("truncating destination %q: %w", path, err)
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			closeAndRemove()
			return fmt.Errorf("seeking destination %q: %w", path, err)
		}
	}
	if err := file.Chmod(mode); err != nil {
		closeAndRemove()
		return fmt.Errorf("setting permissions on destination %q: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		closeAndRemove()
		return fmt.Errorf("writing destination %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		closeAndRemove()
		return fmt.Errorf("syncing destination %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		if created {
			_ = root.root.Remove(relative)
		}
		return fmt.Errorf("closing destination %q: %w", path, err)
	}
	return nil
}

func (p *Policy) Remove(path, tenant string, material Material) error {
	root, relative, err := p.resolve(path, tenant)
	if err != nil {
		return err
	}
	info, err := root.root.Lstat(relative)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("sensitive file %q must be a regular, non-symlink file", path)
	}
	if err := validateFilePermissions(info, material); err != nil {
		return fmt.Errorf("checking sensitive file %q: %w", path, err)
	}
	current, err := root.root.Lstat(relative)
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) {
		return fmt.Errorf("sensitive file %q changed before removal", path)
	}
	return root.root.Remove(relative)
}

func (p *Policy) MkdirAll(path, tenant string, mode os.FileMode) error {
	root, relative, err := p.resolve(path, tenant)
	if err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	current := ""
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if err := root.root.Mkdir(current, mode); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := root.root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("directory %q must be a real directory not writable by group or others", path)
		}
	}
	return nil
}

// OpenDirectory creates and opens a child directory while keeping subsequent
// operations rooted to the approved directory handle.
func (p *Policy) OpenDirectory(path, tenant string, mode os.FileMode) (*os.Root, error) {
	root, relative, err := p.resolve(path, tenant)
	if err != nil {
		return nil, err
	}
	if relative == "." {
		return root.root.OpenRoot(".")
	}
	if err := p.MkdirAll(path, tenant, mode); err != nil {
		return nil, err
	}
	return root.root.OpenRoot(relative)
}

func (p *Policy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	var closeErr error
	for _, root := range p.roots {
		if err := root.root.Close(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}

func (p *Policy) resolve(path, tenant string) (allowedRoot, string, error) {
	if p == nil {
		return allowedRoot{}, "", errors.New("file storage policy is not configured")
	}
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00') {
		return allowedRoot{}, "", errors.New("file path is empty or contains a null byte")
	}
	if !filepath.IsAbs(path) {
		return allowedRoot{}, "", fmt.Errorf("file path %q must be absolute", path)
	}
	if hasTraversal(path) {
		return allowedRoot{}, "", fmt.Errorf("file path %q contains traversal or encoded path separators", path)
	}
	absolute := filepath.Clean(path)

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return allowedRoot{}, "", errors.New("file storage policy is closed")
	}
	roots := p.globalRoots
	if tenantRoots, ok := p.tenantRoots[strings.TrimSpace(tenant)]; ok {
		roots = tenantRoots
	}
	for _, root := range roots {
		relative, err := filepath.Rel(root.path, absolute)
		if err != nil || relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		return root, relative, nil
	}
	return allowedRoot{}, "", fmt.Errorf("file path %q is outside the configured roots for tenant %q", path, tenant)
}

func validateParent(root *os.Root, relative string) error {
	parent := filepath.Dir(relative)
	if parent == "." {
		return nil
	}
	current := ""
	for _, component := range strings.Split(parent, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		if component == ".." {
			return errors.New("path traversal is not allowed")
		}
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("parent component %q is not a real directory", current)
		}
	}
	return nil
}

func permissions(material Material) (os.FileMode, error) {
	switch material {
	case PrivateKey, Secret:
		return 0o600, nil
	case Certificate, PublicKey, CSR, CABundle:
		return 0o644, nil
	default:
		return 0, fmt.Errorf("unsupported sensitive material type %q", material)
	}
}

func validateFilePermissions(info os.FileInfo, material Material) error {
	if !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if material == PrivateKey || material == Secret {
		if info.Mode().Perm()&0o077 != 0 {
			return errors.New("private material permissions must be 0600 or stricter")
		}
		return checkOwner(info)
	}
	return nil
}
