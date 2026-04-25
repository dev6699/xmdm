package artifactcleanup

import (
	"context"
	"fmt"

	"xmdm/server/internal/artifacts"
	files "xmdm/server/internal/files"
)

type Repository interface {
	ListOrphanArtifacts(ctx context.Context, tenantID string) ([]files.Artifact, error)
	RetireArtifact(ctx context.Context, tenantID, id string) (files.Artifact, error)
	DeleteArtifact(ctx context.Context, tenantID, id string) error
}

type Result struct {
	Candidates []files.Artifact
	Retired    []files.Artifact
	Deleted    []files.Artifact
}

func Run(ctx context.Context, repo Repository, objectStore artifacts.Store, tenantID string, apply bool) (Result, error) {
	if repo == nil {
		return Result{}, fmt.Errorf("missing cleanup repository")
	}
	if objectStore == nil {
		return Result{}, fmt.Errorf("missing artifact object store")
	}
	if tenantID == "" {
		return Result{}, fmt.Errorf("missing tenant id")
	}

	candidates, err := repo.ListOrphanArtifacts(ctx, tenantID)
	if err != nil {
		return Result{}, err
	}

	result := Result{Candidates: candidates}
	if !apply {
		return result, nil
	}

	for _, artifact := range candidates {
		current := artifact
		if current.Status == files.StatusActive {
			retired, err := repo.RetireArtifact(ctx, tenantID, current.ID)
			if err != nil {
				return result, err
			}
			current = retired
			result.Retired = append(result.Retired, retired)
		}

		if err := objectStore.Delete(ctx, current.StorageKey); err != nil {
			return result, err
		}
		if err := repo.DeleteArtifact(ctx, tenantID, current.ID); err != nil {
			return result, err
		}
		result.Deleted = append(result.Deleted, current)
	}

	return result, nil
}
