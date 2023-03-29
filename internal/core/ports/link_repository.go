package ports

import (
	"context"

	"github.com/google/uuid"
	core "github.com/iden3/go-iden3-core"

	"github.com/polygonid/sh-id-platform/internal/core/domain"
)

// LinkRepository the interface that defines the available methods
type LinkRepository interface {
	Save(ctx context.Context, link *domain.Link) (*uuid.UUID, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Link, error)
	Delete(ctx context.Context, id uuid.UUID, issuerDID core.DID) error
}