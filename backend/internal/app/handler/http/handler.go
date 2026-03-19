package http

import (
	"context"
	"log/slog"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/potibm/kasseapparat/internal/app/config"
	"github.com/potibm/kasseapparat/internal/app/mailer"
	"github.com/potibm/kasseapparat/internal/app/models"
	"github.com/potibm/kasseapparat/internal/app/monitor"
	sqliteRepo "github.com/potibm/kasseapparat/internal/app/repository/sqlite"
	sumupRepo "github.com/potibm/kasseapparat/internal/app/repository/sumup"
	purchaseService "github.com/potibm/kasseapparat/internal/app/service/purchase"
	"golang.org/x/oauth2"
)

type StatusPublisher interface {
	PushUpdate(purchaseID uuid.UUID, status models.PurchaseStatus)
}

type Handler struct {
	repo            sqliteRepo.RepositoryInterface
	sumupRepository sumupRepo.RepositoryInterface
	purchaseService purchaseService.Service
	monitor         monitor.Poller
	statusPublisher StatusPublisher
	mailer          mailer.Mailer
	config          config.Config
	decimalPlaces   int32
	oidcConfig      *OIDCConfig
}

// the oidc library builds on top of oauth2 and you actually need tow config structs :S
type OIDCConfig struct {
	oidc   oidc.Config
	oauth2 oauth2.Config
}

func newOIDCConfig(oidcConfigValues config.OIDCConfig) (*OIDCConfig, error) {
	if oidcConfigValues.ClientID == "" {
		slog.Info("OIDC ClientID not set. skipping OIDC config")
		return nil, nil
	}
	if oidcConfigValues.ClientSecret == "" {
		slog.Info("OIDC ClientSecret not set. skipping OIDC config")
		return nil, nil
	}
	if oidcConfigValues.IssuerURL == "" {
		slog.Info("OIDC IssuerURL not set. skipping OIDC config")
		return nil, nil
	}
	if oidcConfigValues.RedirectURL == "" {
		slog.Info("OIDC RedirectURL not set. skipping OIDC config")
		return nil, nil
	}

	provider, err := oidc.NewProvider(context.Background(), oidcConfigValues.IssuerURL)
	if err != nil {
		return nil, err
	}

	oidcConfig := oidc.Config{
		ClientID: oidcConfigValues.ClientID,
	}

	oauth2Config := oauth2.Config{
		ClientID:     oidcConfigValues.ClientID,
		ClientSecret: oidcConfigValues.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  oidcConfigValues.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	return &OIDCConfig{
		oidc:   oidcConfig,
		oauth2: oauth2Config,
	}, nil
}

type HandlerConfig struct {
	Repo            sqliteRepo.RepositoryInterface
	SumupRepository sumupRepo.RepositoryInterface
	PurchaseService purchaseService.Service
	Monitor         monitor.Poller
	StatusPublisher StatusPublisher
	Mailer          mailer.Mailer
	AppConfig       config.Config
	OIDCConfig      *OIDCConfig
}

func NewHandler(config HandlerConfig) *Handler {
	oidcConfig, err := newOIDCConfig(config.AppConfig.OIDCConfig)
	if err != nil {
		slog.Error("Failed to initialize OIDC config. Skipping OIDC...", "error", err)
	}

	return &Handler{
		repo:            config.Repo,
		sumupRepository: config.SumupRepository,
		purchaseService: config.PurchaseService,
		monitor:         config.Monitor,
		statusPublisher: config.StatusPublisher,
		mailer:          config.Mailer,
		config:          config.AppConfig,
		decimalPlaces:   int32(config.AppConfig.FormatConfig.FractionDigitsMax),
		oidcConfig:      oidcConfig,
	}
}
