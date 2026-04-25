package main

import (
	"context"
	"fmt"
	"log"

	"github.com/umohsamuel/impact/internals/configs/env"
	configs "github.com/umohsamuel/impact/internals/configs/goth"
	"github.com/umohsamuel/impact/internals/configs/logger"
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
	newLogger := logger.NewSugarLogger(environmentVariables.ProductionEnvironment)

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

	logger.NewSugarLogger(false).LogWithFields("info", "db connected successfully!!!")
	defer rows.Close()

	for rows.Next() {
		var now string
		rows.Scan(&now)
		fmt.Println("DB time:", now)
	}

	adapterDependencies := adapters.AdapterDependencies{
		Logger:               newLogger,
		EnvironmentVariables: environmentVariables,
		DB:                   pool,
	}
	newAdapters := adapters.NewAdapters(adapterDependencies)
	newServices := services.NewServices(newAdapters)
	newPort := ports.NewPort(newServices, newLogger, environmentVariables)
	newPort.GinServer.Engine.Run()
}
