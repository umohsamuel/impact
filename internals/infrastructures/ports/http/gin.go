package http

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/umohsamuel/impact/internals/configs/env"
	"github.com/umohsamuel/impact/internals/configs/response"
	"github.com/umohsamuel/impact/internals/infrastructures/db/gen"
	"github.com/umohsamuel/impact/internals/infrastructures/ports/http/handlers"
	"github.com/umohsamuel/impact/internals/services"
)

type GinServer struct {
	Services    *services.Services
	Engine      *gin.Engine
	Environment *env.EnvironmentVariables
}

type apiConfig struct {
	DB *gen.Queries
}

func NewGinServer(services *services.Services, environment *env.EnvironmentVariables) *GinServer {

	ginServer := &GinServer{
		Services:    services,
		Engine:      gin.Default(),
		Environment: environment,
	}

	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"POST", "GET", "PUT", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization", "Accept", "User-Agent", "Cache-Control", "Pragma"}
	config.ExposeHeaders = []string{"Content-Length"}
	config.AllowCredentials = true
	config.MaxAge = 12 * time.Hour

	ginServer.Engine.Use(cors.New(config))

	ginServer.Engine.Static("/downloads", "tmp")

	ginServer.Health()

	api := ginServer.Engine.Group("/api/v1")
	ginServer.User(api)
	ginServer.Ffmpeg(api)

	return ginServer

}

func (server *GinServer) Health() {
	server.Engine.GET("/health", func(c *gin.Context) {
		response.NewSuccessResponse("server up!!!", nil, nil).Send(c)
	})
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func (server *GinServer) User(rg *gin.RouterGroup) {

	rg.POST("/create/user", func(c *gin.Context) {

		var user CreateUserRequest

		if err := c.ShouldBindJSON(&user); err != nil {

			response.NewErrorResponse(err).Send(c)

			return
		}

		updatedUser, err := server.Services.Queries.CreateUser(c, user.Name)

		if err != nil {
			response.NewErrorResponse(err).Send(c)

			return
		}

		response.NewSuccessResponse("success", updatedUser, nil).Send(c)
	})

}

func (server *GinServer) Ffmpeg(rg *gin.RouterGroup) {
	ffmpegHandler := handlers.NewFFMPEGHandler(server.Environment, server.Services.LLMImplementation)

	rg.POST("/generate-impact-frames", ffmpegHandler.ExtractFrames)
}
