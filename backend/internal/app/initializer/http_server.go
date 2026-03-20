package initializer

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
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
	staticFiles embed.FS,
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
	registerVite(r, staticFiles, config.AppConfig.GinMode != "release")

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

func registerVite(r *gin.Engine, staticFiles embed.FS, isDev bool) {
	// Initialize the helper function
	var config vite.Config
	if isDev {
		config = vite.Config{
			FS:        os.DirFS("../frontend/dist"), // Point to the frontend directory in development
			IsDev:     true,
			ViteURL:   "http://localhost:5173",
			PublicFS:  os.DirFS("../frontend/public"),
			ViteEntry: "src/main.jsx",
		}
	} else {
		assetsFs, err := fs.Sub(staticFiles, "assets")
		if err != nil {
			slog.Error("Error creating sub file system for static files", "error", err)
			panic(err)
		}
		config = vite.Config{
			FS:        assetsFs, // Use the embedded filesystem in production
			IsDev:     false,
			ViteEntry: "src/main.jsx",
		}
	}

	// slog.Info("Registering Vite with file system", "isDev", isDev)
	// viteFragment, err := vite.HTMLFragment(vite.Config{
	// 	FS:        fsImpl,                  // Required: Vite build output directory
	// 	IsDev:     isDev,                   // Required: Development or Production mode
	// 	ViteURL:   "http://localhost:5173", // Optional: Defaults to this URL
	// 	ViteEntry: "src/main.jsx",          // Optional: Depends on your frontend setup
	// })
	// if err != nil {
	// 	panic(err)
	// }

	// // Create a template
	// tmpl := template.Must(template.New("index").Parse(indexTemplate))
	// r.NoRoute(func(ctx *gin.Context) {
	// 	ctx.Status(http.StatusOK)
	// 	serverState := newServerState(ctx)
	// 	// marshal server state to json and pass it to the template
	// 	serverStateJson, err := json.Marshal(serverState)
	// 	pageData := map[string]any{
	// 		"Vite":        viteFragment,
	// 		"ServerState": string(serverStateJson),
	// 	}

	// 	err = tmpl.Execute(ctx.Writer, pageData)
	// 	if err != nil {
	// 		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
	// 		return
	// 	}

	// })

	viteHandler, err := vite.NewHandler(config)
	if err != nil {
		slog.Error("Error initializing Vite handler", "error", err)
		panic(err)
	}

	// Create a new handler.
	handler := func(ctx *gin.Context) {
		// ctx.Status(http.StatusOK)
		serverState := newServerState(ctx)
		// marshal server state to json and pass it to the template
		serverStateJson, err := json.Marshal(serverState)
		if err != nil {
			slog.Error("Error marshaling server state to JSON", "error", err)
			ctx.AbortWithStatusJSON(httpHandler.InternalServerError.Code, gin.H{"error": "Internal Server Error"})
			return
		}
		r := ctx.Request
		w := ctx.Writer
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			// Serve the index.html file.
			requestCtx := r.Context()

			// create a script tag containing the server state json
			scriptTag := `<script>const serverState = ` + string(serverStateJson) + `;</script>`

			requestCtx = vite.ScriptsToContext(requestCtx, scriptTag)

			viteHandler.ServeHTTP(w, r.WithContext(requestCtx))
			return
		}

		slog.Info("Serving static file", "path", r.URL.Path)

		viteHandler.ServeHTTP(w, r)
	}

	r.NoRoute(handler)
}
