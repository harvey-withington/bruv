package repo

import (
	"bruv/internal/model"
	"fmt"
)

// GetProjectMembers reads the project-scoped members.json registry.
// If the file doesn't exist, it returns an empty slice and no error.
func (r *Repository) GetProjectMembers(brandSlug, streamSlug, projectSlug string) ([]model.ProjectMember, error) {
	path := r.projectMembersFilePath(brandSlug, streamSlug, projectSlug)
	if !fileExists(path) {
		return []model.ProjectMember{}, nil
	}

	var members []model.ProjectMember
	if err := readJSON(path, &members); err != nil {
		return nil, fmt.Errorf("read project members: %w", err)
	}
	return members, nil
}

// SaveProjectMembers writes the project-scoped members.json registry.
func (r *Repository) SaveProjectMembers(brandSlug, streamSlug, projectSlug string, members []model.ProjectMember) error {
	path := r.projectMembersFilePath(brandSlug, streamSlug, projectSlug)
	if err := writeJSON(path, members); err != nil {
		return fmt.Errorf("write project members: %w", err)
	}
	return nil
}
