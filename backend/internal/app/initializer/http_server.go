package initializer

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"text/template"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/olivere/vite"
	"github.com/potibm/kasseapparat/internal/app/auth"
	"github.com/potibm/kasseapparat/internal/app/config"
	httpHandler "github.com/potibm/kasseapparat/internal/app/handler/http"
	"github.com/potibm/kasseapparat/internal/app/handler/websocket"
	"github.com/potibm/kasseapparat/internal/app/middleware"
	"github.com/potibm/kasseapparat/internal/app/models"
	sqliteRepo "github.com/potibm/kasseapparat/internal/app/repository/sqlite"
	sloggin "github.com/samber/slog-gin"
)

var (
	r *gin.Engine
)

const API_VERSION = "v2"

type PublicUserData struct {
	GravatarURL string `json:"gravatarUrl"`
	Role        string `json:"role"`
	Username    string `json:"username"`
	ID          uint   `json:"id"`
}

func (u *PublicUserData) fromUser(user *models.User) {
	if user == nil {
		return
	}
	u.GravatarURL = user.GravatarURL()
	u.Role = user.Role()
	u.Username = user.Username
	u.ID = user.ID
}

type serverState struct {
	UserData   *PublicUserData `json:"userData"`
	ExpiryDate *time.Time      `json:"expiryDate"`
}

func newServerState(ctx *gin.Context) serverState {
	user, exists := auth.GetUserFromContext(ctx)
	if !exists {
		return serverState{}
	}
	publicUserData := &PublicUserData{}
	publicUserData.fromUser(user)

	return serverState{
		UserData:   publicUserData,
		ExpiryDate: nil,
	}
}

func InitializeHttpServer(
	httpHandler httpHandler.Handler,
	authImpl auth.Auth,
	websocketHandler websocket.HandlerInterface,
	repository sqliteRepo.Repository,
	config config.Config,
	logger *slog.Logger,
) (*gin.Engine, error) {
	gin.SetMode(config.AppConfig.GinMode)

	r = gin.New()
	store := cookie.NewStore([]byte(config.SessionConfig.Secret))
	r.Use(sessions.Sessions(config.SessionConfig.Name, store))
	r.Use(auth.UserMiddleware(repository))

	authImpl.Register(r)

	r.Use(
		gin.Recovery(),
		sentrygin.New(sentrygin.Options{
			Repanic: false,
		}),
		sloggin.New(logger),
		middleware.ErrorHandlingMiddleware(),
	)

	r.GET("/api/"+API_VERSION+"/purchases/stats", httpHandler.GetPurchaseStats)

	r.Use(CreateCorsMiddleware(config.CorsAllowOrigins))
	registerApiRoutes(httpHandler, websocketHandler, authImpl)
	registerVite(r, authImpl, true)

	// r.NoRoute(func(ctx *gin.Context) {
	// 	// hmm
	// 	ctx.JSON(http.StatusNotFound, gin.H{"error": "Not Foundaaa"})
	// })

	return r, nil
}

func CreateCorsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = allowedOrigins
	corsConfig.AllowAllOrigins = false
	corsConfig.AllowCredentials = false
	corsConfig.AddExposeHeaders("X-Total-Count", "Content-Disposition")

	return cors.New(corsConfig)
}

func SlogUserID() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if exists {
			if user, ok := user.(*models.User); ok {
				sloggin.AddCustomAttributes(c,
					slog.String("user_id", strconv.Itoa(int(user.ID))),
				)
			}
		}

		c.Next()
	}
}

func SentryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if exists {
			if user, ok := user.(*models.User); ok {
				sentry.ConfigureScope(func(scope *sentry.Scope) {
					scope.SetUser(sentry.User{
						ID: strconv.Itoa(int(user.ID)),
					})
				})
			}
		}

		c.Next()
	}
}

func registerApiRoutes(
	httpHandler httpHandler.Handler,
	websocketHandler websocket.HandlerInterface,
	authImpl auth.Auth,
) {

	protectedApiRouter := r.Group("/api/" + API_VERSION)
	protectedApiRouter.Use(auth.RequireAuth(), SentryMiddleware(), SlogUserID())
	{
		registerProductRoutes(protectedApiRouter, httpHandler)
		registerProductInterestRoutes(protectedApiRouter, httpHandler)
		protectedApiRouter.GET("/productStats", httpHandler.GetProductStats)

		registerGuestlistRoutes(protectedApiRouter, httpHandler)
		registerGuestRoutes(protectedApiRouter, httpHandler)
		protectedApiRouter.POST("/guestsUpload", httpHandler.ImportGuestsFromDeineTicketsCsv)

		registerPurchaseRoutes(protectedApiRouter, httpHandler)
		registerUserRoutes(protectedApiRouter, httpHandler)

		registerSumupReadersRoutes(protectedApiRouter, httpHandler)
		registerSumupTransactionRoutes(protectedApiRouter, httpHandler)
	}

	// unprotected routes
	unprotectedApiRouter := r.Group("/api/" + API_VERSION)
	{
		unprotectedApiRouter.GET("/config", httpHandler.GetConfig)
		unprotectedApiRouter.POST("/sumup/webhook", httpHandler.GetSumupTransactionWebhook)
		unprotectedApiRouter.GET("/purchases/:id/ws", websocketHandler.HandleTransactionWebSocket)

		authImpl.RegisterApiRoutes(&httpHandler, unprotectedApiRouter)
	}
}

func registerProductRoutes(rg *gin.RouterGroup, handler httpHandler.Handler) {
	products := rg.Group("/products")
	{
		products.GET("", handler.GetProducts)
		products.GET("/:id", handler.GetProductByID)
		products.GET("/:id/guests", handler.GetGuestsByProductID)
		products.PUT("/:id", handler.UpdateProductByID)
		products.DELETE("/:id", handler.DeleteProductByID)
		products.POST("", handler.CreateProduct)
	}
}

func registerGuestlistRoutes(rg *gin.RouterGroup, handler httpHandler.Handler) {
	guestlist := rg.Group("/guestlists")
	{
		guestlist.GET("", handler.GetGuestlists)
		guestlist.GET("/:id", handler.GetGuestlistByID)
		guestlist.PUT("/:id", handler.UpdateGuestlistByID)
		guestlist.DELETE("/:id", handler.DeleteGuestlistByID)
		guestlist.POST("", handler.CreateGuestlist)
	}
}

func registerGuestRoutes(rg *gin.RouterGroup, handler httpHandler.Handler) {
	guests := rg.Group("/guests")
	{
		guests.GET("", handler.GetGuests)
		guests.GET("/:id", handler.GetGuestByID)
		guests.PUT("/:id", handler.UpdateGuestByID)
		guests.DELETE("/:id", handler.DeleteGuestByID)
		guests.POST("", handler.CreateGuest)
	}
}

func registerPurchaseRoutes(
	rg *gin.RouterGroup,
	handler httpHandler.Handler,
) {
	purchases := rg.Group("/purchases")
	{
		purchases.GET("", handler.GetPurchases)
		purchases.GET("/:id", handler.GetPurchaseByID)
		purchases.POST("", handler.PostPurchases)
		purchases.DELETE("/:id", handler.DeletePurchase)
		purchases.GET("/export", handler.ExportPurchases)
		purchases.POST("/:id/refund", handler.RefundPurchase)
	}
}

func registerUserRoutes(rg *gin.RouterGroup, handler httpHandler.Handler) {
	users := rg.Group("/users")
	{
		users.GET("", handler.GetUsers)
		users.GET("/:id", handler.GetUserByID)
		users.PUT("/:id", handler.UpdateUserByID)
		users.DELETE("/:id", handler.DeleteUserByID)
		users.POST("", handler.CreateUser)
	}
}

func registerProductInterestRoutes(rg *gin.RouterGroup, handler httpHandler.Handler) {
	productInterests := rg.Group("/productInterests")
	{
		productInterests.GET("", handler.GetProductInterests)
		productInterests.DELETE("/:id", handler.DeleteProductInterestByID)
		productInterests.POST("", handler.CreateProductInterest)
	}
}

func registerSumupReadersRoutes(rg *gin.RouterGroup, handler httpHandler.Handler) {
	sumupReaders := rg.Group("/sumup/readers")
	{
		sumupReaders.GET("", handler.GetSumupReaders)
		sumupReaders.GET("/:id", handler.GetSumupReaderByID)
		sumupReaders.DELETE("/:id", handler.DeleteSumupReader)
		sumupReaders.POST("", handler.CreateSumupReader)
	}
}
func registerSumupTransactionRoutes(rg *gin.RouterGroup, handler httpHandler.Handler) {
	sumupTransactions := rg.Group("/sumup/transactions")
	{
		sumupTransactions.GET("", handler.GetSumupTransactions)
		sumupTransactions.GET("/:id", handler.GetSumupTransactionByID)
	}
}

func registerVite(r *gin.Engine, authImpl auth.Auth, isDev bool) {
	// Initialize the helper function
	viteFragment, err := vite.HTMLFragment(vite.Config{
		FS:        os.DirFS("../frontend/dist"), // Required: Vite build output directory
		IsDev:     isDev,                        // Required: Development or Production mode
		ViteURL:   "http://localhost:5173",      // Optional: Defaults to this URL
		ViteEntry: "src/main.jsx",               // Optional: Depends on your frontend setup
	})
	if err != nil {
		panic(err)
	}

	// Create a template
	tmpl := template.Must(template.New("index").Parse(indexTemplate))
	r.NoRoute(func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
		serverState := newServerState(ctx)
		// marshal server state to json and pass it to the template
		serverStateJson, err := json.Marshal(serverState)
		pageData := map[string]any{
			"Vite":        viteFragment,
			"ServerState": string(serverStateJson),
		}

		err = tmpl.Execute(ctx.Writer, pageData)
		if err != nil {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
			return
		}

	})
}

const indexTemplate = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <link rel="icon" href="/favicon.ico" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="theme-color" content="#000000" /> 
    <meta
        name="description"
        content="Small Point-of-Sale System for demoparties"
    />
    <!--
      manifest.json provides metadata used when your web app is installed on a
      user's mobile device or desktop. See https://developers.google.com/web/fundamentals/web-app-manifest/
          -->
        <link rel="manifest" href="/manifest.json" />
        <title>Kasseapparat</title>
        <script>
			const serverState = {{ .ServerState }};
        </script>
        {{ .Vite.Tags }}
  </head>
  <body>
    <noscript>You need to enable JavaScript to run this app.</noscript>
    <div id="root"></div>
  </body>
</html>
`
