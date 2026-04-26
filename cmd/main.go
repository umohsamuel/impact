package main

import (
	"context"
	"fmt"
	"log"

	"github.com/umohsamuel/impact/internals/configs/connections"
	"github.com/umohsamuel/impact/internals/configs/env"
	configs "github.com/umohsamuel/impact/internals/configs/goth"
	"github.com/umohsamuel/impact/internals/infrastructures/adapters"
	"github.com/umohsamuel/impact/internals/infrastructures/db"
	"github.com/umohsamuel/impact/internals/infrastructures/ports"
	"github.com/umohsamuel/impact/internals/services"
)

var (
	environmentVariables = env.LoadEnvironment()
)

func init() {
	configs.Goth(environmentVariables)
}

func main() {

	ctx := context.Background()

	pool, err := db.NewPool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, "SELECT NOW()")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("---- db connected successfully!!! ----")

	newGoogleGenAIClient := connections.NewGoogleGenAIClient(environmentVariables)

	adapterDependencies := adapters.AdapterDependencies{
		EnvironmentVariables: environmentVariables,
		DB:                   pool,
		GoogleGenAIClient:    newGoogleGenAIClient,
	}
	newAdapters := adapters.NewAdapters(adapterDependencies)
	newServices := services.NewServices(newAdapters)
	newPort := ports.NewPort(newServices, environmentVariables)
	newPort.GinServer.Engine.Run()
}
