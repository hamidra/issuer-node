package ports

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iden3/go-iden3-core/v2/w3c"
	"github.com/iden3/go-schema-processor/v2/verifiable"
	"github.com/iden3/iden3comm/v2/protocol"

	"github.com/polygonid/sh-id-platform/internal/core/domain"
	linkState "github.com/polygonid/sh-id-platform/pkg/link"
)

// CreateQRCodeResponse - is the result of creating a link QRcode.
type CreateQRCodeResponse struct {
	Link      *domain.Link
	QrCode    string
	QrID      uuid.UUID
	SessionID string
}

// LinkStatus is a Link type request. All|Active|Inactive|Exceeded
type LinkStatus string

const (
	LinkAll      LinkStatus = "all"      // LinkAll : All links
	LinkActive   LinkStatus = "active"   // LinkActive : Active links
	LinkInactive LinkStatus = "inactive" // LinkInactive : Inactive links
	LinkExceeded LinkStatus = "exceeded" // LinkExceeded : Expired links or with more credentials issued than expected
)

// LinkTypeReqFromString constructs a LinkStatus from a string
func LinkTypeReqFromString(s string) (LinkStatus, error) {
	s = strings.ToLower(s)
	if s != "all" && s != "active" && s != "inactive" && s != "exceeded" {
		return "", fmt.Errorf("unknown linkTypeReq: %s", s)
	}
	return LinkStatus(s), nil
}

// GetQRCodeResponse - is the get link qrcode response.
type GetQRCodeResponse struct {
	Link  *domain.Link
	State *linkState.State
}

// LinkService - the interface that defines the available methods
type LinkService interface {
	Save(ctx context.Context, did w3c.DID, maxIssuance *int, validUntil *time.Time, schemaID uuid.UUID, credentialExpiration *time.Time, credentialSignatureProof bool, credentialMTPProof bool, credentialAttributes domain.CredentialSubject, refreshService *verifiable.RefreshService, displayMethod *verifiable.DisplayMethod, credentialStatusType verifiable.CredentialStatusType) (*domain.Link, error)
	Activate(ctx context.Context, issuerID w3c.DID, linkID uuid.UUID, active bool) error
	Delete(ctx context.Context, id uuid.UUID, did w3c.DID) error
	GetByID(ctx context.Context, issuerID w3c.DID, id uuid.UUID) (*domain.Link, error)
	GetAll(ctx context.Context, issuerDID w3c.DID, status LinkStatus, query *string) ([]domain.Link, error)
	CreateQRCode(ctx context.Context, issuerDID w3c.DID, linkID uuid.UUID, serverURL string) (*CreateQRCodeResponse, error)
	IssueOrFetchClaim(ctx context.Context, sessionID string, issuerDID w3c.DID, userDID w3c.DID, linkID uuid.UUID, hostURL string) (*protocol.CredentialsOfferMessage, error)
	ProcessCallBack(ctx context.Context, message string, sessionID uuid.UUID, linkID uuid.UUID, hostURL string) (*protocol.CredentialsOfferMessage, error)
	GetQRCode(ctx context.Context, sessionID uuid.UUID, issuerID w3c.DID, linkID uuid.UUID) (*GetQRCodeResponse, error)
}
