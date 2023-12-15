package api_ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/iden3/go-iden3-core/v2/w3c"
	"github.com/iden3/go-schema-processor/v2/verifiable"
	"github.com/iden3/iden3comm/v2"
	"github.com/iden3/iden3comm/v2/packers"

	"github.com/polygonid/sh-id-platform/internal/common"
	"github.com/polygonid/sh-id-platform/internal/config"
	"github.com/polygonid/sh-id-platform/internal/core/domain"
	"github.com/polygonid/sh-id-platform/internal/core/ports"
	"github.com/polygonid/sh-id-platform/internal/core/services"
	"github.com/polygonid/sh-id-platform/internal/gateways"
	"github.com/polygonid/sh-id-platform/internal/health"
	"github.com/polygonid/sh-id-platform/internal/log"
	"github.com/polygonid/sh-id-platform/internal/repositories"
	link_state "github.com/polygonid/sh-id-platform/pkg/link"
	"github.com/polygonid/sh-id-platform/pkg/schema"
)

// Server implements StrictServerInterface and holds the implementation of all API controllers
// This is the glue to the API autogenerated code
type Server struct {
	cfg                *config.Configuration
	identityService    ports.IdentityService
	claimService       ports.ClaimsService
	schemaService      ports.SchemaService
	connectionsService ports.ConnectionsService
	linkService        ports.LinkService
	qrService          ports.QrStoreService
	publisherGateway   ports.Publisher
	packageManager     *iden3comm.PackageManager
	health             *health.Status
}

// NewServer is a Server constructor
func NewServer(cfg *config.Configuration, identityService ports.IdentityService, claimsService ports.ClaimsService, schemaService ports.SchemaService, connectionsService ports.ConnectionsService, linkService ports.LinkService, qrService ports.QrStoreService, publisherGateway ports.Publisher, packageManager *iden3comm.PackageManager, health *health.Status) *Server {
	return &Server{
		cfg:                cfg,
		identityService:    identityService,
		claimService:       claimsService,
		schemaService:      schemaService,
		connectionsService: connectionsService,
		linkService:        linkService,
		qrService:          qrService,
		publisherGateway:   publisherGateway,
		packageManager:     packageManager,
		health:             health,
	}
}

// GetSchema is the UI endpoint that searches and schema by Id and returns it.
func (s *Server) GetSchema(ctx context.Context, request GetSchemaRequestObject) (GetSchemaResponseObject, error) {
	schema, err := s.schemaService.GetByID(ctx, s.cfg.APIUI.IssuerDID, request.Id)
	if errors.Is(err, services.ErrSchemaNotFound) {
		log.Debug(ctx, "schema not found", "id", request.Id)
		return GetSchema404JSONResponse{N404JSONResponse{Message: "schema not found"}}, nil
	}
	if err != nil {
		log.Error(ctx, "loading schema", "err", err, "id", request.Id)
	}
	return GetSchema200JSONResponse(schemaResponse(schema)), nil
}

// GetSchemas returns the list of schemas that match the request.Params.Query filter. If param query is nil it will return all
func (s *Server) GetSchemas(ctx context.Context, request GetSchemasRequestObject) (GetSchemasResponseObject, error) {
	col, err := s.schemaService.GetAll(ctx, s.cfg.APIUI.IssuerDID, request.Params.Query)
	if err != nil {
		return GetSchemas500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return GetSchemas200JSONResponse(schemaCollectionResponse(col)), nil
}

// Health is a method
func (s *Server) Health(_ context.Context, _ HealthRequestObject) (HealthResponseObject, error) {
	var resp Health200JSONResponse = s.health.Status()

	return resp, nil
}

// ImportSchema is the UI endpoint to import schema metadata
func (s *Server) ImportSchema(ctx context.Context, request ImportSchemaRequestObject) (ImportSchemaResponseObject, error) {
	req := request.Body
	if err := guardImportSchemaReq(req); err != nil {
		log.Debug(ctx, "Importing schema bad request", "err", err, "req", req)
		return ImportSchema400JSONResponse{N400JSONResponse{Message: fmt.Sprintf("bad request: %s", err.Error())}}, nil
	}
	iReq := ports.NewImportSchemaRequest(req.Url, req.SchemaType, req.Title, req.Version, req.Description)
	schema, err := s.schemaService.ImportSchema(ctx, s.cfg.APIUI.IssuerDID, iReq)
	if err != nil {
		log.Error(ctx, "Importing schema", "err", err, "req", req)
		return ImportSchema500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return ImportSchema201JSONResponse{Id: schema.ID.String()}, nil
}

func guardImportSchemaReq(req *ImportSchemaJSONRequestBody) error {
	if req == nil {
		return errors.New("empty body")
	}
	if strings.TrimSpace(req.Url) == "" {
		return errors.New("empty url")
	}
	if strings.TrimSpace(req.SchemaType) == "" {
		return errors.New("empty type")
	}
	if _, err := url.ParseRequestURI(req.Url); err != nil {
		return fmt.Errorf("parsing url: %w", err)
	}
	return nil
}

// GetDocumentation this method will be overridden in the main function
func (s *Server) GetDocumentation(_ context.Context, _ GetDocumentationRequestObject) (GetDocumentationResponseObject, error) {
	return nil, nil
}

// GetFavicon this method will be overridden in the main function
func (s *Server) GetFavicon(_ context.Context, _ GetFaviconRequestObject) (GetFaviconResponseObject, error) {
	return nil, nil
}

// AuthCallback receives the authentication information of a holder
func (s *Server) AuthCallback(ctx context.Context, request AuthCallbackRequestObject) (AuthCallbackResponseObject, error) {
	if request.Body == nil || *request.Body == "" {
		log.Debug(ctx, "empty request body auth-callback request")
		return AuthCallback400JSONResponse{N400JSONResponse{"Cannot proceed with empty body"}}, nil
	}

	_, err := s.identityService.Authenticate(ctx, *request.Body, request.Params.SessionID, s.cfg.APIUI.ServerURL, s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Debug(ctx, "error authenticating", err.Error())
		return AuthCallback500JSONResponse{}, nil
	}

	return AuthCallback200Response{}, nil
}

// GetAuthenticationConnection returns the connection related to a given session
func (s *Server) GetAuthenticationConnection(ctx context.Context, req GetAuthenticationConnectionRequestObject) (GetAuthenticationConnectionResponseObject, error) {
	conn, err := s.connectionsService.GetByUserSessionID(ctx, req.Id)
	if err != nil {
		log.Error(ctx, "get authentication connection", "err", err, "req", req)
		if errors.Is(err, services.ErrConnectionDoesNotExist) {
			return GetAuthenticationConnection404JSONResponse{N404JSONResponse{err.Error()}}, nil
		}
		return GetAuthenticationConnection500JSONResponse{N500JSONResponse{"Unexpected error while getting authentication session"}}, nil
	}

	return GetAuthenticationConnection200JSONResponse{
		Connection: AuthenticationConnection{
			Id:         conn.ID.String(),
			UserID:     conn.UserDID.String(),
			IssuerID:   conn.IssuerDID.String(),
			CreatedAt:  TimeUTC(conn.CreatedAt),
			ModifiedAt: TimeUTC(conn.ModifiedAt),
		},
	}, nil
}

// AuthQRCode returns the qr code for authenticating a user
func (s *Server) AuthQRCode(ctx context.Context, _ AuthQRCodeRequestObject) (AuthQRCodeResponseObject, error) {
	qrCode, sessionID, err := s.identityService.CreateAuthenticationQRCode(ctx, s.cfg.APIUI.ServerURL, s.cfg.APIUI.IssuerDID)
	if err != nil {
		return AuthQRCode500JSONResponse{N500JSONResponse{"Unexpected error while creating qr code"}}, nil
	}
	return AuthQRCode200JSONResponse{
		QrCodeLink: qrCode,
		SessionID:  sessionID.String(),
	}, nil
}

// GetConnection returns a connection with its related credentials
func (s *Server) GetConnection(ctx context.Context, request GetConnectionRequestObject) (GetConnectionResponseObject, error) {
	conn, err := s.connectionsService.GetByIDAndIssuerID(ctx, request.Id, s.cfg.APIUI.IssuerDID)
	if err != nil {
		if errors.Is(err, services.ErrConnectionDoesNotExist) {
			return GetConnection400JSONResponse{N400JSONResponse{"The given connection does not exist"}}, nil
		}
		log.Debug(ctx, "get connection internal server error", "err", err, "req", request)
		return GetConnection500JSONResponse{N500JSONResponse{"There was an error retrieving the connection"}}, nil
	}

	filter := &ports.ClaimsFilter{
		Subject: conn.UserDID.String(),
	}
	credentials, _, err := s.claimService.GetAll(ctx, s.cfg.APIUI.IssuerDID, filter)
	if err != nil && !errors.Is(err, services.ErrClaimNotFound) {
		log.Debug(ctx, "get connection internal server error retrieving credentials", "err", err, "req", request)
		return GetConnection500JSONResponse{N500JSONResponse{"There was an error retrieving the connection"}}, nil
	}

	w3credentials, err := schema.FromClaimsModelToW3CCredential(credentials)
	if err != nil {
		log.Debug(ctx, "get connection internal server error converting credentials to w3c", "err", err, "req", request)
		return GetConnection500JSONResponse{N500JSONResponse{"There was an error parsing the credential of the given connection"}}, nil
	}

	return GetConnection200JSONResponse(connectionResponse(conn, w3credentials, credentials)), nil
}

// GetConnections returns the list of credentials of a determined issuer
func (s *Server) GetConnections(ctx context.Context, request GetConnectionsRequestObject) (GetConnectionsResponseObject, error) {
	req := ports.NewGetAllRequest(request.Params.Credentials, request.Params.Query)
	conns, err := s.connectionsService.GetAllByIssuerID(ctx, s.cfg.APIUI.IssuerDID, req.Query, req.WithCredentials)
	if err != nil {
		log.Error(ctx, "get connection request", "err", err)
		return GetConnections500JSONResponse{N500JSONResponse{"Unexpected error while retrieving connections"}}, nil
	}

	resp, err := connectionsResponse(conns)
	if err != nil {
		log.Error(ctx, "get connection request invalid claim format", "err", err)
		return GetConnections500JSONResponse{N500JSONResponse{"Unexpected error while retrieving connections"}}, nil

	}

	return GetConnections200JSONResponse(resp), nil
}

// DeleteConnection deletes a connection
func (s *Server) DeleteConnection(ctx context.Context, request DeleteConnectionRequestObject) (DeleteConnectionResponseObject, error) {
	req := ports.NewDeleteRequest(request.Id, request.Params.DeleteCredentials, request.Params.RevokeCredentials)
	if req.RevokeCredentials {
		err := s.claimService.RevokeAllFromConnection(ctx, req.ConnID, s.cfg.APIUI.IssuerDID)
		if err != nil {
			log.Error(ctx, "delete connection, revoking credentials", "err", err, "req", request.Id.String())
			return DeleteConnection500JSONResponse{N500JSONResponse{"There was an error revoking the credentials of the given connection"}}, nil
		}
	}

	err := s.connectionsService.Delete(ctx, request.Id, req.DeleteCredentials, s.cfg.APIUI.IssuerDID)
	if err != nil {
		if errors.Is(err, services.ErrConnectionDoesNotExist) {
			log.Info(ctx, "delete connection, non existing conn", "err", err, "req", request.Id.String())
			return DeleteConnection400JSONResponse{N400JSONResponse{"The given connection does not exist"}}, nil
		}
		log.Error(ctx, "delete connection", "err", err, "req", request.Id.String())
		return DeleteConnection500JSONResponse{N500JSONResponse{deleteConnection500Response(req.DeleteCredentials, req.RevokeCredentials)}}, nil
	}

	return DeleteConnection200JSONResponse{Message: deleteConnectionResponse(req.DeleteCredentials, req.RevokeCredentials)}, nil
}

// DeleteConnectionCredentials deletes all the credentials of the given connection
func (s *Server) DeleteConnectionCredentials(ctx context.Context, request DeleteConnectionCredentialsRequestObject) (DeleteConnectionCredentialsResponseObject, error) {
	err := s.connectionsService.DeleteCredentials(ctx, request.Id, s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Error(ctx, "delete connection request", err, "req", request)
		return DeleteConnectionCredentials500JSONResponse{N500JSONResponse{"There was an error deleting the credentials of the given connection"}}, nil
	}

	return DeleteConnectionCredentials200JSONResponse{Message: "Credentials of the connection successfully deleted"}, nil
}

// GetCredential returns a credential
func (s *Server) GetCredential(ctx context.Context, request GetCredentialRequestObject) (GetCredentialResponseObject, error) {
	credential, err := s.claimService.GetByID(ctx, &s.cfg.APIUI.IssuerDID, request.Id)
	if err != nil {
		if errors.Is(err, services.ErrClaimNotFound) {
			return GetCredential400JSONResponse{N400JSONResponse{"The given credential id does not exist"}}, nil
		}
		return GetCredential500JSONResponse{N500JSONResponse{"There was an error trying to retrieve the credential information"}}, nil
	}

	w3c, err := schema.FromClaimModelToW3CCredential(*credential)
	if err != nil {
		return GetCredential500JSONResponse{N500JSONResponse{"Invalid claim format"}}, nil
	}

	return GetCredential200JSONResponse(credentialResponse(w3c, credential)), nil
}

// GetCredentials returns a collection of credentials that matches the request.
func (s *Server) GetCredentials(ctx context.Context, request GetCredentialsRequestObject) (GetCredentialsResponseObject, error) {
	filter, err := getCredentialsFilter(ctx, request)
	if err != nil {
		return GetCredentials400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
	}
	credentials, total, err := s.claimService.GetAll(ctx, s.cfg.APIUI.IssuerDID, filter)
	if err != nil {
		log.Error(ctx, "loading credentials", "err", err, "req", request)
		return GetCredentials500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	response := make([]Credential, len(credentials))
	for i, credential := range credentials {
		w3c, err := schema.FromClaimModelToW3CCredential(*credential)
		if err != nil {
			log.Error(ctx, "creating credentials response", "err", err, "req", request)
			return GetCredentials500JSONResponse{N500JSONResponse{"Invalid claim format"}}, nil
		}
		response[i] = credentialResponse(w3c, credential)
	}
	return credentialsResponse(response, filter.Page, total, filter.MaxResults), nil
}

// DeleteCredential deletes a credential
func (s *Server) DeleteCredential(ctx context.Context, request DeleteCredentialRequestObject) (DeleteCredentialResponseObject, error) {
	err := s.claimService.Delete(ctx, request.Id)
	if err != nil {
		if errors.Is(err, services.ErrClaimNotFound) {
			return DeleteCredential400JSONResponse{N400JSONResponse{"The given credential does not exist"}}, nil
		}
		return DeleteCredential500JSONResponse{N500JSONResponse{"There was an error deleting the credential"}}, nil
	}

	return DeleteCredential200JSONResponse{Message: "Credential successfully deleted"}, nil
}

// GetYaml this method will be overridden in the main function
func (s *Server) GetYaml(_ context.Context, _ GetYamlRequestObject) (GetYamlResponseObject, error) {
	return nil, nil
}

// CreateCredential - creates a new credential
func (s *Server) CreateCredential(ctx context.Context, request CreateCredentialRequestObject) (CreateCredentialResponseObject, error) {
	if request.Body.SignatureProof == nil && request.Body.MtProof == nil {
		return CreateCredential400JSONResponse{N400JSONResponse{Message: "you must to provide at least one proof type"}}, nil
	}
	req := ports.NewCreateClaimRequest(&s.cfg.APIUI.IssuerDID, request.Body.CredentialSchema, request.Body.CredentialSubject, request.Body.Expiration, request.Body.Type, nil, nil, nil, request.Body.SignatureProof, request.Body.MtProof, nil, true, verifiable.CredentialStatusType(s.cfg.CredentialStatus.CredentialStatusType), toVerifiableRefreshService(request.Body.RefreshService))
	resp, err := s.claimService.Save(ctx, req)
	if err != nil {
		if errors.Is(err, services.ErrJSONLdContext) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrProcessSchema) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrLoadingSchema) {
			return CreateCredential422JSONResponse{N422JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrParseClaim) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrInvalidCredentialSubject) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrLoadingSchema) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrMalformedURL) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrUnsupportedRefreshServiceType) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		if errors.Is(err, services.ErrRefreshServiceLacksExpirationTime) {
			return CreateCredential400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		return CreateCredential500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return CreateCredential201JSONResponse{Id: resp.ID.String()}, nil
}

// RevokeCredential - revokes a credential per a given nonce
func (s *Server) RevokeCredential(ctx context.Context, request RevokeCredentialRequestObject) (RevokeCredentialResponseObject, error) {
	if err := s.claimService.Revoke(ctx, s.cfg.APIUI.IssuerDID, uint64(request.Nonce), ""); err != nil {
		if errors.Is(err, repositories.ErrClaimDoesNotExist) {
			return RevokeCredential404JSONResponse{N404JSONResponse{
				Message: "the claim does not exist",
			}}, nil
		}
		log.Error(ctx, "revoke credential", "err", err, "req", request)
		return RevokeCredential500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return RevokeCredential202JSONResponse{
		Message: "claim revocation request sent",
	}, nil
}

// GetRevocationStatus - returns weather a credential is revoked or not, this endpoint must be public available
func (s *Server) GetRevocationStatus(ctx context.Context, request GetRevocationStatusRequestObject) (GetRevocationStatusResponseObject, error) {
	rs, err := s.claimService.GetRevocationStatus(ctx, s.cfg.APIUI.IssuerDID, uint64(request.Nonce))
	if err != nil {
		return GetRevocationStatus500JSONResponse{N500JSONResponse{
			Message: err.Error(),
		}}, nil
	}

	return GetRevocationStatus200JSONResponse(getRevocationStatusResponse(rs)), err
}

// PublishState - publish the state onchange
func (s *Server) PublishState(ctx context.Context, request PublishStateRequestObject) (PublishStateResponseObject, error) {
	publishedState, err := s.publisherGateway.PublishState(ctx, &s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Error(ctx, "error publishing the state", "err", err)
		if errors.Is(err, gateways.ErrStateIsBeingProcessed) || errors.Is(err, gateways.ErrNoStatesToProcess) {
			return PublishState400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		return PublishState500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}

	return PublishState202JSONResponse{
		ClaimsTreeRoot:     publishedState.ClaimsTreeRoot,
		RevocationTreeRoot: publishedState.RevocationTreeRoot,
		RootOfRoots:        publishedState.RootOfRoots,
		State:              publishedState.State,
		TxID:               publishedState.TxID,
	}, nil
}

// RetryPublishState - retry to publish the current state if it failed previously.
func (s *Server) RetryPublishState(ctx context.Context, request RetryPublishStateRequestObject) (RetryPublishStateResponseObject, error) {
	publishedState, err := s.publisherGateway.RetryPublishState(ctx, &s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Error(ctx, "error retrying the publishing the state", "err", err)
		if errors.Is(err, gateways.ErrStateIsBeingProcessed) || errors.Is(err, gateways.ErrNoFailedStatesToProcess) {
			return RetryPublishState400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		return RetryPublishState500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return RetryPublishState202JSONResponse{
		ClaimsTreeRoot:     publishedState.ClaimsTreeRoot,
		RevocationTreeRoot: publishedState.RevocationTreeRoot,
		RootOfRoots:        publishedState.RootOfRoots,
		State:              publishedState.State,
		TxID:               publishedState.TxID,
	}, nil
}

// GetStateStatus - get the state status
func (s *Server) GetStateStatus(ctx context.Context, _ GetStateStatusRequestObject) (GetStateStatusResponseObject, error) {
	pendingActions, err := s.identityService.HasUnprocessedAndFailedStatesByID(ctx, s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Error(ctx, "get state status", "err", err)
		return GetStateStatus500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}

	return GetStateStatus200JSONResponse{PendingActions: pendingActions}, nil
}

// GetStateTransactions - get the state transactions
func (s *Server) GetStateTransactions(ctx context.Context, _ GetStateTransactionsRequestObject) (GetStateTransactionsResponseObject, error) {
	states, err := s.identityService.GetStates(ctx, s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Error(ctx, "get state transactions", "err", err)
		return GetStateTransactions500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}

	return GetStateTransactions200JSONResponse(stateTransactionsResponse(states)), nil
}

// RevokeConnectionCredentials revoke all the non revoked credentials of the given connection
func (s *Server) RevokeConnectionCredentials(ctx context.Context, request RevokeConnectionCredentialsRequestObject) (RevokeConnectionCredentialsResponseObject, error) {
	err := s.claimService.RevokeAllFromConnection(ctx, request.Id, s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Error(ctx, "revoke connection credentials", "err", err, "req", request)
		return RevokeConnectionCredentials500JSONResponse{N500JSONResponse{"There was an error revoking the credentials of the given connection"}}, nil
	}

	return RevokeConnectionCredentials202JSONResponse{Message: "Credentials revocation request sent"}, nil
}

// CreateLink - creates a link for issuing a credential
func (s *Server) CreateLink(ctx context.Context, request CreateLinkRequestObject) (CreateLinkResponseObject, error) {
	if request.Body.Expiration != nil {
		if isBeforeNow(*request.Body.Expiration) {
			return CreateLink400JSONResponse{N400JSONResponse{Message: "invalid claimLinkExpiration. Cannot be a date time prior current time."}}, nil
		}
	}
	if !request.Body.MtProof && !request.Body.SignatureProof {
		return CreateLink400JSONResponse{N400JSONResponse{Message: "at least one proof type should be enabled"}}, nil
	}
	if len(request.Body.CredentialSubject) == 0 {
		return CreateLink400JSONResponse{N400JSONResponse{Message: "you must provide at least one attribute"}}, nil
	}

	credSubject := make(domain.CredentialSubject, len(request.Body.CredentialSubject))
	for key, val := range request.Body.CredentialSubject {
		credSubject[key] = val
	}

	if request.Body.LimitedClaims != nil {
		if *request.Body.LimitedClaims <= 0 {
			return CreateLink400JSONResponse{N400JSONResponse{Message: "limitedClaims must be higher than 0"}}, nil
		}
	}

	var expirationDate *time.Time
	if request.Body.CredentialExpiration != nil {
		expirationDate = &request.Body.CredentialExpiration.Time
	}

	createdLink, err := s.linkService.Save(ctx, s.cfg.APIUI.IssuerDID, request.Body.LimitedClaims, request.Body.Expiration, request.Body.SchemaID, expirationDate, request.Body.SignatureProof, request.Body.MtProof, credSubject, toVerifiableRefreshService(request.Body.RefreshService))
	if err != nil {
		log.Error(ctx, "error saving the link", "err", err.Error())
		if errors.Is(err, services.ErrLoadingSchema) {
			return CreateLink500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
		}
		return CreateLink400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
	}
	return CreateLink201JSONResponse{Id: createdLink.ID.String()}, nil
}

// GetLink returns a link from an id
func (s *Server) GetLink(ctx context.Context, request GetLinkRequestObject) (GetLinkResponseObject, error) {
	link, err := s.linkService.GetByID(ctx, s.cfg.APIUI.IssuerDID, request.Id)
	if err != nil {
		if errors.Is(err, services.ErrLinkNotFound) {
			return GetLink404JSONResponse{N404JSONResponse{Message: "link not found"}}, nil
		}
		log.Error(ctx, "obtaining a link", "err", err.Error(), "id", request.Id)
		return GetLink500JSONResponse{N500JSONResponse{Message: "error getting link"}}, nil
	}

	return GetLink200JSONResponse(getLinkResponse(*link)), nil
}

// GetLinks - Returns a list of links based on a search criteria.
func (s *Server) GetLinks(ctx context.Context, request GetLinksRequestObject) (GetLinksResponseObject, error) {
	var err error
	status := ports.LinkAll
	if request.Params.Status != nil {
		if status, err = ports.LinkTypeReqFromString(string(*request.Params.Status)); err != nil {
			log.Warn(ctx, "unknown request type getting links", "err", err, "type", request.Params.Status)
			return GetLinks400JSONResponse{N400JSONResponse{Message: "unknown request type. Allowed: all|active|inactive|exceed"}}, nil
		}
	}
	links, err := s.linkService.GetAll(ctx, s.cfg.APIUI.IssuerDID, status, request.Params.Query)
	if err != nil {
		log.Error(ctx, "getting links", "err", err, "req", request)
	}

	return GetLinks200JSONResponse(getLinkResponses(links)), err
}

// AcivateLink - Activates or deactivates a link
func (s *Server) AcivateLink(ctx context.Context, request AcivateLinkRequestObject) (AcivateLinkResponseObject, error) {
	err := s.linkService.Activate(ctx, s.cfg.APIUI.IssuerDID, request.Id, request.Body.Active)
	if err != nil {
		if errors.Is(err, repositories.ErrLinkDoesNotExist) || errors.Is(err, services.ErrLinkAlreadyActive) || errors.Is(err, services.ErrLinkAlreadyInactive) {
			return AcivateLink400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
		}
		log.Error(ctx, "error activating or deactivating link", err.Error(), "id", request.Id)
		return AcivateLink500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return AcivateLink200JSONResponse{Message: "Link updated"}, nil
}

// DeleteLink - delete a link
func (s *Server) DeleteLink(ctx context.Context, request DeleteLinkRequestObject) (DeleteLinkResponseObject, error) {
	if err := s.linkService.Delete(ctx, request.Id, s.cfg.APIUI.IssuerDID); err != nil {
		if errors.Is(err, repositories.ErrLinkDoesNotExist) {
			return DeleteLink400JSONResponse{N400JSONResponse{Message: "link does not exist"}}, nil
		}
		return DeleteLink500JSONResponse{N500JSONResponse{Message: err.Error()}}, nil
	}
	return DeleteLink200JSONResponse{Message: "link deleted"}, nil
}

// CreateLinkQrCode - Creates a link QrCode
func (s *Server) CreateLinkQrCode(ctx context.Context, request CreateLinkQrCodeRequestObject) (CreateLinkQrCodeResponseObject, error) {
	createLinkQrCodeResponse, err := s.linkService.CreateQRCode(ctx, s.cfg.APIUI.IssuerDID, request.Id, s.cfg.APIUI.ServerURL)
	if err != nil {
		if errors.Is(err, services.ErrLinkNotFound) {
			return CreateLinkQrCode404JSONResponse{N404JSONResponse{Message: "error: link not found"}}, nil
		}
		if errors.Is(err, services.ErrLinkAlreadyExpired) || errors.Is(err, services.ErrLinkMaxExceeded) || errors.Is(err, services.ErrLinkInactive) {
			return CreateLinkQrCode404JSONResponse{N404JSONResponse{Message: "error: " + err.Error()}}, nil
		}
		log.Error(ctx, "Unexpected error while creating qr code", "err", err)
		return CreateLinkQrCode500JSONResponse{N500JSONResponse{"Unexpected error while creating qr code"}}, nil
	}
	return CreateLinkQrCode200JSONResponse{
		Issuer: IssuerDescription{
			DisplayName: s.cfg.APIUI.IssuerName,
			Logo:        s.cfg.APIUI.IssuerLogo,
		},
		QrCode:     createLinkQrCodeResponse.QrCode,
		SessionID:  createLinkQrCodeResponse.SessionID,
		LinkDetail: getLinkSimpleResponse(*createLinkQrCodeResponse.Link),
	}, nil
}

// GetCredentialQrCode - returns a QR Code for fetching the credential
func (s *Server) GetCredentialQrCode(ctx context.Context, request GetCredentialQrCodeRequestObject) (GetCredentialQrCodeResponseObject, error) {
	qrLink, schemaType, err := s.claimService.GetCredentialQrCode(ctx, &s.cfg.APIUI.IssuerDID, request.Id, s.cfg.APIUI.ServerURL)
	if err != nil {
		if errors.Is(err, services.ErrClaimNotFound) {
			return GetCredentialQrCode400JSONResponse{N400JSONResponse{"Credential not found"}}, nil
		}
		return GetCredentialQrCode500JSONResponse{N500JSONResponse{err.Error()}}, nil
	}
	return GetCredentialQrCode200JSONResponse{
		QrCodeLink: qrLink,
		SchemaType: schemaType,
	}, nil
}

// CreateLinkQrCodeCallback - Callback endpoint for the link qr code creation.
func (s *Server) CreateLinkQrCodeCallback(ctx context.Context, request CreateLinkQrCodeCallbackRequestObject) (CreateLinkQrCodeCallbackResponseObject, error) {
	if request.Body == nil || *request.Body == "" {
		log.Debug(ctx, "empty request body auth-callback request")
		return CreateLinkQrCodeCallback400JSONResponse{N400JSONResponse{"Cannot proceed with empty body"}}, nil
	}

	arm, err := s.identityService.Authenticate(ctx, *request.Body, request.Params.SessionID, s.cfg.APIUI.ServerURL, s.cfg.APIUI.IssuerDID)
	if err != nil {
		log.Debug(ctx, "error authenticating", err.Error())
		return CreateLinkQrCodeCallback500JSONResponse{}, nil
	}

	userDID, err := w3c.ParseDID(arm.From)
	if err != nil {
		log.Debug(ctx, "error getting user DID", err.Error())
		return CreateLinkQrCodeCallback500JSONResponse{}, nil
	}

	err = s.linkService.IssueClaim(ctx, request.Params.SessionID.String(), s.cfg.APIUI.IssuerDID, *userDID, request.Params.LinkID, s.cfg.APIUI.ServerURL, verifiable.CredentialStatusType(s.cfg.CredentialStatus.CredentialStatusType))
	if err != nil {
		log.Debug(ctx, "error issuing the claim", "error", err)
		return CreateLinkQrCodeCallback500JSONResponse{}, nil
	}

	return CreateLinkQrCodeCallback200Response{}, nil
}

// GetLinkQRCode - returns te qr code for adding the credential
//
//	TODO: Aquí
func (s *Server) GetLinkQRCode(ctx context.Context, request GetLinkQRCodeRequestObject) (GetLinkQRCodeResponseObject, error) {
	getQRCodeResponse, err := s.linkService.GetQRCode(ctx, request.Params.SessionID, s.cfg.APIUI.IssuerDID, request.Id)
	if err != nil {
		if errors.Is(services.ErrLinkNotFound, err) {
			return GetLinkQRCode404JSONResponse{Message: "error: link not found"}, nil
		}
		return GetLinkQRCode400JSONResponse{N400JSONResponse{Message: err.Error()}}, nil
	}

	if getQRCodeResponse.State.Status == link_state.StatusPending || getQRCodeResponse.State.Status == link_state.StatusDone || getQRCodeResponse.State.Status == link_state.StatusPendingPublish {
		return GetLinkQRCode200JSONResponse{
			Status:     common.ToPointer(getQRCodeResponse.State.Status),
			QrCode:     getQRCodeResponse.State.QRCode,
			LinkDetail: getLinkSimpleResponse(*getQRCodeResponse.Link),
		}, nil
	}

	return GetLinkQRCode400JSONResponse{N400JSONResponse{
		Message: fmt.Sprintf("error fetching the link qr code: %s", err),
	}}, nil
}

// Agent is the controller to fetch credentials from mobile
func (s *Server) Agent(ctx context.Context, request AgentRequestObject) (AgentResponseObject, error) {
	if request.Body == nil || *request.Body == "" {
		log.Debug(ctx, "agent empty request")
		return Agent400JSONResponse{N400JSONResponse{"cannot proceed with an empty request"}}, nil
	}
	basicMessage, err := s.packageManager.UnpackWithType(packers.MediaTypeZKPMessage, []byte(*request.Body))
	if err != nil {
		log.Debug(ctx, "agent bad request", "err", err, "body", *request.Body)
		return Agent400JSONResponse{N400JSONResponse{"cannot proceed with the given request"}}, nil
	}

	req, err := ports.NewAgentRequest(basicMessage)
	if err != nil {
		log.Error(ctx, "agent parsing request", "err", err)
		return Agent400JSONResponse{N400JSONResponse{err.Error()}}, nil
	}

	agent, err := s.claimService.Agent(ctx, req)
	if err != nil {
		log.Error(ctx, "agent error", "err", err)
		return Agent400JSONResponse{N400JSONResponse{err.Error()}}, nil
	}

	return Agent200JSONResponse{
		Body:     agent.Body,
		From:     agent.From,
		Id:       agent.ID,
		ThreadID: agent.ThreadID,
		To:       agent.To,
		Typ:      string(agent.Typ),
		Type:     string(agent.Type),
	}, nil
}

// GetQrFromStore is the controller to get qr bodies
func (s *Server) GetQrFromStore(ctx context.Context, request GetQrFromStoreRequestObject) (GetQrFromStoreResponseObject, error) {
	if request.Params.Id == nil {
		log.Warn(ctx, "qr store. Missing id parameter")
		return GetQrFromStore400JSONResponse{N400JSONResponse{"id is required"}}, nil
	}
	body, err := s.qrService.Find(ctx, *request.Params.Id)
	if err != nil {
		log.Error(ctx, "qr store. Finding qr", "err", err, "id", *request.Params.Id)
		return GetQrFromStore500JSONResponse{N500JSONResponse{"error looking for qr body"}}, nil
	}
	return NewQrContentResponse(body), nil
}

func getCredentialsFilter(ctx context.Context, req GetCredentialsRequestObject) (*ports.ClaimsFilter, error) {
	filter := &ports.ClaimsFilter{}
	if req.Params.Did != nil {
		did, err := w3c.ParseDID(*req.Params.Did)
		if err != nil {
			log.Warn(ctx, "get credentials. Parsing did", "err", err, "did", did)
			return nil, errors.New("cannot parse did parameter: wrong format")
		}
		filter.Subject, filter.FTSAndCond = did.String(), true
	}
	if req.Params.Status != nil {
		switch GetCredentialsParamsStatus(strings.ToLower(string(*req.Params.Status))) {
		case Revoked:
			filter.Revoked = common.ToPointer(true)
		case Expired:
			filter.ExpiredOn = common.ToPointer(time.Now())
		case All:
			// Nothing to be done
		default:
			return nil, errors.New("wrong type value. Allowed values: [all, revoked, expired]")
		}
	}
	if req.Params.Query != nil {
		filter.FTSQuery = *req.Params.Query
	}

	filter.MaxResults = 50
	if req.Params.MaxResults != nil {
		if *req.Params.MaxResults <= 0 {
			filter.MaxResults = 50
		} else {
			filter.MaxResults = *req.Params.MaxResults
		}
	}

	if req.Params.Page != nil {
		if *req.Params.Page <= 0 {
			return nil, errors.New("page param must be higher than 0")
		}
		filter.Page = req.Params.Page
	}

	return filter, nil
}

func isBeforeNow(t time.Time) bool {
	today := time.Now().UTC()
	return t.Before(today)
}

// RegisterStatic add method to the mux that are not documented in the API.
func RegisterStatic(mux *chi.Mux) {
	mux.Get("/", documentation)
	mux.Get("/static/docs/api_ui/api.yaml", swagger)
	mux.Get("/favicon.ico", favicon)
}

func documentation(w http.ResponseWriter, _ *http.Request) {
	writeFile("api_ui/spec.html", "text/html; charset=UTF-8", w)
}

func favicon(w http.ResponseWriter, _ *http.Request) {
	writeFile("api_ui/polygon.png", "image/png", w)
}

func swagger(w http.ResponseWriter, _ *http.Request) {
	writeFile("api_ui/api.yaml", "text/html; charset=UTF-8", w)
}

func writeFile(path string, mimeType string, w http.ResponseWriter) {
	f, err := os.ReadFile(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}
	w.Header().Set("Content-Type", mimeType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f)
}

func toVerifiableRefreshService(s *RefreshService) *verifiable.RefreshService {
	if s == nil {
		return nil
	}
	return &verifiable.RefreshService{
		ID:   s.Id,
		Type: verifiable.RefreshServiceType(s.Type),
	}
}
