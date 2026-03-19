package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/potibm/kasseapparat/internal/app/config"
	kasseHttp "github.com/potibm/kasseapparat/internal/app/handler/http"
	"github.com/potibm/kasseapparat/internal/app/models"
	sqliteRepo "github.com/potibm/kasseapparat/internal/app/repository/sqlite"
)

type Auth interface {
	Register(engine *gin.Engine)
	RegisterApiRoutes(httpHandler *kasseHttp.Handler, r *gin.RouterGroup)
}

func NewAuth(repository *sqliteRepo.Repository, config config.Config) (Auth, error) {
	oidcConfig, err := newOIDCConfig(config.OIDCConfig)
	if err != nil {
		return nil, err
	}
	if oidcConfig != nil {
		return &OIDCAuth{
			config:        *oidcConfig,
			sessionConfig: config.SessionConfig,
			repository:    repository,
		}, nil
	}
	return nil, nil
	// return NewJwtAuth(repository, config.JwtConfig), nil
}

func GetUserFromContext(c *gin.Context) (*models.User, bool) {
	user, exists := c.Get("user")
	if !exists || user == nil {
		return nil, false
	}
	return user.(*models.User), true
}
