package ports

import (
	"github.com/umohsamuel/impact/internals/configs/env"
	"github.com/umohsamuel/impact/internals/infrastructures/ports/http"
	"github.com/umohsamuel/impact/internals/services"
)

type Ports struct {
	GinServer *http.GinServer
}

func NewPort(services *services.Services, environment *env.EnvironmentVariables) *Ports {

	return &Ports{
		GinServer: http.NewGinServer(services, environment),
	}
}
