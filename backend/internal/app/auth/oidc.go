package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/potibm/kasseapparat/internal/app/config"
	kasseHttp "github.com/potibm/kasseapparat/internal/app/handler/http"
	"github.com/potibm/kasseapparat/internal/app/models"
	sqliteRepo "github.com/potibm/kasseapparat/internal/app/repository/sqlite"
	"golang.org/x/oauth2"
)

// the oidc library builds on top of oauth2 and you actually need tow config structs :S
type OIDCConfig struct {
	provider *oidc.Provider
	oidc     oidc.Config
	oauth2   oauth2.Config
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
		provider: provider,
		oidc:     oidcConfig,
		oauth2:   oauth2Config,
	}, nil
}

// required for kasseapparat
type userInfoClaims struct {
	UserName string `json:"preferred_username"`
}

func generateRandomString(nByte int) (string, error) {
	b := make([]byte, nByte)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (auth *OIDCAuth) oidcLogin(c *gin.Context) {
	state, err := generateRandomString(16) // Use a cryptographically secure random string generator
	if err != nil {
		_ = c.Error(kasseHttp.InternalServerError.WithCauseMsg(err))

		return
	}
	nonce, err := generateRandomString(16)
	if err != nil {
		_ = c.Error(kasseHttp.InternalServerError.WithCauseMsg(err))

		return
	}
	codeVerifier := oauth2.GenerateVerifier()
	codeChallenge := oauth2.S256ChallengeOption(codeVerifier)
	session := sessions.Default(c)
	session.Set("oidc_state", state)
	session.Set("oidc_nonce", nonce)
	session.Set("oidc_code_verifier", codeVerifier)
	session.Save()
	authURL := auth.config.oauth2.AuthCodeURL(state, oidc.Nonce(nonce), codeChallenge)
	slog.Info("Redirecting to OIDC provider", slog.String("authURL", authURL))
	c.Redirect(http.StatusFound, authURL)
}

func (auth *OIDCAuth) validateCallbackRequest(c *gin.Context) (*oauth2.Token, error) {
	code := c.Query("code")
	state := c.Query("state")

	session := sessions.Default(c)
	expectedState := session.Get("oidc_state")
	codeVerifier := session.Get("oidc_code_verifier").(string)

	session.Delete("oidc_state")
	session.Delete("oidc_code_verifier")

	if state != expectedState {
		return nil, errors.New("Invalid state parameter")
	}
	codeVerifierOption := oauth2.VerifierOption(codeVerifier)
	token, err := auth.config.oauth2.Exchange(c, code, codeVerifierOption)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (auth *OIDCAuth) getOrCreateUserFromToken(c *gin.Context, token *oauth2.Token) (*models.User, error) {
	userInfo, err := auth.config.provider.UserInfo(c, oauth2.StaticTokenSource(token))
	if err != nil {
		return nil, err
	}
	// this is NOT how you should do it. we would need more info in the userModel to properly handle OIDC
	// we would need to link the user with issuer and subject and merge existing accounts etcpp.
	// HOWEVER as we only have one OIDC provider and we are either allowing OIDC or classic auth this should be safe
	// IF auth isn't going to be changed after initial deployment and if we never have more than one OIDC provider this is NOT safe.
	user, err := auth.repository.GetUserByEmail(userInfo.Email)
	if err != nil {
		return nil, err
	}
	claims := userInfoClaims{}
	if err := userInfo.Claims(&claims); err != nil {
		return nil, err
	}
	if user != nil {
		user.Username = claims.UserName
		user.Email = userInfo.Email
		auth.repository.UpdateUserByID(int(user.ID), *user)
	} else {
		createdUser, err := auth.repository.CreateUser(models.User{
			Username: claims.UserName,
			Email:    userInfo.Email,
		})
		if err != nil {
			return nil, err
		}
		user = &createdUser
	}
	return user, nil
}

func (auth *OIDCAuth) oidcCallback(c *gin.Context) {
	session := sessions.Default(c)

	token, err := auth.validateCallbackRequest(c)
	if err != nil {
		slog.Error("Failed to validate OIDC callback request", "error", err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	user, err := auth.getOrCreateUserFromToken(c, token)
	if err != nil {
		slog.Error("Failed to get or create user from OIDC token", "error", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err})
		return
	}

	session.Set("refresh_token", token.RefreshToken)
	session.Set("user_id", int(user.ID))
	session.Set("expiry", token.Expiry)
	session.Save()
	c.Redirect(http.StatusFound, "/")
}

func (auth *OIDCAuth) Expiry() *time.Time {
	return nil
}

func NewOIDCAuth(repository *sqliteRepo.Repository, oidcConfig OIDCConfig, sessionConfig config.SessionConfig) *OIDCAuth {
	return &OIDCAuth{
		config:        oidcConfig,
		sessionConfig: sessionConfig,
	}
}

type OIDCAuth struct {
	repository    *sqliteRepo.Repository
	config        OIDCConfig
	sessionConfig config.SessionConfig
}

func (auth *OIDCAuth) Register(engine *gin.Engine) {
	engine.GET("/oidc/login", auth.oidcLogin)
	engine.GET("/oidc/callback", auth.oidcCallback)
}

func (auth *OIDCAuth) RegisterApiRoutes(httpHandler *kasseHttp.Handler, r *gin.RouterGroup) {

}

func (a *OIDCAuth) GetUser() (*models.User, error) {
	return nil, nil
}
