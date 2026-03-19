package auth

import (
	"log/slog"
	"os"

	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/potibm/kasseapparat/internal/app/config"
	"github.com/potibm/kasseapparat/internal/app/exitcode"
	kasseHttp "github.com/potibm/kasseapparat/internal/app/handler/http"
	"github.com/potibm/kasseapparat/internal/app/middleware"
	sqliteRepo "github.com/potibm/kasseapparat/internal/app/repository/sqlite"
)

func InitializeJwtMiddleware(repository *sqliteRepo.Repository, jwtConfig config.JwtConfig) *jwt.GinJWTMiddleware {
	const timeout = 10 // Duration that a jwt token is valid, in minutes

	jwtMiddleware, err := jwt.New(
		middleware.InitParams(repository, jwtConfig.Realm, jwtConfig.Secret, timeout, jwtConfig.SecureCookie),
	)
	if err != nil {
		panic("[Error] failed to initialize JWT middleware: " + err.Error())
	}

	return jwtMiddleware
}

type JwtAuth struct {
	config     config.JwtConfig
	repository *sqliteRepo.Repository
	middleware *jwt.GinJWTMiddleware
}

func (auth *JwtAuth) Register(engine *gin.Engine) {

}

func (auth *JwtAuth) RegisterApiRoutes(httpHandler *kasseHttp.Handler, r *gin.RouterGroup) {
	r.POST("/auth/changePasswordToken", httpHandler.RequestChangePasswordToken)
	r.POST("/auth/changePassword", httpHandler.UpdateUserPassword)
	// No API routes for JWT, as authentication is handled via /auth/login and /auth/refresh routes
}

func (auth *JwtAuth) Middleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		errInit := auth.middleware.MiddlewareInit()
		if errInit != nil {
			slog.Error("Error initializing auth middleware", "error", errInit)
			os.Exit(int(exitcode.Software))
		}
	}
}

func NewJwtAuth(repository *sqliteRepo.Repository, config config.JwtConfig) *JwtAuth {
	return &JwtAuth{
		repository: repository,
		middleware: InitializeJwtMiddleware(repository, config),
	}
}
