package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/thezzisu/pkudisk-fuse/internal/anyshare"
	"github.com/thezzisu/pkudisk-fuse/internal/transfer"
	"github.com/thezzisu/pkudisk-fuse/internal/vfs"
)

type tuiTarget struct {
	backend   transfer.Backend
	id, owner string
	namespace *vfs.AccountBackend
	scopeID   string
}

// Transfers and sharing use the owning account's original API. Only browser
// metadata carries scoped IDs, preserving selection across duplicate libraries.
func (m *tuiModel) target(id string) (tuiTarget, error) {
	target := tuiTarget{backend: m.service.Backend, id: id}
	if namespace, ok := m.backend.(*vfs.AccountBackend); ok {
		account, remote, err := namespace.Account(id)
		if err != nil {
			return tuiTarget{}, err
		}
		if remote == "" {
			return tuiTarget{}, errors.New("account directories are virtual; open a library before file operations")
		}
		target.backend, _ = account.Backend.(transfer.Backend)
		target.id, target.owner, target.namespace, target.scopeID = remote, account.Name, namespace, id
	}
	if target.backend == nil {
		return tuiTarget{}, errors.New("this backend does not support file operations")
	}
	return target, nil
}

func (t tuiTarget) scope(entry anyshare.Entry) anyshare.Entry {
	if t.namespace != nil {
		return t.namespace.Scope(t.scopeID, entry)
	}
	return entry
}

func (t tuiTarget) error(err error) error {
	if err != nil && t.owner != "" {
		return fmt.Errorf("account %q: %w", t.owner, err)
	}
	return err
}

func (t tuiTarget) extractionDestination(selector string) (string, error) {
	if t.namespace == nil || strings.HasPrefix(selector, "gns://") {
		return selector, nil
	}
	prefix := "/" + t.owner + "/"
	if !strings.HasPrefix(selector, prefix) {
		return "", fmt.Errorf("extract into a library under %s; cross-account extraction is not supported", prefix)
	}
	return "/" + strings.TrimPrefix(selector, prefix), nil
}

func (m *tuiModel) virtual(id string) bool {
	if namespace, ok := m.backend.(*vfs.AccountBackend); ok {
		return namespace.VirtualDirectory(id)
	}
	return false
}

func (m *tuiModel) accountContext() (string, string) {
	if m.mode == managementMode && m.management != nil && m.management.target.namespace != nil {
		if account, _, err := m.management.target.namespace.Account(m.management.target.scopeID); err == nil {
			return account.Name, account.Server
		}
	}
	if namespace, ok := m.backend.(*vfs.AccountBackend); ok && m.location.entry != nil {
		if account, _, err := namespace.Account(m.location.entry.ID); err == nil {
			return account.Name, account.Server
		}
	}
	return m.account, m.server
}
